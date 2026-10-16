#!/usr/bin/env python3
"""Print markdown tables from results/*.json (python3 -I summarize.py)."""
import json
import os

R = os.path.join(os.path.dirname(os.path.abspath(__file__)), "results")


def load(n):
    with open(os.path.join(R, n + ".json")) as f:
        return json.load(f)


def verdicts(block):
    if not block:
        return "n/a"
    a = block["attempts"]
    allowed = sum(1 for x in a if x["verdict"] == "ALLOWED")
    return f"{allowed}/{len(a)} writes landed"


inst = load("install_times")
f, dh, tb, ea = load("faucet"), load("dbhub"), load("toolbox"), load("execauto")
ds, sr, ws, pb = load("datasette"), load("sqlite_rest"), load("ws4sqlite"), load("pocketbase")

print("### MCP servers\n")
print("| Tool | Version | Install (s, no download) | Launch to first query (s) | tools/list: tools / bytes / ~tokens (bytes/4) | Writes, default config | Writes, read-only config | Unauth HTTP MCP | Row cap on SELECT * InvoiceLine (2240) |")
print("|---|---|---|---|---|---|---|---|---|")
fm = f["mcp_http_with_key"]
print(f"| Faucet | {f['git_describe']} | {inst['faucet_go_build']} (go build from source) | {f['cold_start']['seconds_serve_launch_to_first_query']} (HTTP, after 3 CLI setup cmds; total {f['cold_start']['seconds_total']}); stdio {f['cold_start_mcp_stdio_seconds']} | {fm['tool_count']} / {fm['tools_list_result_bytes']} / ~{fm['token_estimate_bytes_div_4']} | admin JWT: {verdicts(f['default_B0_admin_no_readonly_flag'])}; stdio: {verdicts(f['default_D_stdio_no_readonly_flag'])} | GET-only key: {verdicts(f['readonly_A_rbac_get_only_key'])}; read_only service (admin, HTTP): {verdicts(f['readonly_B_service_flag'])}; stdio: {verdicts(f['readonly_C_stdio_read_only_service'])} | {f['auth_unauthenticated']['mcp_initialize']['http_status']} | faucet_query: {f['row_cap']['mcp_faucet_query_no_limit']['rows']} default, {f['row_cap']['mcp_faucet_query_limit_5000']['rows']} max |")
d0, d1 = dh["mcp_tools_list_default"], dh["mcp_tools_list_readonly"]
print(f"| DBHub | 1.5.0 | {inst['dbhub_npm_install_offline']} (npm, offline) | {dh['cold_start']['seconds']} (HTTP) | {d0['tool_count']} / {d0['tools_list_result_bytes']} / ~{d0['token_estimate_bytes_div_4']} (read-only: {d1['tool_count']} / {d1['tools_list_result_bytes']} / ~{d1['token_estimate_bytes_div_4']}) | {verdicts(dh['default_writes'])} | {verdicts(dh['readonly'])} | {dh['auth_unauthenticated']['mcp']['http_status']} | {dh['row_cap']['execute_sql_select_star_InvoiceLine']['rows']} |")
t0 = tb["mcp_tools_list_default"]
print(f"| MCP Toolbox | 1.14.0 | {inst['toolbox_chmod']} (binary) | {tb['cold_start']['seconds']} (HTTP) | {t0['tool_count']} / {t0['tools_list_result_bytes']} / ~{t0['token_estimate_bytes_div_4']} | {verdicts(tb['writes_default'])} | mode=ro URI: {verdicts(tb['writes_ro_uri'])} | {tb['auth_unauthenticated']['mcp']['http_status']} | {tb['row_cap']['execute_sql_select_star_InvoiceLine']['rows']} |")
e0 = ea["mcp_tools_list"]
print(f"| ExecuteAutomation database-server | 1.1.0 | {inst['execauto_npm_install_offline']} (npm, offline) | {ea['cold_start']['seconds']} (stdio) | {e0['tool_count']} / {e0['tools_list_result_bytes']} / ~{e0['token_estimate_bytes_div_4']} | {verdicts(ea['default_writes'])} | no read-only mode | n/a (stdio only) | {ea['row_cap']['read_query_select_star_InvoiceLine']['rows']} |")

print("\n### REST over SQLite\n")
print("| Tool | Version | Install (s, no download) | Launch to first query (s) | Writes, default | Writes, read-only config | Unauth request | Default row cap (InvoiceLine, 2240) | OpenAPI |")
print("|---|---|---|---|---|---|---|---|---|")
print(f"| Faucet | {f['git_describe']} | {inst['faucet_go_build']} | {f['cold_start']['seconds_serve_launch_to_first_query']} | admin: see MCP table | GET-only key: {verdicts(f['readonly_A_rbac_get_only_key'])} (REST+MCP); read_only service: {verdicts(f['readonly_B_service_flag'])} | data API {f['auth_unauthenticated']['rest_table']['http_status']}, /openapi.json {f['auth_unauthenticated']['openapi_json']['http_status']} | {f['row_cap']['rest_no_limit_param']['rows']} default, {f['row_cap']['rest_limit_5000']['rows']} max | yes, {f['openapi']['openapi_version']}, {f['openapi']['path_count']} paths |")
print(f"| Datasette | 0.65.5 | {inst['datasette_venv_plus_pip_offline']} (venv + pip offline) | {ds['cold_start']['seconds']} | {verdicts(ds['default_writes'])} | (read-only by default) | {ds['auth_unauthenticated']['rest']['http_status']} | table JSON {ds['row_cap']['table_json_default_page']['rows']}/page; ?sql= {ds['row_cap']['arbitrary_sql_select_star']['rows']} (truncated={ds['row_cap']['arbitrary_sql_select_star']['truncated_flag']}) | no |")
print(f"| sqlite-rest | v0.4.1 | {inst['sqlite_rest_go_install_cgo']} (go install, cgo) | {sr['cold_start']['seconds']} | {verdicts(sr['writes_default'])} | mode=ro URI: {verdicts(sr['writes_ro_uri'])} | {sr['auth_unauthenticated']['rest']['http_status']} (auth flags mandatory) | {sr['row_cap']['GET /InvoiceLine']['rows']} | no |")
print(f"| ws4sqlite | v0.16.6 | {inst['ws4sqlite_untar']} (untar) | {ws['cold_start']['seconds']} | {verdicts(ws['writes_default'])} | readOnly: true: {verdicts(ws['writes_readonly'])} | {ws['auth_unauthenticated']['rest']['http_status']} | {ws['row_cap']['query SELECT * FROM InvoiceLine']['rows']} | no |")
print(f"| PocketBase | 0.40.5 | {inst['pocketbase_unzip']} (unzip) | did not serve Chinook tables | n/a | n/a | n/a | n/a | n/a |")
