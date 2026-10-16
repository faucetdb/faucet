#!/usr/bin/env python3
"""Repeat 'process launch -> first successful query' N times per tool (default 5).

Each sample: fresh DB copy, fresh process, poll every 20 ms until the first
query succeeds. Binaries/packages are already installed (see install.sh), so
this excludes download and install. Writes results/coldstart.json.

Run: HARNESS_DOWNLOADS=... python3 -I coldstart.py [N]
"""
import json
import os
import shutil
import statistics
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import mcpclient  # noqa: E402
import run_all as h  # noqa: E402

N = int(sys.argv[1]) if len(sys.argv) > 1 else 5
PROBE = h.PROBE_SELECT


def poll(fn, timeout=60):
    ok, _ = h.wait_for(fn, timeout=timeout, interval=0.02)
    return ok


def mcp_http_probe(url, tool, args, headers=None):
    def fn():
        c = mcpclient.HTTPClient(url, headers)
        c.initialize()
        return (not h.tool_text(c.call_tool(tool, args))[0]), None
    return fn


def sample_faucet_http():
    data = os.path.join(h.WORK, "cs-faucet")
    shutil.rmtree(data, ignore_errors=True)
    db = h.fresh_db("cs-faucet")
    fb = h.BIN["faucet"]
    env = {**os.environ, "FAUCET_TELEMETRY": "0"}
    subprocess.run([fb, "--data-dir", data, "db", "add", "--name", "chinook", "--driver", "sqlite", "--path", db],
                   capture_output=True, env=env, check=True)
    subprocess.run([fb, "--data-dir", data, "role", "create", "--name", "ro", "--verbs", "GET"],
                   capture_output=True, env=env, check=True)
    out = subprocess.run([fb, "--data-dir", data, "key", "create", "--role", "ro"], capture_output=True, text=True,
                         env=env, check=True).stdout
    key = next(t for t in out.split() if t.startswith("faucet_"))
    port = h.free_port()
    p = h.Proc("cs-faucet", [fb, "--data-dir", data, "serve", "--foreground", "--host", "127.0.0.1", "-p", str(port)],
               env={"FAUCET_TELEMETRY": "0"})
    try:
        ok = poll(lambda: (h.http("GET", f"http://127.0.0.1:{port}/api/v1/chinook/_table/Artist?limit=1",
                                  {"X-API-Key": key})[0] == 200, None))
        return p.elapsed() if ok else None
    finally:
        p.stop()


def sample_faucet_mcp_http():
    data = os.path.join(h.WORK, "cs-faucet")  # reuse the data dir from the previous sample
    fb = h.BIN["faucet"]
    out = subprocess.run([fb, "--data-dir", data, "key", "create", "--role", "ro"], capture_output=True, text=True,
                         env={**os.environ, "FAUCET_TELEMETRY": "0"}, check=True).stdout
    key = next(t for t in out.split() if t.startswith("faucet_"))
    port = h.free_port()
    p = h.Proc("cs-faucet-mcp", [fb, "--data-dir", data, "serve", "--foreground", "--host", "127.0.0.1", "-p", str(port)],
               env={"FAUCET_TELEMETRY": "0"})
    try:
        ok = poll(mcp_http_probe(f"http://127.0.0.1:{port}/mcp", "faucet_query",
                                 {"service": "chinook", "table": "Artist", "limit": 1}, {"X-API-Key": key}))
        return p.elapsed() if ok else None
    finally:
        p.stop()


def sample_stdio(argv, tool, args, env=None):
    t = time.monotonic()
    c = mcpclient.StdioClient(argv, env=env)
    try:
        c.initialize()
        r = c.call_tool(tool, args)
        return round(time.monotonic() - t, 3) if not h.tool_text(r)[0] else None
    finally:
        c.close()


def sample_faucet_stdio():
    data = os.path.join(h.WORK, "cs-faucet")
    return sample_stdio([h.BIN["faucet"], "--data-dir", data, "mcp"], "faucet_query",
                        {"service": "chinook", "table": "Artist", "limit": 1},
                        env={**os.environ, "FAUCET_TELEMETRY": "0"})


def sample_dbhub():
    db = h.fresh_db("cs-dbhub")
    port = h.free_port()
    p = h.Proc("cs-dbhub", ["node", h.BIN["dbhub"], "--transport", "http", "--port", str(port), "--dsn", f"sqlite://{db}"])
    try:
        return p.elapsed() if poll(mcp_http_probe(f"http://127.0.0.1:{port}/mcp", "execute_sql", {"sql": PROBE})) else None
    finally:
        p.stop()


