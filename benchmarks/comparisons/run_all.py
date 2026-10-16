#!/usr/bin/env python3
"""Hands-on comparison harness: Faucet vs other SQLite-capable MCP / REST tools.

Every tool is pointed at its own fresh copy of the same Chinook SQLite file.
For each tool the harness measures (where applicable):
  1. cold start: process launch -> first successful query (seconds)
  2. config needed for a read-only setup (recorded as data, see CONFIG_NOTES)
  3. MCP tools/list: tool count, compact JSON bytes, bytes/4 token *estimate*
  4. read-only enforcement: INSERT / UPDATE / DELETE through every exposed path,
     with the server's answer AND the ground truth read back from the file
  5. whether the HTTP endpoint answers an unauthenticated request
  6. default row cap on SELECT * FROM InvoiceLine (2240 rows)
  7. whether an OpenAPI document is served

Run:  HARNESS_DOWNLOADS=/path/to/downloads python3 -I run_all.py [tool ...]
Tools: faucet dbhub toolbox execauto datasette sqlite_rest ws4sqlite pocketbase
"""
import json
import os
import re
import shutil
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time
import traceback
import urllib.error
import urllib.parse
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import mcpclient  # noqa: E402

DL = os.environ.get("HARNESS_DOWNLOADS")
if not DL:
    sys.exit("set HARNESS_DOWNLOADS")
RESULTS = os.path.join(HERE, "results")
RAW = os.path.join(RESULTS, "raw")
os.makedirs(RAW, exist_ok=True)
WORK = os.environ.get("HARNESS_WORK") or tempfile.mkdtemp(prefix="harness-work-")
os.makedirs(WORK, exist_ok=True)
CHINOOK = os.path.join(DL, "chinook", "Chinook_Sqlite.sqlite")

BIN = {
    "faucet": os.path.join(DL, "faucet", "faucet"),
    "dbhub": os.path.join(DL, "dbhub", "inst", "node_modules", "@bytebase", "dbhub", "dist", "index.js"),
    "execauto": os.path.join(DL, "execauto", "inst", "node_modules", "@executeautomation",
                             "database-server", "dist", "src", "index.js"),
    "toolbox": os.path.join(DL, "toolbox", "toolbox"),
    "datasette": os.path.join(DL, "datasette", "venv", "bin", "datasette"),
    "sqlite_rest": os.path.join(DL, "sqlite-rest", "sqlite-rest"),
    "ws4sqlite": os.path.join(DL, "ws4sqlite", "ws4sqlite"),
    "pocketbase": os.path.join(DL, "pocketbase", "pocketbase"),
}

LARGE_SELECT = "SELECT * FROM InvoiceLine"
PROBE_SELECT = "SELECT Name FROM Artist WHERE ArtistId = 1"


# ----------------------------------------------------------------- helpers
def free_port():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    p = s.getsockname()[1]
    s.close()
    return p


def fresh_db(name):
    p = os.path.join(WORK, name + ".db")
    for suffix in ("", "-wal", "-shm"):
        if os.path.exists(p + suffix):
            os.remove(p + suffix)
    shutil.copyfile(CHINOOK, p)
    return p


def mutations(k):
    """Distinct targets per path k so attempts don't interfere with each other."""
    return {
        "insert": {"sql": f"INSERT INTO Artist (ArtistId, Name) VALUES ({9000 + k}, 'harness-insert-{k}')",
                   "artist_id": 9000 + k},
        "update": {"sql": f"UPDATE Artist SET Name = 'harness-update-{k}' WHERE ArtistId = {10 + k}",
                   "artist_id": 10 + k, "name": f"harness-update-{k}"},
        "delete": {"sql": f"DELETE FROM InvoiceLine WHERE InvoiceLineId = {2240 - k}",
                   "invoice_line_id": 2240 - k},
        # CTE-prefixed DELETE: valid SQLite, but does not start with DELETE.
        "cte_delete": {"sql": f"WITH x AS (SELECT 1) DELETE FROM InvoiceLine WHERE InvoiceLineId = {2100 - k}",
                       "invoice_line_id": 2100 - k},
    }


def landed(db, op, m):
    """Ground truth from the file itself (opened read-only)."""
    for _ in range(3):
        try:
            c = sqlite3.connect(f"file:{db}?mode=ro", uri=True, timeout=5)
            try:
                if op == "insert":
                    return c.execute("SELECT count(*) FROM Artist WHERE ArtistId=?", (m["artist_id"],)).fetchone()[0] == 1
                if op == "update":
                    return c.execute("SELECT Name FROM Artist WHERE ArtistId=?", (m["artist_id"],)).fetchone()[0] == m["name"]
                if op in ("delete", "cte_delete"):
                    return c.execute("SELECT count(*) FROM InvoiceLine WHERE InvoiceLineId=?",
                                     (m["invoice_line_id"],)).fetchone()[0] == 0
            finally:
                c.close()
        except sqlite3.OperationalError:
            time.sleep(0.3)
    return None


def http(method, url, headers=None, body=None, timeout=30):
    data = None
    h = dict(headers or {})
    if body is not None:
        data = body if isinstance(body, bytes) else json.dumps(body).encode()
        h.setdefault("Content-Type", "application/json")
    req = urllib.request.Request(url, data=data, headers=h, method=method)
    try:
        r = urllib.request.urlopen(req, timeout=timeout)
        return r.status, dict(r.headers), r.read().decode(errors="replace")
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers), e.read().decode(errors="replace")
    except Exception as e:  # connection refused etc.
        return None, {}, repr(e)


def short(text, n=300):
    text = text if isinstance(text, str) else json.dumps(text)
    return text if len(text) <= n else text[:n] + "...[truncated]"


