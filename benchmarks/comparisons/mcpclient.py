#!/usr/bin/env python3
"""Minimal MCP client (stdlib only) for the comparison harness.

Speaks JSON-RPC 2.0 over:
  * stdio (newline-delimited JSON, per the MCP stdio transport), and
  * Streamable HTTP (POST, Accept: application/json + text/event-stream,
    Mcp-Session-Id carried after initialize).

Used as a library by run_all.py, or from the CLI:

  python3 -I mcpclient.py --stdio -- node dbhub.js --dsn sqlite:///x.db
  python3 -I mcpclient.py --http http://127.0.0.1:8080/mcp -H 'X-API-Key: ...'

The CLI prints initialize + tools/list results as JSON.
"""
import argparse
import json
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request

PROTOCOL_VERSION = "2025-06-18"
CLIENT_INFO = {"name": "faucet-comparison-harness", "version": "1.0"}


class MCPError(Exception):
    pass


def compact(obj):
    return json.dumps(obj, separators=(",", ":"), ensure_ascii=False)


class _Base:
    def __init__(self):
        self._id = 0
        self.wire_log = []  # (direction, raw text) for the raw results file

    def _next_id(self):
        self._id += 1
        return self._id

    def request(self, method, params=None, timeout=60):
        raise NotImplementedError

    def notify(self, method, params=None):
        raise NotImplementedError

    def initialize(self):
        res = self.request("initialize", {
            "protocolVersion": PROTOCOL_VERSION,
            "capabilities": {},
            "clientInfo": CLIENT_INFO,
        })
        self.notify("notifications/initialized")
        return res

    def tools_list(self):
        tools, cursor, pages = [], None, []
        while True:
            params = {"cursor": cursor} if cursor else {}
            res = self.request("tools/list", params)
            pages.append(res)
            r = res.get("result", {})
            tools.extend(r.get("tools", []))
            cursor = r.get("nextCursor")
            if not cursor:
                break
        return tools, pages

    def call_tool(self, name, arguments, timeout=60):
        return self.request("tools/call", {"name": name, "arguments": arguments}, timeout=timeout)


class StdioClient(_Base):
    def __init__(self, argv, env=None, cwd=None):
        super().__init__()
        self.proc = subprocess.Popen(
            argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            env=env, cwd=cwd, text=True, bufsize=1)
        self.stderr_lines = []
        self._responses = {}
        self._cv = threading.Condition()
        threading.Thread(target=self._read_stdout, daemon=True).start()
        threading.Thread(target=self._read_stderr, daemon=True).start()

    def _read_stdout(self):
        for line in self.proc.stdout:
            line = line.strip()
            if not line:
                continue
            self.wire_log.append(("<", line))
            try:
                msg = json.loads(line)
            except ValueError:
                continue  # non-JSON noise on stdout
            if "id" in msg and ("result" in msg or "error" in msg):
                with self._cv:
                    self._responses[msg["id"]] = (msg, len(line.encode()))
                    self._cv.notify_all()

    def _read_stderr(self):
        for line in self.proc.stderr:
            self.stderr_lines.append(line.rstrip("\n"))

    def _send(self, obj):
        line = compact(obj)
        self.wire_log.append((">", line))
        self.proc.stdin.write(line + "\n")
        self.proc.stdin.flush()

    def request(self, method, params=None, timeout=60):
        rid = self._next_id()
        msg = {"jsonrpc": "2.0", "id": rid, "method": method}
        if params is not None:
            msg["params"] = params
        self._send(msg)
        deadline = time.time() + timeout
        with self._cv:
            while rid not in self._responses:
                left = deadline - time.time()
                if left <= 0 or self.proc.poll() is not None and rid not in self._responses:
                    raise MCPError(f"no response to {method} (exit={self.proc.poll()}) stderr={self.stderr_lines[-5:]}")
                self._cv.wait(min(left, 0.2))
            msg, nbytes = self._responses.pop(rid)
        msg["_wire_bytes"] = nbytes
        return msg

    def notify(self, method, params=None):
        msg = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            msg["params"] = params
        self._send(msg)

    def close(self):
        try:
            self.proc.stdin.close()
        except Exception:
            pass
        try:
            self.proc.terminate()
            self.proc.wait(5)
        except Exception:
            self.proc.kill()


