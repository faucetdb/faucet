# Hands-on comparison: Faucet vs other SQLite MCP servers and REST layers

This is a reproducible harness. It points Faucet and seven other open-source tools at the same Chinook SQLite file and records what each one does by default and in its read-only configuration. Anything we did not measure says "not measured".

- **Machine:** Linux 6.11.0-29-generic x86_64, 8 vCPU, 15 GB RAM. No Docker. Go 1.25.9, Node v22.18.0, Python 3.12.7.
- **Fixture:** Chinook 1.4.5, `https://github.com/lerocha/chinook-database/releases/download/v1.4.5/Chinook_Sqlite.sqlite`, sha256 `bdf635be69850bd3be09c9a2dbeef7ddfb80036bd3ef3381383cd03b61e4a61a` (11 tables; `InvoiceLine` has 2240 rows). Each tool and each mode gets its own fresh copy.

## Tools and versions

| Tool | Version | How installed |
|---|---|---|
| Faucet | `git describe`: v0.1.15-4-g2327d63 (binary reports `dev` because it was built without ldflags) | `go build ./cmd/faucet` from this repo |
| DBHub | 1.5.0 | `npm i @bytebase/dbhub@1.5.0` |
| MCP Toolbox for Databases (googleapis/mcp-toolbox, formerly genai-toolbox) | 1.14.0 | release binary from `storage.googleapis.com/mcp-toolbox-for-databases/v1.14.0/linux/amd64/toolbox` |
| ExecuteAutomation database-server (multi-DB MCP server: SQLite, SQL Server, Postgres, MySQL) | 1.1.0 | `npm i @executeautomation/database-server@1.1.0` |
| Datasette | 0.65.5 (stable; the 1.0 alphas were not tested) | `pip install datasette==0.65.5` in a venv |
| sqlite-rest (b4fun) | v0.4.1 | `go install github.com/b4fun/sqlite-rest@v0.4.1` (the project publishes no release binaries; needs cgo) |
| ws4sqlite | v0.16.6 | release tarball |
| PocketBase | 0.40.5 | release zip |

## Files

| File | Purpose |
|---|---|
| `install.sh` | Downloads everything into `$HARNESS_DOWNLOADS/<tool>/`, which is untimed, then times only the install steps that need no network. Writes `results/install_times.json`. |
| `mcpclient.py` | A small stdlib-only MCP client: JSON-RPC 2.0 over stdio and over Streamable HTTP (SSE or JSON responses, `Mcp-Session-Id`). Protocol version is `2025-06-18`. |
| `run_all.py` | The main harness. It writes `results/<tool>.json`, and `results/raw/` holds the full initialize and tools/list responses plus Faucet's OpenAPI document. |
| `coldstart.py` | Runs "process launch → first successful query" 5 times per tool. Writes `results/coldstart.json`. |
| `summarize.py` | Prints the result tables from `results/*.json`. |

## Reproduce

```bash
export HARNESS_DOWNLOADS=/some/empty/dir     # third-party downloads, one dir per tool
export FAUCET_SRC=/path/to/faucet            # this repo
export HARNESS_WORK=/some/scratch/dir        # DB copies, data dirs, server logs
./install.sh
python3 -I run_all.py            # or a subset: faucet dbhub toolbox execauto datasette sqlite_rest ws4sqlite pocketbase
python3 -I coldstart.py 5
python3 -I summarize.py
```

All servers bind to 127.0.0.1 on a free port, and every process is stopped when its section finishes.

## Method

1. **Install time.** `install.sh` downloads packages first and does not time that. It then times what is left: `go build` with an empty `GOCACHE` (the module cache is warm), an offline `npm i` from a warm npm cache, `pip install --no-index` from wheels already on disk, and untar, unzip or chmod.
2. **Cold start.** The clock starts at process launch and stops at the first successful query. Polling runs every 20 ms. The table reports the median of 5 runs, and the samples are in `results/coldstart.json`. For Faucet, the CLI setup (`db add`, `role create`, `key create`) runs before launch. Those three commands took about 0.07 s in total (0.026 + 0.021 + 0.025 s) (see `results/faucet.json` → `cold_start.steps`).
3. **Config for read-only.** We use each tool's documented read-only mechanism, or say that it has none.
4. **MCP tools/list.** We count the tools and measure the compact JSON of the `result` object (`{"tools":[...]}`) in bytes. The token figure is **an estimate**: bytes / 4. For every server tested, tool definitions do not depend on table count, so the 11 Chinook tables do not change these numbers.
5. **Read-only enforcement.** We send INSERT (`Artist`), UPDATE (`Artist`) and DELETE (`InvoiceLine`) through every write-capable path: CRUD tools, raw-SQL tools and REST verbs. For raw-SQL paths we also try a CTE-prefixed DELETE (`WITH x AS (SELECT 1) DELETE ...`), which is valid SQLite but does not start with `DELETE`. Each attempt uses its own row ids. The verdict comes from **reading the file back** through a separate `mode=ro` connection, not from the server's reply. The server's reply is still recorded.
6. **Auth.** We send a request with no credentials to the data endpoint and the MCP endpoint, using each tool's defaults.
7. **Row cap.** We run `SELECT * FROM InvoiceLine` (2240 rows), or the REST equivalent with no limit parameter.
8. **OpenAPI.** We check the documented or common paths.

## Results

Raw output goes to `results/` (gitignored). `python3 -I summarize.py` prints the cold-start, MCP and REST tables used in the faucetdb.ai comparison posts.

## Not measured

- Throughput and latency under load.
- Memory use.
- Postgres, MySQL or any engine other than SQLite.
- Toolbox's `--enable-api` endpoint and OpenAPI.
- Datasette 1.0 alpha write API.
- PocketBase view collections.
- Installing Faucet from release binaries.
- MCP servers not listed above.
- Cold starts on a cold OS page cache. Every run here had the binaries already in the page cache.