class Proc:
    def __init__(self, name, argv, env=None, cwd=None):
        self.log_path = os.path.join(WORK, name + ".log")
        self.log = open(self.log_path, "w")
        e = dict(os.environ)
        e.update(env or {})
        self.t0 = time.monotonic()
        self.p = subprocess.Popen(argv, stdout=self.log, stderr=subprocess.STDOUT, env=e, cwd=cwd or WORK,
                                  stdin=subprocess.DEVNULL)

    def elapsed(self):
        return round(time.monotonic() - self.t0, 3)

    def stop(self):
        if self.p.poll() is None:
            self.p.terminate()
            try:
                self.p.wait(10)
            except subprocess.TimeoutExpired:
                self.p.kill()
        self.log.close()

    def tail(self, n=20):
        try:
            return open(self.log_path).read().splitlines()[-n:]
        except Exception:
            return []


def wait_for(fn, timeout=60, interval=0.05):
    end = time.monotonic() + timeout
    last = None
    while time.monotonic() < end:
        try:
            ok, last = fn()
            if ok:
                return True, last
        except Exception as e:  # noqa
            last = repr(e)
        time.sleep(interval)
    return False, last


def tool_text(resp):
    """Return (is_error, text) for a tools/call JSON-RPC response."""
    if "error" in resp:
        return True, json.dumps(resp["error"])
    r = resp.get("result", {})
    texts = [c.get("text", "") for c in r.get("content", []) if c.get("type") == "text"]
    if not texts and "structuredContent" in r:
        texts = [json.dumps(r["structuredContent"])]
    return bool(r.get("isError")), "\n".join(texts)


def count_rows_in_text(text):
    """Best effort: count rows in a tool's JSON text output."""
    lines = [l for l in text.splitlines() if l.strip()]
    if len(lines) > 1 and all(l.lstrip().startswith("{") and l.rstrip().endswith("}") for l in lines):
        try:
            return len([json.loads(l) for l in lines])  # one JSON object per line (NDJSON-style)
        except ValueError:
            pass
    try:
        obj = json.loads(text)
    except Exception:
        # some servers wrap JSON in prose; find the first JSON value
        for opener in ("{", "["):
            i = text.find(opener)
            if i >= 0:
                try:
                    obj = json.JSONDecoder().raw_decode(text[i:])[0]
                    break
                except Exception:
                    continue
        else:
            return None
    return _rows(obj)


def _rows(obj):
    if isinstance(obj, list):
        if obj and isinstance(obj[0], dict) and len(obj) == 1:
            inner = _rows(obj[0])
            if inner is not None and inner != 1:
                return inner
        return len(obj)
    if isinstance(obj, dict):
        for key in ("rows", "records", "resource", "data", "statements", "result", "results", "resultSet", "items"):
            if key in obj:
                r = _rows(obj[key])
                if r is not None:
                    return r
        if "count" in obj and isinstance(obj["count"], int):
            return obj["count"]
    return None


def mcp_metrics(client, tag):
    init = client.initialize()
    tools, pages = client.tools_list()
    m = mcpclient.tools_list_metrics(tools)
    with open(os.path.join(RAW, f"{tag}_tools_list.json"), "w") as f:
        json.dump({"initialize": init, "tools_list_pages": pages}, f, indent=2)
    m["server_info"] = init.get("result", {}).get("serverInfo")
    m["protocol_version"] = init.get("result", {}).get("protocolVersion")
    return m


def mcp_attempt(client, db, k, op, tool, args_fn, path_label):
    m = mutations(k)[op]
    t = time.monotonic()
    try:
        resp = client.call_tool(tool, args_fn(m))
        is_err, text = tool_text(resp)
    except mcpclient.MCPError as e:
        is_err, text = True, f"transport error: {e}"
    did = landed(db, op, m)
    return {"path": path_label, "op": op, "statement_or_args": args_fn(m), "server_reported_error": is_err,
            "server_message": short(text, 400), "write_landed_in_file": did,
            "verdict": "ALLOWED" if did else "BLOCKED", "ms": round((time.monotonic() - t) * 1000, 1)}


def rest_attempt(db, k, op, method, url, headers, body_fn, path_label):
    m = mutations(k)[op]
    u = url(m) if callable(url) else url
    b = body_fn(m) if body_fn else None
    st, _, text = http(method, u, headers, b)
    did = landed(db, op, m)
    return {"path": path_label, "op": op, "request": f"{method} {u}", "body": b, "http_status": st,
            "server_message": short(text, 400), "write_landed_in_file": did,
            "verdict": "ALLOWED" if did else "BLOCKED"}