class HTTPClient(_Base):
    def __init__(self, url, headers=None):
        super().__init__()
        self.url = url
        self.headers = dict(headers or {})
        self.session_id = None
        self.protocol_version = None

    def _post(self, obj, timeout=60):
        body = compact(obj).encode()
        self.wire_log.append((">", body.decode()))
        h = {"Content-Type": "application/json",
             "Accept": "application/json, text/event-stream"}
        h.update(self.headers)
        if self.session_id:
            h["Mcp-Session-Id"] = self.session_id
        if self.protocol_version:
            h["MCP-Protocol-Version"] = self.protocol_version
        req = urllib.request.Request(self.url, data=body, headers=h, method="POST")
        try:
            resp = urllib.request.urlopen(req, timeout=timeout)
        except urllib.error.HTTPError as e:
            text = e.read().decode(errors="replace")
            self.wire_log.append(("<", f"HTTP {e.code} {text}"))
            raise MCPError(f"HTTP {e.code}: {text[:500]}")
        sid = resp.headers.get("Mcp-Session-Id")
        if sid:
            self.session_id = sid
        ctype = resp.headers.get("Content-Type", "")
        raw = resp.read().decode(errors="replace")
        self.wire_log.append(("<", raw))
        return ctype, raw, resp.status

    @staticmethod
    def _parse(ctype, raw, rid):
        if "text/event-stream" in ctype:
            msgs, data = [], []
            for line in raw.splitlines():
                if line.startswith("data:"):
                    data.append(line[5:].lstrip())
                elif line == "" and data:
                    msgs.append("\n".join(data))
                    data = []
            if data:
                msgs.append("\n".join(data))
            for m in msgs:
                try:
                    obj = json.loads(m)
                except ValueError:
                    continue
                if obj.get("id") == rid:
                    return obj, len(m.encode())
            raise MCPError(f"no matching SSE message for id {rid}: {raw[:300]}")
        obj = json.loads(raw)
        if isinstance(obj, list):
            obj = next(o for o in obj if o.get("id") == rid)
        return obj, len(raw.encode())

    def request(self, method, params=None, timeout=60):
        rid = self._next_id()
        msg = {"jsonrpc": "2.0", "id": rid, "method": method}
        if params is not None:
            msg["params"] = params
        ctype, raw, _ = self._post(msg, timeout)
        obj, nbytes = self._parse(ctype, raw, rid)
        obj["_wire_bytes"] = nbytes
        if method == "initialize" and "result" in obj:
            self.protocol_version = obj["result"].get("protocolVersion")
        return obj

    def notify(self, method, params=None):
        msg = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            msg["params"] = params
        try:
            self._post(msg)
        except MCPError:
            pass

    def close(self):
        if self.session_id:
            h = dict(self.headers)
            h["Mcp-Session-Id"] = self.session_id
            try:
                urllib.request.urlopen(urllib.request.Request(self.url, headers=h, method="DELETE"), timeout=5)
            except Exception:
                pass


def tools_list_metrics(tools):
    """Size of the tools/list result payload ({"tools":[...]}) serialized compactly."""
    payload = compact({"tools": tools}).encode()
    return {
        "tool_count": len(tools),
        "tool_names": [t.get("name") for t in tools],
        "tools_list_result_bytes": len(payload),
        "token_estimate_bytes_div_4": round(len(payload) / 4),
        "annotations": {t.get("name"): t.get("annotations") for t in tools},
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--stdio", action="store_true")
    ap.add_argument("--http")
    ap.add_argument("-H", "--header", action="append", default=[])
    ap.add_argument("cmd", nargs=argparse.REMAINDER)
    a = ap.parse_args()
    if a.http:
        hdrs = dict(h.split(":", 1) for h in a.header)
        c = HTTPClient(a.http, {k.strip(): v.strip() for k, v in hdrs.items()})
    else:
        cmd = a.cmd[1:] if a.cmd and a.cmd[0] == "--" else a.cmd
        c = StdioClient(cmd)
    try:
        init = c.initialize()
        tools, _ = c.tools_list()
        print(json.dumps({"initialize": init, "metrics": tools_list_metrics(tools), "tools": tools}, indent=2))
    finally:
        c.close()


if __name__ == "__main__":
    sys.exit(main())