def sample_toolbox():
    db = h.fresh_db("cs-toolbox")
    port = h.free_port()
    p = h.Proc("cs-toolbox", [h.BIN["toolbox"], "--prebuilt", "sqlite", "--port", str(port), "--disable-version-check"],
               env={"SQLITE_DATABASE": db})
    try:
        return p.elapsed() if poll(mcp_http_probe(f"http://127.0.0.1:{port}/mcp", "execute_sql", {"sql": PROBE})) else None
    finally:
        p.stop()


def sample_execauto():
    return sample_stdio(["node", h.BIN["execauto"], h.fresh_db("cs-ea")], "read_query", {"query": PROBE})


def sample_datasette():
    db = h.fresh_db("chinook")
    port = h.free_port()
    p = h.Proc("cs-datasette", [h.BIN["datasette"], "serve", db, "-h", "127.0.0.1", "-p", str(port)])
    try:
        return p.elapsed() if poll(lambda: (h.http("GET", f"http://127.0.0.1:{port}/chinook/Artist.json?_size=1")[0] == 200, None)) else None
    finally:
        p.stop()


def sample_sqlite_rest():
    db = h.fresh_db("cs-sr")
    tok = os.path.join(h.WORK, "cs-sr.token")
    open(tok, "wb").write(b"cs-secret-0123456789")
    hdr = {"Authorization": "Bearer " + h._jwt_hs256(b"cs-secret-0123456789")}
    port = h.free_port()
    p = h.Proc("cs-sr", [h.BIN["sqlite_rest"], "serve", "--db-dsn", db, "--http-addr", f"127.0.0.1:{port}",
                         "--metrics-addr", "", "--auth-token-file", tok, "--security-allow-table", "Artist"])
    try:
        return p.elapsed() if poll(lambda: (h.http("GET", f"http://127.0.0.1:{port}/Artist?limit=1", hdr)[0] == 200, None)) else None
    finally:
        p.stop()


def sample_ws4sqlite():
    d = os.path.join(h.WORK, "cs-ws4")
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(d)
    db = os.path.join(d, "chinook.db")
    shutil.copyfile(h.CHINOOK, db)
    port = h.free_port()
    p = h.Proc("cs-ws4", [h.BIN["ws4sqlite"], "--db", db, "--port", str(port), "--bind-host", "127.0.0.1"])
    try:
        return p.elapsed() if poll(lambda: (h.http("POST", f"http://127.0.0.1:{port}/chinook", None,
                                                   {"transaction": [{"query": PROBE}]})[0] == 200, None)) else None
    finally:
        p.stop()


SAMPLERS = [
    ("faucet_rest_http", "faucet serve launch -> first 200 REST GET with API key (CLI setup done before launch)", sample_faucet_http),
    ("faucet_mcp_http", "faucet serve launch -> first successful faucet_query over Streamable HTTP with API key", sample_faucet_mcp_http),
    ("faucet_mcp_stdio", "faucet mcp (stdio) launch -> initialize -> first successful faucet_query", sample_faucet_stdio),
    ("dbhub_mcp_http", "node dbhub --transport http launch -> first successful execute_sql SELECT", sample_dbhub),
    ("toolbox_mcp_http", "toolbox --prebuilt sqlite launch -> first successful execute_sql SELECT", sample_toolbox),
    ("execauto_mcp_stdio", "node database-server <db> (stdio) launch -> initialize -> first successful read_query", sample_execauto),
    ("datasette_rest", "datasette serve launch -> first 200 table JSON", sample_datasette),
    ("sqlite_rest_rest", "sqlite-rest serve launch -> first 200 GET with JWT", sample_sqlite_rest),
    ("ws4sqlite_rest", "ws4sqlite launch -> first 200 query", sample_ws4sqlite),
]


def main():
    out = {"samples_per_tool": N, "poll_interval_ms": 20,
           "note": "Seconds from process launch to first successful query; excludes download and install. Polling granularity 20 ms.",
           "results": {}}
    for name, definition, fn in SAMPLERS:
        samples = []
        for _ in range(N):
            samples.append(fn())
        ok = [s for s in samples if s is not None]
        out["results"][name] = {"definition": definition, "samples_s": samples,
                                "median_s": round(statistics.median(ok), 3) if ok else None,
                                "min_s": min(ok) if ok else None, "max_s": max(ok) if ok else None}
        print(name, out["results"][name]["median_s"], samples)
    out["measured_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    with open(os.path.join(h.RESULTS, "coldstart.json"), "w") as f:
        json.dump(out, f, indent=2)


if __name__ == "__main__":
    main()