def save(name, data):
    data["measured_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    with open(os.path.join(RESULTS, f"{name}.json"), "w") as f:
        json.dump(data, f, indent=2, default=str)
    print(f"[{name}] saved results/{name}.json")


def version_of(argv):
    try:
        return subprocess.run(argv, capture_output=True, text=True, timeout=30).stdout.strip() or \
            subprocess.run(argv, capture_output=True, text=True, timeout=30).stderr.strip()
    except Exception as e:
        return repr(e)


# ================================================================= FAUCET
def run_faucet():
    out = {"tool": "Faucet", "kind": ["mcp", "rest"]}
    fb = BIN["faucet"]
    out["version"] = version_of([fb, "version", "--json"])
    try:
        out["git_describe"] = subprocess.run(["git", "-C", os.environ.get("FAUCET_SRC", os.path.join(HERE, "..", "..")),
                                              "describe", "--tags", "--always", "--dirty"],
                                             capture_output=True, text=True).stdout.strip()
    except Exception:
        out["git_describe"] = "unknown"
    data_dir = os.path.join(WORK, "faucet-data")
    shutil.rmtree(data_dir, ignore_errors=True)
    db = fresh_db("faucet")
    port = free_port()
    env = {"FAUCET_TELEMETRY": "0", "FAUCET_ADMIN_EMAIL": "admin@example.com",
           "FAUCET_ADMIN_PASSWORD": "harness-pass-123"}
    base = f"http://127.0.0.1:{port}"
    steps = []

    def cli(*args):
        t = time.monotonic()
        r = subprocess.run([fb, "--data-dir", data_dir, *args], capture_output=True, text=True,
                           env={**os.environ, **env})
        steps.append({"cmd": "faucet " + " ".join(args), "exit": r.returncode,
                      "seconds": round(time.monotonic() - t, 3),
                      "stdout": short(re.sub(r"faucet_[0-9a-f]{64}", "faucet_<redacted>", r.stdout + r.stderr), 600)})
        return r

    # ---- cold start: CLI setup + serve + first query with a GET-only key
    t_all = time.monotonic()
    cli("db", "add", "--name", "chinook", "--driver", "sqlite", "--path", db)
    cli("role", "create", "--name", "readonly", "--verbs", "GET", "--description", "read-only")
    r = cli("key", "create", "--role", "readonly", "--label", "harness")
    key = next((tok for tok in r.stdout.split() if tok.startswith("faucet_")), None)
    out["api_key_prefix"] = key[:15] if key else None
    srv = Proc("faucet-serve", [fb, "--data-dir", data_dir, "serve", "--foreground", "--host", "127.0.0.1",
                                "-p", str(port)], env=env)
    try:
        hk = {"X-API-Key": key}
        ok, last = wait_for(lambda: (http("GET", f"{base}/api/v1/chinook/_table/Artist?limit=1", hk)[0] == 200, None))
        out["cold_start"] = {
            "definition": "faucet db add + role create + key create + serve --foreground launch -> first 200 from GET /api/v1/chinook/_table/Artist with the key (binary already built)",
            "seconds_total": round(time.monotonic() - t_all, 3),
            "seconds_serve_launch_to_first_query": srv.elapsed() if ok else None,
            "ok": ok, "steps": steps}

        # ---- sample REST filter call
        q = urllib.parse.urlencode({"filter": "Milliseconds > 300000 AND GenreId = 1",
                                    "fields": "TrackId,Name,Milliseconds", "order": "Milliseconds DESC",
                                    "limit": "3", "include_count": "true"})
        st, _, body = http("GET", f"{base}/api/v1/chinook/_table/Track?{q}", hk)
        out["sample_rest_filter"] = {"request": f"GET /api/v1/chinook/_table/Track?{q}", "http_status": st,
                                     "response": json.loads(body) if st == 200 else body}

        # ---- auth: unauthenticated requests
        out["auth_unauthenticated"] = {}
        for label, path in [("rest_table", "/api/v1/chinook/_table/Artist"), ("openapi_json", "/openapi.json"),
                            ("service_doc", "/api/v1/chinook/_doc"), ("healthz", "/healthz")]:
            st, _, body = http("GET", base + path)
            out["auth_unauthenticated"][label] = {"GET": path, "http_status": st, "body": short(body, 150)}
        st, _, body = http("POST", base + "/mcp", {"Accept": "application/json, text/event-stream"},
                           {"jsonrpc": "2.0", "id": 1, "method": "initialize",
                            "params": {"protocolVersion": "2025-06-18", "capabilities": {},
                                       "clientInfo": {"name": "x", "version": "1"}}})
        out["auth_unauthenticated"]["mcp_initialize"] = {"POST": "/mcp", "http_status": st, "body": short(body, 150)}

        # ---- OpenAPI
        st, _, body = http("GET", base + "/openapi.json")
        spec = json.loads(body) if st == 200 else {}
        out["openapi"] = {"served": st == 200, "openapi_version": spec.get("openapi"),
                          "path_count": len(spec.get("paths", {})), "bytes": len(body),
                          "endpoints": ["/openapi.json (combined, unauthenticated)", "/api/v1/{service}/_doc"]}
        with open(os.path.join(RAW, "faucet_openapi.json"), "w") as f:
            f.write(body)

        # ---- row caps (REST)
        caps = {}
        for label, qs in [("no_limit_param", ""), ("limit_5000", "?limit=5000")]:
            st, _, body = http("GET", f"{base}/api/v1/chinook/_table/InvoiceLine{qs}", hk)
            caps["rest_" + label] = {"http_status": st, "rows": len(json.loads(body).get("resource", [])) if st == 200 else None}
        out["row_cap"] = caps

        # ---- MCP over Streamable HTTP with the GET-only key
        c = mcpclient.HTTPClient(base + "/mcp", hk)
        try:
            out["mcp_http_with_key"] = mcp_metrics(c, "faucet_http_key")
            resp = c.call_tool("faucet_query", {"service": "chinook", "table": "InvoiceLine"})
            caps["mcp_faucet_query_no_limit"] = {"rows": count_rows_in_text(tool_text(resp)[1])}
            resp = c.call_tool("faucet_query", {"service": "chinook", "table": "InvoiceLine", "limit": 5000})
            caps["mcp_faucet_query_limit_5000"] = {"rows": count_rows_in_text(tool_text(resp)[1]),
                                                   "message": short(tool_text(resp)[1], 200)}
            ro = []
            # path index k keeps targets distinct
            ro += faucet_mcp_writes(c, db, 1, "MCP http, GET-only key")
            out["readonly_A_rbac_get_only_key"] = {
                "setup": "CLI only: role with --verbs GET + API key; service read_only flag NOT set (no CLI flag exists for it)",
                "attempts": ro + faucet_rest_writes(base, hk, db, 5, "REST, GET-only key")}
        finally:
            c.close()

        # ---- admin session; service read_only=true + raw_sql_allowed=true via admin API
        st, _, body = http("POST", base + "/api/v1/system/admin/session", None,
                           {"email": env["FAUCET_ADMIN_EMAIL"], "password": env["FAUCET_ADMIN_PASSWORD"]})
        jwt = json.loads(body).get("session_token") if st == 200 else None
        ha = {"Authorization": f"Bearer {jwt}"}
        # B0: defaults (read_only=false, raw_sql off) as admin, to show what is open by default
        c = mcpclient.HTTPClient(base + "/mcp", ha)
        try:
            c.initialize()
            attempts = faucet_mcp_writes(c, db, 9, "MCP http, admin JWT, service defaults")
        finally:
            c.close()
        out["default_B0_admin_no_readonly_flag"] = {
            "setup": "service defaults (read_only=false, raw_sql_allowed=false); admin JWT (admins bypass RBAC)",
            "attempts": attempts}
        st, _, body = http("PUT", base + "/api/v1/system/service/chinook", ha,
                           {"read_only": True, "raw_sql_allowed": True})
        out["set_read_only_via_admin_api"] = {"request": "PUT /api/v1/system/service/chinook {\"read_only\":true,\"raw_sql_allowed\":true}",
                                              "http_status": st, "body": short(body, 300)}
        c = mcpclient.HTTPClient(base + "/mcp", ha)
        try:
            c.initialize()
            attempts = faucet_mcp_writes(c, db, 13, "MCP http, admin JWT, read_only service")
        finally:
            c.close()
        attempts += faucet_rest_writes(base, ha, db, 17, "REST, admin JWT, read_only service")
        out["readonly_B_service_flag"] = {
            "setup": "service read_only=true (and raw_sql_allowed=true to prove read_only still wins), set via admin REST API; requests made as admin",
            "attempts": attempts}
    finally:
        srv.stop()

    # ---- stdio MCP (runs as local admin) with the read_only service, then with read_only off
    def stdio_round(k, label):
        c = mcpclient.StdioClient([fb, "--data-dir", data_dir, "mcp"], env={**os.environ, **env})
        try:
            m = mcp_metrics(c, f"faucet_stdio_{k}")
            return m, faucet_mcp_writes(c, db, k, label)
        finally:
            c.close()

    m, att = stdio_round(21, "MCP stdio (admin), read_only service")
    out["mcp_stdio"] = m
    out["readonly_C_stdio_read_only_service"] = {"setup": "faucet mcp (stdio, runs as admin) with service read_only=true", "attempts": att}
    # turn read_only off again through a temporary server, then stdio writes
    srv = Proc("faucet-serve2", [fb, "--data-dir", data_dir, "serve", "--foreground", "--host", "127.0.0.1",
                                 "-p", str(port)], env=env)
    try:
        wait_for(lambda: (http("GET", base + "/healthz")[0] == 200, None))
        st, _, body = http("POST", base + "/api/v1/system/admin/session", None,
                           {"email": env["FAUCET_ADMIN_EMAIL"], "password": env["FAUCET_ADMIN_PASSWORD"]})
        jwt = json.loads(body).get("session_token")
        http("PUT", base + "/api/v1/system/service/chinook", {"Authorization": f"Bearer {jwt}"},
             {"read_only": False, "raw_sql_allowed": False})
    finally:
        srv.stop()
    m, att = stdio_round(25, "MCP stdio (admin), service defaults")
    out["default_D_stdio_no_readonly_flag"] = {"setup": "faucet mcp (stdio, admin) with service defaults (read_only=false, raw_sql off)", "attempts": att}

    # stdio cold start: launch -> first faucet_query result
    t = time.monotonic()
    c = mcpclient.StdioClient([fb, "--data-dir", data_dir, "mcp"], env={**os.environ, **env})
    try:
        c.initialize()
        resp = c.call_tool("faucet_query", {"service": "chinook", "table": "Artist", "limit": 1})
        out["cold_start_mcp_stdio_seconds"] = round(time.monotonic() - t, 3)
        out["cold_start_mcp_stdio_ok"] = not tool_text(resp)[0]
    finally:
        c.close()
    out["config_for_read_only"] = {
        "files": 0,
        "commands": ["faucet db add --name chinook --driver sqlite --path chinook.db",
                     "faucet role create --name readonly --verbs GET",
                     "faucet key create --role readonly"],
        "note": "The per-service read_only flag (which also blocks admins and stdio MCP) has no CLI flag in this build; it is set via the admin UI or PUT /api/v1/system/service/{name}."}
    save("faucet", out)


def faucet_mcp_writes(c, db, k, label):
    a = []
    a.append(mcp_attempt(c, db, k, "insert", "faucet_insert",
                         lambda m: {"service": "chinook", "table": "Artist",
                                    "records": [{"ArtistId": m["artist_id"], "Name": "harness-insert"}]},
                         label + " / faucet_insert"))
    a.append(mcp_attempt(c, db, k, "update", "faucet_update",
                         lambda m: {"service": "chinook", "table": "Artist", "filter": f"ArtistId = {m['artist_id']}",
                                    "record": {"Name": m["name"]}}, label + " / faucet_update"))
    a.append(mcp_attempt(c, db, k, "delete", "faucet_delete",
                         lambda m: {"service": "chinook", "table": "InvoiceLine",
                                    "filter": f"InvoiceLineId = {m['invoice_line_id']}"}, label + " / faucet_delete"))
    for j, op in enumerate(("insert", "update", "delete")):
        a.append(mcp_attempt(c, db, k + 1 + j, op, "faucet_raw_sql",
                             lambda m: {"service": "chinook", "sql": m["sql"]}, label + " / faucet_raw_sql"))
    return a


def faucet_rest_writes(base, h, db, k, label):
    t = f"{base}/api/v1/chinook/_table"
    return [
        rest_attempt(db, k, "insert", "POST", f"{t}/Artist", h,
                     lambda m: {"ArtistId": m["artist_id"], "Name": "harness-insert"}, label + " / POST"),
        rest_attempt(db, k, "update", "PATCH",
                     lambda m: f"{t}/Artist?" + urllib.parse.urlencode({"filter": f"ArtistId = {m['artist_id']}"}), h,
                     lambda m: {"Name": m["name"]}, label + " / PATCH"),
        rest_attempt(db, k, "delete", "DELETE",
                     lambda m: f"{t}/InvoiceLine?" + urllib.parse.urlencode({"filter": f"InvoiceLineId = {m['invoice_line_id']}"}),
                     h, None, label + " / DELETE"),
    ]


# ================================================================= DBHUB
def run_dbhub():
    out = {"tool": "DBHub (@bytebase/dbhub)", "kind": ["mcp"], "version": "1.5.0 (npm)"}
    js = BIN["dbhub"]

    def sql_writes(c, db, k, label, include_cte=True):
        ops = ["insert", "update", "delete"] + (["cte_delete"] if include_cte else [])
        return [mcp_attempt(c, db, k + j, op, "execute_sql", lambda m: {"sql": m["sql"]}, label + " / execute_sql")
                for j, op in enumerate(ops)]

    # --- default: --dsn only, HTTP transport
    db = fresh_db("dbhub-default")
    port = free_port()
    srv = Proc("dbhub-default", ["node", js, "--transport", "http", "--port", str(port), "--dsn", f"sqlite://{db}"])
    url = f"http://127.0.0.1:{port}/mcp"
    try:
        def first_query():
            c = mcpclient.HTTPClient(url)
            c.initialize()
            r = c.call_tool("execute_sql", {"sql": PROBE_SELECT})
            return (not tool_text(r)[0]), None
        ok, last = wait_for(first_query, timeout=60, interval=0.1)
        out["cold_start"] = {"definition": "node dbhub --transport http --dsn sqlite://... launch -> first successful execute_sql SELECT over Streamable HTTP (package already installed)",
                             "seconds": srv.elapsed() if ok else None, "ok": ok, "last_error": last}
        c = mcpclient.HTTPClient(url)
        c.initialize()
        out["mcp_tools_list_default"] = mcp_metrics(c, "dbhub_default")
        r = c.call_tool("execute_sql", {"sql": LARGE_SELECT})
        out["row_cap"] = {"execute_sql_select_star_InvoiceLine": {"rows": count_rows_in_text(tool_text(r)[1]),
                                                                  "message_head": short(tool_text(r)[1], 200)}}
        out["default_writes"] = {"setup": "defaults (--dsn only)", "attempts": sql_writes(c, db, 1, "default")}
        st, h, body = http("POST", url, {"Accept": "application/json, text/event-stream"},
                           {"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
        out["auth_unauthenticated"] = {"mcp": {"POST": "/mcp tools/list without any credential", "http_status": st,
                                               "body": short(body, 150)},
                                       "listen_address_log": [l for l in srv.tail(60) if "listening" in l.lower() or "Auth" in l]}
        st, _, body = http("GET", f"http://127.0.0.1:{port}/api/sources")
        out["auth_unauthenticated"]["workbench_api_sources"] = {"GET": "/api/sources", "http_status": st, "body": short(body, 150)}
    finally:
        srv.stop()

    # --- read-only via dbhub.toml
    db = fresh_db("dbhub-ro")
    toml = os.path.join(WORK, "dbhub.toml")
    cfg = f'[[sources]]\nid = "chinook"\ndsn = "sqlite://{db}"\n\n[[tools]]\nname = "execute_sql"\nsource = "chinook"\nreadonly = true\n'
    open(toml, "w").write(cfg)
    port = free_port()
    srv = Proc("dbhub-ro", ["node", js, "--transport", "http", "--port", str(port), "--config", toml])
    url = f"http://127.0.0.1:{port}/mcp"
    try:
        ok, last = wait_for(lambda: (http("GET", f"http://127.0.0.1:{port}/healthz")[0] == 200, None), 60)
        c = mcpclient.HTTPClient(url)
        c.initialize()
        out["mcp_tools_list_readonly"] = mcp_metrics(c, "dbhub_readonly")
        out["readonly"] = {"setup": "dbhub.toml with [[tools]] execute_sql readonly = true", "config_text": cfg,
                           "attempts": sql_writes(c, db, 11, "readonly")}
        r = c.call_tool("execute_sql", {"sql": LARGE_SELECT})
        out["row_cap"]["readonly_execute_sql_select_star_InvoiceLine"] = {"rows": count_rows_in_text(tool_text(r)[1])}
    finally:
        srv.stop()
    out["config_for_read_only"] = {"files": 1, "lines": len([l for l in cfg.splitlines() if l.strip()]),
                                   "note": "--readonly CLI flag was removed; read-only is per-tool in dbhub.toml"}
    out["openapi"] = {"served": False, "note": "MCP server only; no REST API / OpenAPI document"}
    save("dbhub", out)


# ================================================================= TOOLBOX
def run_toolbox():
    tb = BIN["toolbox"]
    out = {"tool": "MCP Toolbox for Databases (googleapis/mcp-toolbox, formerly genai-toolbox)", "kind": ["mcp"],
           "version": version_of([tb, "--version"])}

    def sql_writes(c, db, k, label):
        return [mcp_attempt(c, db, k + j, op, "execute_sql", lambda m: {"sql": m["sql"]}, label + " / execute_sql")
                for j, op in enumerate(["insert", "update", "delete", "cte_delete"])]

    for mode in ("default", "ro_uri"):
        db = fresh_db("toolbox-" + mode)
        port = free_port()
        dbspec = db if mode == "default" else f"file:{db}?mode=ro"
        srv = Proc("toolbox-" + mode, [tb, "--prebuilt", "sqlite", "--port", str(port), "--disable-version-check"],
                   env={"SQLITE_DATABASE": dbspec})
        url = f"http://127.0.0.1:{port}/mcp"
        try:
            def first_query():
                c = mcpclient.HTTPClient(url)
                c.initialize()
                r = c.call_tool("execute_sql", {"sql": PROBE_SELECT})
                return (not tool_text(r)[0]), tool_text(r)[1]
            ok, last = wait_for(first_query, timeout=60, interval=0.1)
            if mode == "default":
                out["cold_start"] = {"definition": "SQLITE_DATABASE=... toolbox --prebuilt sqlite launch -> first successful execute_sql SELECT over Streamable HTTP",
                                     "seconds": srv.elapsed() if ok else None, "ok": ok, "last": short(str(last), 200)}
            c = mcpclient.HTTPClient(url)
            c.initialize()
            out[f"mcp_tools_list_{mode}"] = mcp_metrics(c, f"toolbox_{mode}")
            label = {"default": "prebuilt sqlite, defaults", "ro_uri": "prebuilt sqlite, SQLITE_DATABASE=file:...?mode=ro"}[mode]
            if mode == "default":  # row cap measured before any write attempt touches InvoiceLine
                r = c.call_tool("execute_sql", {"sql": LARGE_SELECT})
                out["row_cap"] = {"execute_sql_select_star_InvoiceLine": {"rows": count_rows_in_text(tool_text(r)[1]),
                                                                          "message_head": short(tool_text(r)[1], 200)}}
            out[f"writes_{mode}"] = {"setup": label, "attempts": sql_writes(c, db, 1 if mode == "default" else 11, label)}
            if mode == "default":
                st, _, body = http("POST", url, {"Accept": "application/json, text/event-stream"},
                                   {"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
                out["auth_unauthenticated"] = {"mcp": {"POST": "/mcp tools/list, no credential", "http_status": st,
                                                       "body": short(body, 150)},
                                               "default_bind": "127.0.0.1 (from --help: --address default)"}
        finally:
            srv.stop()
    out["config_for_read_only"] = {
        "files": 0, "env": "SQLITE_DATABASE=file:/path/chinook.db?mode=ro",
        "note": "The sqlite source has no read-only option (Source.IsReadOnly() returns false in v1.14.0 source). Read-only here relies on the SQLite URI mode=ro, i.e. the database driver, not Toolbox. A custom tools.yaml exposing only fixed-statement sqlite-sql tools is the other option (not measured)."}
    out["openapi"] = {"served": "not measured as OpenAPI", "note": "--enable-api exposes a native /api endpoint; no OpenAPI document checked"}
    save("toolbox", out)


# ================================================================= EXECUTEAUTOMATION
def run_execauto():
    js = BIN["execauto"]
    out = {"tool": "ExecuteAutomation database-server (@executeautomation/database-server)", "kind": ["mcp"],
           "version": "1.1.0 (npm)", "transport": "stdio only"}
    db = fresh_db("execauto")
    t = time.monotonic()
    c = mcpclient.StdioClient(["node", js, db])
    try:
        c.initialize()
        r = c.call_tool("read_query", {"query": PROBE_SELECT})
        out["cold_start"] = {"definition": "node index.js <db> (stdio) launch -> first successful read_query",
                             "seconds": round(time.monotonic() - t, 3), "ok": not tool_text(r)[0]}
        tools, _ = c.tools_list()
        m = mcpclient.tools_list_metrics(tools)
        with open(os.path.join(RAW, "execauto_tools_list.json"), "w") as f:
            json.dump(tools, f, indent=2)
        out["mcp_tools_list"] = m
        r = c.call_tool("read_query", {"query": LARGE_SELECT})
        out["row_cap"] = {"read_query_select_star_InvoiceLine": {"rows": count_rows_in_text(tool_text(r)[1]),
                                                                 "message_head": short(tool_text(r)[1], 200)}}
        a = []
        for j, op in enumerate(["insert", "update", "delete"]):
            a.append(mcp_attempt(c, db, 1 + j, op, "write_query", lambda m: {"query": m["sql"]}, "write_query"))
        for j, op in enumerate(["insert", "update", "delete", "cte_delete"]):
            a.append(mcp_attempt(c, db, 11 + j, op, "read_query", lambda m: {"query": m["sql"]}, "read_query"))
        out["default_writes"] = {"setup": "defaults (no read-only mode exists)", "attempts": a}
    finally:
        c.close()
    out["config_for_read_only"] = {"files": None, "note": "No read-only option in v1.1.0 (CLI accepts only the db path / connection flags); write_query, create_table, alter_table, drop_table are always listed."}
    out["auth_unauthenticated"] = {"note": "stdio only, no HTTP endpoint"}
    out["openapi"] = {"served": False}
    save("execauto", out)


# ================================================================= DATASETTE
def run_datasette():
    ds = BIN["datasette"]
    out = {"tool": "Datasette", "kind": ["rest"], "version": version_of([ds, "--version"])}
    db = fresh_db("chinook")  # datasette names the database after the file
    port = free_port()
    srv = Proc("datasette", [ds, "serve", db, "-h", "127.0.0.1", "-p", str(port)])
    base = f"http://127.0.0.1:{port}"
    try:
        ok, _ = wait_for(lambda: (http("GET", f"{base}/chinook/Artist.json?_size=1")[0] == 200, None), 60)
        out["cold_start"] = {"definition": "datasette serve chinook.db launch -> first 200 from /chinook/Artist.json",
                             "seconds": srv.elapsed() if ok else None, "ok": ok}
        st, _, body = http("GET", f"{base}/chinook/InvoiceLine.json?_shape=array")
        st2, _, body2 = http("GET", f"{base}/chinook.json?" + urllib.parse.urlencode({"sql": LARGE_SELECT, "_shape": "array"}))
        st3, _, body3 = http("GET", f"{base}/chinook.json?" + urllib.parse.urlencode({"sql": LARGE_SELECT}))
        meta = json.loads(body3) if st3 == 200 else {}
        out["row_cap"] = {"table_json_default_page": {"http_status": st, "rows": len(json.loads(body)) if st == 200 else None},
                          "arbitrary_sql_select_star": {"http_status": st2, "rows": len(json.loads(body2)) if st2 == 200 else None,
                                                        "truncated_flag": meta.get("truncated")}}
        a = []
        for j, op in enumerate(["insert", "update", "delete", "cte_delete"]):
            a.append(rest_attempt(db, 1 + j, op, "GET",
                                  lambda m: f"{base}/chinook.json?" + urllib.parse.urlencode({"sql": m["sql"]}),
                                  None, None, "GET /chinook.json?sql="))
        a.append(rest_attempt(db, 11, "insert", "POST", f"{base}/chinook/Artist/-/insert", None,
                              lambda m: {"row": {"ArtistId": m["artist_id"], "Name": "x"}}, "POST /-/insert (1.0 write API)"))
        a.append(rest_attempt(db, 12, "delete", "POST", lambda m: f"{base}/chinook/InvoiceLine/{m['invoice_line_id']}/-/delete",
                              None, lambda m: {}, "POST /<row>/-/delete (1.0 write API)"))
        out["default_writes"] = {"setup": "defaults (datasette serve chinook.db)", "attempts": a}
        st, _, body = http("GET", f"{base}/chinook/Artist.json?_size=1")
        out["auth_unauthenticated"] = {"rest": {"GET": "/chinook/Artist.json", "http_status": st}}
        for p in ("/-/openapi.json", "/openapi.json"):
            st, _, _ = http("GET", base + p)
            out.setdefault("openapi", {})[p] = st
        out["openapi"]["served"] = False if all(v != 200 for v in out["openapi"].values()) else True
    finally:
        srv.stop()
    out["config_for_read_only"] = {"files": 0, "note": "0.65.x has no write API; arbitrary SQL runs on a read-only connection. Plugins can add writes."}
    save("datasette", out)


# ================================================================= SQLITE-REST
def _jwt_hs256(secret):
    import base64
    import hashlib
    import hmac

    def b64(b):
        return base64.urlsafe_b64encode(b).rstrip(b"=")
    head = b64(json.dumps({"alg": "HS256", "typ": "JWT"}).encode())
    body = b64(json.dumps({"sub": "harness"}).encode())
    sig = b64(hmac.new(secret, head + b"." + body, hashlib.sha256).digest())
    return (head + b"." + body + b"." + sig).decode()


def run_sqlite_rest():
    sr = BIN["sqlite_rest"]
    out = {"tool": "sqlite-rest (b4fun)", "kind": ["rest"], "version": "v0.4.1 (go install, no release binaries)"}
    secret = b"harness-secret-0123456789abcdef"
    tok_file = os.path.join(WORK, "sqlite-rest.token")
    open(tok_file, "wb").write(secret)
    hdr = {"Authorization": "Bearer " + _jwt_hs256(secret)}

    # startup without auth flags
    r = subprocess.run([sr, "serve", "--db-dsn", fresh_db("sr-noauth"), "--http-addr", "127.0.0.1:0", "--metrics-addr", ""],
                       capture_output=True, text=True, timeout=30)
    out["start_without_auth_flags"] = {"exit": r.returncode, "message": short([l for l in (r.stdout + r.stderr).splitlines() if "auth" in l.lower()][:1], 300)}

    for mode in ("default", "ro_uri"):
        db = fresh_db("sr-" + mode)
        port = free_port()
        dsn = db if mode == "default" else f"file:{db}?mode=ro"
        srv = Proc("sqlite-rest-" + mode, [sr, "serve", "--db-dsn", dsn, "--http-addr", f"127.0.0.1:{port}",
                                           "--metrics-addr", "", "--auth-token-file", tok_file,
                                           "--security-allow-table", "Artist,InvoiceLine"])
        base = f"http://127.0.0.1:{port}"
        try:
            ok, _ = wait_for(lambda: (http("GET", f"{base}/Artist?limit=1", hdr)[0] == 200, None), 60)
            if mode == "default":
                out["cold_start"] = {"definition": "sqlite-rest serve (token file + allow-table) launch -> first 200 from GET /Artist",
                                     "seconds": srv.elapsed() if ok else None, "ok": ok}
                st, _, body = http("GET", f"{base}/InvoiceLine", hdr)
                out["row_cap"] = {"GET /InvoiceLine": {"http_status": st, "rows": len(json.loads(body)) if st == 200 else None}}
                st, _, body = http("GET", f"{base}/Artist?limit=1")
                out["auth_unauthenticated"] = {"rest": {"GET": "/Artist", "http_status": st, "body": short(body, 150)}}
                st, _, body = http("GET", f"{base}/Album?limit=1", hdr)
                out["table_not_allowlisted"] = {"GET": "/Album", "http_status": st}
                st, _, body = http("GET", f"{base}/", hdr)
                out["openapi"] = {"GET /": st, "body_head": short(body, 150), "served": st == 200 and "openapi" in body.lower()}
            k = 1 if mode == "default" else 11
            a = [
                rest_attempt(db, k, "insert", "POST", f"{base}/Artist", hdr,
                             lambda m: {"ArtistId": m["artist_id"], "Name": "harness-insert"}, "POST /Artist"),
                rest_attempt(db, k, "update", "PATCH", lambda m: f"{base}/Artist?ArtistId=eq.{m['artist_id']}", hdr,
                             lambda m: {"Name": m["name"]}, "PATCH /Artist"),
                rest_attempt(db, k, "delete", "DELETE", lambda m: f"{base}/InvoiceLine?InvoiceLineId=eq.{m['invoice_line_id']}",
                             hdr, None, "DELETE /InvoiceLine"),
            ]
            out[f"writes_{mode}"] = {"setup": {"default": "token file + --security-allow-table Artist,InvoiceLine",
                                               "ro_uri": "same, with --db-dsn file:...?mode=ro"}[mode], "attempts": a}
        finally:
            srv.stop()
    out["config_for_read_only"] = {"files": 1, "note": "token file (HS256 secret) is mandatory; no read-only switch, allow-listed tables are read & write; read-only relies on the SQLite URI mode=ro"}
    save("sqlite_rest", out)


# ================================================================= WS4SQLITE
def run_ws4sqlite():
    ws = BIN["ws4sqlite"]
    out = {"tool": "ws4sqlite", "kind": ["rest"], "version": version_of([ws, "-version"])}
    for mode in ("default", "readonly"):
        d = os.path.join(WORK, "ws4-" + mode)
        shutil.rmtree(d, ignore_errors=True)
        os.makedirs(d)
        db = os.path.join(d, "chinook.db")
        shutil.copyfile(CHINOOK, db)
        cfg = None
        if mode == "readonly":
            cfg = "readOnly: true\n"
            open(os.path.join(d, "chinook.yaml"), "w").write(cfg)
        port = free_port()
        srv = Proc("ws4sqlite-" + mode, [ws, "--db", db, "--port", str(port), "--bind-host", "127.0.0.1"])
        url = f"http://127.0.0.1:{port}/chinook"

        def q(sql, key="query"):
            return http("POST", url, None, {"transaction": [{key: sql}]})
        try:
            ok, _ = wait_for(lambda: (q(PROBE_SELECT)[0] == 200, None), 60)
            if mode == "default":
                out["cold_start"] = {"definition": "ws4sqlite --db chinook.db launch -> first 200 query",
                                     "seconds": srv.elapsed() if ok else None, "ok": ok}
                st, _, body = q(LARGE_SELECT)
                rs = json.loads(body)["results"][0].get("resultSet") if st == 200 else None
                out["row_cap"] = {"query SELECT * FROM InvoiceLine": {"http_status": st, "rows": len(rs) if rs is not None else None}}
                out["auth_unauthenticated"] = {"rest": {"POST": "/chinook query, no credential", "http_status": st}}
                for p in ("/openapi.json", "/chinook/openapi.json"):
                    out.setdefault("openapi", {})[p] = http("GET", f"http://127.0.0.1:{port}{p}")[0]
                out["openapi"]["served"] = False
            k = 1 if mode == "default" else 11
            a = []
            for j, op in enumerate(["insert", "update", "delete"]):
                a.append(rest_attempt(db, k + j, op, "POST", url, None,
                                      lambda m: {"transaction": [{"statement": m["sql"]}]}, "POST statement"))
            for j, op in enumerate(["insert", "update", "delete", "cte_delete"]):
                a.append(rest_attempt(db, k + 4 + j, op, "POST", url, None,
                                      lambda m: {"transaction": [{"query": m["sql"]}]}, "POST query (read field)"))
            out[f"writes_{mode}"] = {"setup": {"default": "defaults", "readonly": "chinook.yaml: readOnly: true"}[mode],
                                     "config_text": cfg, "attempts": a}
        finally:
            srv.stop()
    out["config_for_read_only"] = {"files": 1, "lines": 1, "text": "readOnly: true (companion chinook.yaml)"}
    save("ws4sqlite", out)


# ================================================================= POCKETBASE
def run_pocketbase():
    pb = BIN["pocketbase"]
    out = {"tool": "PocketBase", "kind": ["rest"], "version": version_of([pb, "--version"])}
    d = os.path.join(WORK, "pb_data")
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(d)
    target = os.path.join(d, "data.db")
    shutil.copyfile(CHINOOK, target)
    before = set(r[0] for r in sqlite3.connect(target).execute("SELECT name FROM sqlite_master WHERE type='table'"))
    port = free_port()
    srv = Proc("pocketbase", [pb, "serve", "--dir", d, "--http", f"127.0.0.1:{port}"])
    base = f"http://127.0.0.1:{port}"
    try:
        ok, _ = wait_for(lambda: (http("GET", f"{base}/api/health")[0] == 200, None), 60)
        out["started"] = ok
        st, _, body = http("GET", f"{base}/api/collections/Artist/records")
        out["GET /api/collections/Artist/records"] = {"http_status": st, "body": short(body, 200)}
    finally:
        srv.stop()
    after = set(r[0] for r in sqlite3.connect(target).execute("SELECT name FROM sqlite_master WHERE type='table'"))
    out["tables_added_to_file_by_pocketbase"] = sorted(after - before)
    out["log_tail"] = srv.tail(15)
    out["conclusion"] = ("Chinook copied in as pb_data/data.db: PocketBase started and ran its own migrations into the file, "
                         "but the existing Chinook tables are not exposed (they are not PocketBase collections). "
                         "Serving them would require re-creating them as collections (or SQL view collections) via its admin/API.")
    save("pocketbase", out)


TOOLS = {"faucet": run_faucet, "dbhub": run_dbhub, "toolbox": run_toolbox, "execauto": run_execauto,
         "datasette": run_datasette, "sqlite_rest": run_sqlite_rest, "ws4sqlite": run_ws4sqlite,
         "pocketbase": run_pocketbase}


def main():
    names = sys.argv[1:] or list(TOOLS)
    meta = {"fixture": {"url": "https://github.com/lerocha/chinook-database/releases/download/v1.4.5/Chinook_Sqlite.sqlite",
                        "sha256": subprocess.run(["sha256sum", CHINOOK], capture_output=True, text=True).stdout.split()[0]},
            "work_dir": WORK, "node": version_of(["node", "--version"]), "python": sys.version.split()[0],
            "go": version_of(["go", "version"]), "uname": version_of(["uname", "-srm"]),
            "nproc": os.cpu_count()}
    with open(os.path.join(RESULTS, "environment.json"), "w") as f:
        json.dump(meta, f, indent=2)
    failed = []
    for n in names:
        print(f"=== {n}")
        try:
            TOOLS[n]()
        except Exception:
            traceback.print_exc()
            failed.append(n)
    if failed:
        print("FAILED:", failed)
        sys.exit(1)


if __name__ == "__main__":
    main()
