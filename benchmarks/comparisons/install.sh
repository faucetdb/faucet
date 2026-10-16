#!/usr/bin/env bash
# Downloads every tool into its own directory under $HARNESS_DOWNLOADS, then
# times the install steps that do NOT need the network (build / extract /
# offline install from an already-downloaded package). Writes
# results/install_times.json.
#
# Usage: HARNESS_DOWNLOADS=/some/empty/dir FAUCET_SRC=/path/to/faucet ./install.sh
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
DL="${HARNESS_DOWNLOADS:?set HARNESS_DOWNLOADS}"
FAUCET_SRC="${FAUCET_SRC:-$(cd "$HERE/../.." && pwd)}"
RES="$HERE/results"
mkdir -p "$RES" "$DL"/{chinook,faucet,dbhub,toolbox,execauto,datasette,sqlite-rest,ws4sqlite,pocketbase}

CHINOOK_URL="https://github.com/lerocha/chinook-database/releases/download/v1.4.5/Chinook_Sqlite.sqlite"
CHINOOK_SHA="bdf635be69850bd3be09c9a2dbeef7ddfb80036bd3ef3381383cd03b61e4a61a"
TOOLBOX_URL="https://storage.googleapis.com/mcp-toolbox-for-databases/v1.14.0/linux/amd64/toolbox"
WS4_URL="https://github.com/proofrock/ws4sqlite/releases/download/v0.16.6/ws4sqlite-v0.16.6-linux-amd64.tar.gz"
PB_URL="https://github.com/pocketbase/pocketbase/releases/download/v0.40.5/pocketbase_0.40.5_linux_amd64.zip"
DBHUB_PKG="@bytebase/dbhub@1.5.0"
EA_PKG="@executeautomation/database-server@1.1.0"
DATASETTE_PKG="datasette==0.65.5"
SQLITE_REST_MOD="github.com/b4fun/sqlite-rest@v0.4.1"

now() { date +%s.%N; }
dt() { python3 -I -c "import sys;print(round(float(sys.argv[2])-float(sys.argv[1]),2))" "$1" "$2"; }
declare -A T

# ---------- downloads (not timed) ----------
[ -f "$DL/chinook/Chinook_Sqlite.sqlite" ] || curl -sSL -o "$DL/chinook/Chinook_Sqlite.sqlite" "$CHINOOK_URL"
echo "$CHINOOK_SHA  $DL/chinook/Chinook_Sqlite.sqlite" | sha256sum -c -
[ -f "$DL/toolbox/toolbox.download" ] || curl -sSL -o "$DL/toolbox/toolbox.download" "$TOOLBOX_URL"
[ -f "$DL/ws4sqlite/ws4sqlite.tar.gz" ] || curl -sSL -o "$DL/ws4sqlite/ws4sqlite.tar.gz" "$WS4_URL"
[ -f "$DL/pocketbase/pocketbase.zip" ] || curl -sSL -o "$DL/pocketbase/pocketbase.zip" "$PB_URL"
mkdir -p "$DL/datasette/wheels"
python3 -m pip download -q -d "$DL/datasette/wheels" "$DATASETTE_PKG"
(cd /tmp && GOFLAGS=-modcacherw go mod download -x "$SQLITE_REST_MOD" >/dev/null 2>&1 || true)
# npm: a first install fills the npm cache; the timed install below is --offline.
for d in dbhub execauto; do
  pkg=$DBHUB_PKG; [ $d = execauto ] && pkg=$EA_PKG
  mkdir -p "$DL/$d/warm" && echo '{"private":true}' > "$DL/$d/warm/package.json" && (cd "$DL/$d/warm" && npm i --silent --no-audit --no-fund "$pkg" >/dev/null 2>&1)
done
(cd "$FAUCET_SRC" && go mod download)

# ---------- timed installs (no network) ----------
# Faucet: go build with an empty build cache (module cache warm).
rm -rf "$DL/faucet/gocache" "$DL/sqlite-rest/gocache"
t0=$(now); (cd "$FAUCET_SRC" && GOCACHE="$DL/faucet/gocache" CGO_ENABLED=0 go build -o "$DL/faucet/faucet" ./cmd/faucet); t1=$(now); T[faucet]=$(dt $t0 $t1)
# sqlite-rest: go install (cgo, mattn/go-sqlite3) with an empty build cache.
t0=$(now); (cd /tmp && GOCACHE="$DL/sqlite-rest/gocache" GOBIN="$DL/sqlite-rest" GOFLAGS=-modcacherw go install "$SQLITE_REST_MOD"); t1=$(now); T[sqlite_rest]=$(dt $t0 $t1)
# Toolbox: single static binary, just chmod.
t0=$(now); cp "$DL/toolbox/toolbox.download" "$DL/toolbox/toolbox" && chmod +x "$DL/toolbox/toolbox"; t1=$(now); T[toolbox]=$(dt $t0 $t1)
# ws4sqlite: untar.
t0=$(now); tar xzf "$DL/ws4sqlite/ws4sqlite.tar.gz" -C "$DL/ws4sqlite"; t1=$(now); T[ws4sqlite]=$(dt $t0 $t1)
# PocketBase: unzip.
t0=$(now); (cd "$DL/pocketbase" && python3 -I -m zipfile -e pocketbase.zip x && mv -f x/pocketbase ./pocketbase && chmod +x pocketbase && rm -rf x); t1=$(now); T[pocketbase]=$(dt $t0 $t1)
# Datasette: fresh venv + offline pip install from downloaded wheels.
rm -rf "$DL/datasette/venv"
t0=$(now); python3 -m venv "$DL/datasette/venv" && "$DL/datasette/venv/bin/pip" install -q --no-index --find-links "$DL/datasette/wheels" "$DATASETTE_PKG"; t1=$(now); T[datasette]=$(dt $t0 $t1)
# npm packages: offline install from the warm npm cache into a fresh dir.
for d in dbhub execauto; do
  pkg=$DBHUB_PKG; [ $d = execauto ] && pkg=$EA_PKG
  rm -rf "$DL/$d/inst"; mkdir -p "$DL/$d/inst"; echo '{"private":true}' > "$DL/$d/inst/package.json"
  t0=$(now); (cd "$DL/$d/inst" && npm i --offline --silent --no-audit --no-fund "$pkg" >/dev/null 2>&1); t1=$(now); T[$d]=$(dt $t0 $t1)
done

python3 -I - "$RES/install_times.json" <<EOF
import json, sys
d = {
 "note": "Seconds for install steps only, network download excluded (packages/modules/archives fetched beforehand). Go builds use an empty GOCACHE but a warm module cache (go install may still make a small proxy metadata lookup); npm installs are --offline from a warm npm cache; pip installs are --no-index from downloaded wheels.",
 "faucet_go_build": ${T[faucet]},
 "sqlite_rest_go_install_cgo": ${T[sqlite_rest]},
 "toolbox_chmod": ${T[toolbox]},
 "ws4sqlite_untar": ${T[ws4sqlite]},
 "pocketbase_unzip": ${T[pocketbase]},
 "datasette_venv_plus_pip_offline": ${T[datasette]},
 "dbhub_npm_install_offline": ${T[dbhub]},
 "execauto_npm_install_offline": ${T[execauto]},
}
json.dump(d, open(sys.argv[1], "w"), indent=2)
print(json.dumps(d, indent=2))
EOF
