#!/usr/bin/env bash
# test-mcp-e2e.sh — End-to-end MCP Streamable HTTP protocol test via curl
# Usage: ./scripts/test-mcp-e2e.sh [base_url] [jwt_token]
set -euo pipefail

BASE="${1:-http://127.0.0.1:8081}"
JWT="${2:-}"
PASS=0
FAIL=0

green() { printf '\033[32m%s\033[0m\n' "$1"; }
red()   { printf '\033[31m%s\033[0m\n' "$1"; }

assert_ok() {
    local label="$1" response="$2"
    if echo "$response" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'result' in d" 2>/dev/null; then
        green "  PASS: $label"
        PASS=$((PASS + 1))
    else
        red "  FAIL: $label"
        echo "  Response: $response"
        FAIL=$((FAIL + 1))
    fi
}

AUTH_HEADER=""
if [ -n "$JWT" ]; then
    AUTH_HEADER="Authorization: Bearer $JWT"
fi

echo "=== Faucet MCP E2E Test ==="
echo "Base URL: $BASE/mcp"
echo ""

# Step 1: Initialize
echo "Step 1: Initialize..."
INIT=$(curl -s -D /tmp/mcp-e2e-headers.txt -X POST \
    ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
    -H "Content-Type: application/json" \
    -H "Accept: application/json, text/event-stream" \
    -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e-test","version":"1.0.0"}}}' \
    "$BASE/mcp")
assert_ok "initialize" "$INIT"

SESSION=$(grep -i 'Mcp-Session-Id' /tmp/mcp-e2e-headers.txt 2>/dev/null | tr -d '\r' | awk '{print $2}')
echo "  Session: ${SESSION:-none}"

# Step 2: Initialized notification
echo "Step 2: Send initialized notification..."
curl -s -X POST \
    ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
    ${SESSION:+-H "Mcp-Session-Id: $SESSION"} \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
    "$BASE/mcp" > /dev/null
green "  PASS: notification sent"
PASS=$((PASS + 1))

# Step 3: List tools
echo "Step 3: List tools..."
TOOLS=$(curl -s -X POST \
    ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
    ${SESSION:+-H "Mcp-Session-Id: $SESSION"} \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
    "$BASE/mcp")
assert_ok "tools/list" "$TOOLS"

TOOL_COUNT=$(echo "$TOOLS" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['result']['tools']))" 2>/dev/null || echo 0)
echo "  Tools found: $TOOL_COUNT"

# Step 4: Call faucet_list_services
echo "Step 4: Call faucet_list_services..."
SERVICES=$(curl -s -X POST \
    ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
    ${SESSION:+-H "Mcp-Session-Id: $SESSION"} \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"faucet_list_services","arguments":{}}}' \
    "$BASE/mcp")
assert_ok "faucet_list_services" "$SERVICES"

# Step 5: List resources
echo "Step 5: List resources..."
RESOURCES=$(curl -s -X POST \
    ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
    ${SESSION:+-H "Mcp-Session-Id: $SESSION"} \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":4,"method":"resources/list"}' \
    "$BASE/mcp")
assert_ok "resources/list" "$RESOURCES"

# Step 6: Read resource
echo "Step 6: Read faucet://services resource..."
RES_READ=$(curl -s -X POST \
    ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
    ${SESSION:+-H "Mcp-Session-Id: $SESSION"} \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"faucet://services"}}' \
    "$BASE/mcp")
assert_ok "resources/read" "$RES_READ"

# Step 7: Ping
echo "Step 7: Ping..."
PING=$(curl -s -X POST \
    ${AUTH_HEADER:+-H "$AUTH_HEADER"} \
    ${SESSION:+-H "Mcp-Session-Id: $SESSION"} \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":6,"method":"ping"}' \
    "$BASE/mcp")
assert_ok "ping" "$PING"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ $FAIL -eq 0 ] && green "ALL TESTS PASSED" || red "SOME TESTS FAILED"
exit $FAIL
