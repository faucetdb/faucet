#!/usr/bin/env bash
set -euo pipefail

# Publish Faucet npm packages for a given release version.
#
# Usage:
#   ./scripts/npm-publish.sh v0.1.13
#   DRY_RUN=1 ./scripts/npm-publish.sh v0.1.13   # no publish, no auth preflight
#
# Inputs:
#   - GoReleaser archives in dist/ (downloaded from the GitHub release via `gh`
#     when missing; set GH_TOKEN for gh in CI)
#   - npm and node on PATH
#
# Authentication (decided here, never by ~/.npmrc):
#   - NPM_TOKEN set   -> classic/granular token, verified with `npm whoami`
#   - NPM_TOKEN empty -> npm trusted publishing (OIDC) from GitHub Actions;
#                        needs id-token: write, npm >= 11.5.1, no NODE_AUTH_TOKEN
#
# The script is re-runnable: package versions that already exist on the
# registry are skipped, so a partially failed run can simply be started again.

VERSION="${1:?Usage: npm-publish.sh <version> (e.g. v0.1.13)}"
if [[ ! "$VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+ ]]; then
  echo "ERROR: '$VERSION' is not a release version (expected vX.Y.Z)" >&2
  exit 1
fi
# Strip leading 'v' for npm (v0.1.13 -> 0.1.13)
NPM_VERSION="${VERSION#v}"
DRY_RUN="${DRY_RUN:-0}"
NPM_TOKEN="${NPM_TOKEN:-}"
REGISTRY_HOST="registry.npmjs.org"
REGISTRY_URL="https://${REGISTRY_HOST}/"
MIN_OIDC_NPM="11.5.1"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
NPM_DIR="$ROOT_DIR/npm"
DIST_DIR="$ROOT_DIR/dist"

PLATFORM_PKGS="linux-x64 linux-arm64 darwin-x64 darwin-arm64 win32-x64 win32-arm64"
# GoReleaser os_arch keys (bash 3 compatible: no associative arrays)
GORELEASER_KEYS="linux_amd64 linux_arm64 darwin_amd64 darwin_arm64 windows_amd64 windows_arm64"
npm_pkg_for() {
  case "$1" in
    linux_amd64)   echo "linux-x64" ;;
    linux_arm64)   echo "linux-arm64" ;;
    darwin_amd64)  echo "darwin-x64" ;;
    darwin_arm64)  echo "darwin-arm64" ;;
    windows_amd64) echo "win32-x64" ;;
    windows_arm64) echo "win32-arm64" ;;
  esac
}

log() { echo "[npm-publish] $*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

# version_ge A B -> true when A >= B (numeric major.minor.patch, prerelease ignored)
version_ge() {
  local a1 a2 a3 b1 b2 b3
  IFS=. read -r a1 a2 a3 <<< "${1%%-*}"
  IFS=. read -r b1 b2 b3 <<< "${2%%-*}"
  a1=${a1:-0}; a2=${a2:-0}; a3=${a3:-0}
  b1=${b1:-0}; b2=${b2:-0}; b3=${b3:-0}
  if [[ "$a1" -ne "$b1" ]]; then [[ "$a1" -gt "$b1" ]]; return; fi
  if [[ "$a2" -ne "$b2" ]]; then [[ "$a2" -gt "$b2" ]]; return; fi
  [[ "$a3" -ge "$b3" ]]
}

# ---------------------------------------------------------------------------
# Isolated npm userconfig: the runner's / developer's ~/.npmrc never interferes
# and it is removed on exit together with the extracted binaries.
# ---------------------------------------------------------------------------
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/faucet-npm-publish.XXXXXX")"
NPMRC="$WORK_DIR/npmrc"
export NPM_CONFIG_USERCONFIG="$NPMRC"

# In DRY_RUN mode the version rewrites are undone from pristine copies taken
# before the first edit (so uncommitted local changes survive, unlike a
# `git checkout`), leaving the tree exactly as it was.
BACKUP_DIR="$WORK_DIR/package-json"
mkdir -p "$BACKUP_DIR"
for p in $PLATFORM_PKGS faucet; do
  cp "$NPM_DIR/$p/package.json" "$BACKUP_DIR/$p.json"
done

cleanup() {
  local p
  for p in $PLATFORM_PKGS; do
    rm -rf "${NPM_DIR:?}/${p:?}/bin"
  done
  if [[ "$DRY_RUN" == "1" ]]; then
    for p in $PLATFORM_PKGS faucet; do
      [[ -f "$BACKUP_DIR/$p.json" ]] && cp "$BACKUP_DIR/$p.json" "$NPM_DIR/$p/package.json"
    done
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

# Only ever write literal values here: an .npmrc line such as ${NPM_TOKEN}
# referencing an unset variable makes npm abort ("Failed to replace env in config").
echo "registry=${REGISTRY_URL}" > "$NPMRC"

PUBLISH_FLAGS="--access public"

echo "=== Publishing Faucet npm packages v${NPM_VERSION} ==="

if [[ "$DRY_RUN" == "1" ]]; then
  PUBLISH_FLAGS="$PUBLISH_FLAGS --dry-run"
  echo
  echo "################################################################"
  echo "#  DRY RUN: nothing will be published (npm publish --dry-run)  #"
  echo "#  Auth preflight is skipped.                                  #"
  echo "################################################################"
  echo
  if [[ -n "$NPM_TOKEN" ]]; then
    echo "//${REGISTRY_HOST}/:_authToken=${NPM_TOKEN}" >> "$NPMRC"
  fi
elif [[ -n "$NPM_TOKEN" ]]; then
  # ----- Token mode ---------------------------------------------------------
  log "Auth mode: NPM_TOKEN"
  echo "//${REGISTRY_HOST}/:_authToken=${NPM_TOKEN}" >> "$NPMRC"
  if ! NPM_USER="$(npm whoami 2>/dev/null)" || [[ -z "$NPM_USER" ]]; then
    cat >&2 <<EOF

ERROR: npm rejected the NPM_TOKEN (npm whoami failed). No package was touched.

  npm granular access tokens expire after at most 90 days and classic tokens
  have been revoked, so an old NPM_TOKEN secret stops working silently. On a
  scoped package npm reports this as "404 Not Found - PUT .../@faucetdb%2f...".

Fix one of:

  1. Create a new granular access token on npmjs.com (Access Tokens ->
     Generate New Token -> Granular; packages & scopes: @faucetdb, permission
     read/write, bypass 2FA for automation) and store it in the repository
     secret NPM_TOKEN. Remember it expires again within 90 days.

  2. Preferred: switch to npm trusted publishing (OIDC, no secret at all).
     On npmjs.com open each of the 7 packages (@faucetdb/faucet, linux-x64,
     linux-arm64, darwin-x64, darwin-arm64, win32-x64, win32-arm64) ->
     Settings -> Trusted Publisher -> GitHub Actions with
       owner: faucetdb   repository: faucet   workflow file: npm-publish.yml
     then delete the NPM_TOKEN secret (or leave it empty) so this script
     uses OIDC mode.

EOF
    exit 1
  fi
  log "Authenticated to ${REGISTRY_HOST} as ${NPM_USER}"
else
  # ----- OIDC / trusted publishing mode --------------------------------------
  log "Auth mode: npm trusted publishing (OIDC)"
  if [[ -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" ]]; then
    cat >&2 <<EOF

ERROR: NPM_TOKEN is empty and no GitHub Actions OIDC context is available
(ACTIONS_ID_TOKEN_REQUEST_URL is not set). No package was touched.

Trusted publishing only works inside a GitHub Actions job that has
"permissions: id-token: write" (see .github/workflows/npm-publish.yml) and
whose workflow file is registered as trusted publisher for every @faucetdb
package on npmjs.com.

To publish from this machine instead, set NPM_TOKEN to a valid npm granular
access token, or use DRY_RUN=1 to exercise the pipeline without publishing.

EOF
    exit 1
  fi
  if [[ -n "${NODE_AUTH_TOKEN:-}" ]]; then
    die "NODE_AUTH_TOKEN is set; it would override OIDC authentication. Unset it (do not pass registry-url/NODE_AUTH_TOKEN to setup-node)."
  fi
  NPM_CLI_VERSION="$(npm --version)"
  if ! version_ge "$NPM_CLI_VERSION" "$MIN_OIDC_NPM"; then
    cat >&2 <<EOF

ERROR: npm ${NPM_CLI_VERSION} is too old for trusted publishing (need >= ${MIN_OIDC_NPM}).
Run "npm install -g npm@latest" before this script (the workflow does this).

EOF
    exit 1
  fi
  log "npm ${NPM_CLI_VERSION} OK (>= ${MIN_OIDC_NPM}); npm publish will exchange the OIDC token itself"
fi

# ---------------------------------------------------------------------------
# Archives: download from the GitHub release when dist/ lacks them
# ---------------------------------------------------------------------------
SAMPLE_ARCHIVE="$DIST_DIR/faucet_${NPM_VERSION}_linux_amd64.tar.gz"
if [[ ! -f "$SAMPLE_ARCHIVE" ]]; then
  log "Archives not found in dist/, downloading release ${VERSION} with gh..."
  mkdir -p "$DIST_DIR"
  gh release download "$VERSION" --dir "$DIST_DIR" --pattern '*.tar.gz' --pattern '*.zip' --clobber
fi

# ---------------------------------------------------------------------------
# Extract binaries into the platform packages and set their versions
# ---------------------------------------------------------------------------
for goreleaser_key in $GORELEASER_KEYS; do
  npm_pkg="$(npm_pkg_for "$goreleaser_key")"
  pkg_dir="$NPM_DIR/$npm_pkg"

  if [[ "$goreleaser_key" == windows_* ]]; then
    archive="$DIST_DIR/faucet_${NPM_VERSION}_${goreleaser_key}.zip"
    binary="faucet.exe"
  else
    archive="$DIST_DIR/faucet_${NPM_VERSION}_${goreleaser_key}.tar.gz"
    binary="faucet"
  fi

  if [[ ! -f "$archive" ]]; then
    log "WARNING: archive not found: $archive (skipping @faucetdb/$npm_pkg)"
    continue
  fi

  rm -rf "${pkg_dir:?}/bin"
  mkdir -p "$pkg_dir/bin"
  log "Extracting $binary from ${archive##*/} -> npm/$npm_pkg/bin/"
  if [[ "$archive" == *.zip ]]; then
    unzip -o -q -j "$archive" "$binary" -d "$pkg_dir/bin/"
  else
    tar -xzf "$archive" -C "$pkg_dir/bin/" "$binary"
  fi
  chmod +x "$pkg_dir/bin/$binary"

  log "Setting version $NPM_VERSION in npm/$npm_pkg/package.json"
  node -e '
    const fs = require("fs");
    const [file, version] = process.argv.slice(1);
    const pkg = JSON.parse(fs.readFileSync(file, "utf8"));
    pkg.version = version;
    fs.writeFileSync(file, JSON.stringify(pkg, null, 2) + "\n");
  ' "$pkg_dir/package.json" "$NPM_VERSION"
done

# Main package: version + pinned optionalDependencies
log "Setting version $NPM_VERSION (and optionalDependencies) in npm/faucet/package.json"
node -e '
  const fs = require("fs");
  const [file, version] = process.argv.slice(1);
  const pkg = JSON.parse(fs.readFileSync(file, "utf8"));
  pkg.version = version;
  for (const dep of Object.keys(pkg.optionalDependencies || {})) {
    pkg.optionalDependencies[dep] = version;
  }
  fs.writeFileSync(file, JSON.stringify(pkg, null, 2) + "\n");
' "$NPM_DIR/faucet/package.json" "$NPM_VERSION"

# ---------------------------------------------------------------------------
# Publish
# ---------------------------------------------------------------------------
already_published() {
  local existing
  existing="$(npm view "$1@$2" version 2>/dev/null || true)"
  [[ "$existing" == "$2" ]]
}

# publish_pkg <dir> <package name>
publish_pkg() {
  local dir="$1" name="$2"
  if already_published "$name" "$NPM_VERSION"; then
    log "${name}@${NPM_VERSION} already published, skipping"
    return 0
  fi
  if [[ "$DRY_RUN" == "1" ]]; then
    log "DRY RUN: would publish ${name}@${NPM_VERSION}"
  else
    log "Publishing ${name}@${NPM_VERSION}"
  fi
  # shellcheck disable=SC2086  # PUBLISH_FLAGS is intentionally word-split
  (cd "$dir" && npm publish $PUBLISH_FLAGS)
}

# Platform packages first: the main package depends on them
DONE_COUNT=0
for npm_pkg in $PLATFORM_PKGS; do
  pkg_dir="$NPM_DIR/$npm_pkg"
  if [[ ! -d "$pkg_dir/bin" ]] || [[ -z "$(ls -A "$pkg_dir/bin/" 2>/dev/null)" ]]; then
    log "Skipping @faucetdb/$npm_pkg (no binary found)"
    continue
  fi
  publish_pkg "$pkg_dir" "@faucetdb/$npm_pkg"
  DONE_COUNT=$((DONE_COUNT + 1))
done

if [[ "$DONE_COUNT" -eq 0 ]]; then
  die "no platform packages were published or found on the registry. Aborting before the main package."
fi

# Main package last
publish_pkg "$NPM_DIR/faucet" "@faucetdb/faucet"

echo
if [[ "$DRY_RUN" == "1" ]]; then
  echo "=== DRY RUN complete: ${DONE_COUNT} platform packages + @faucetdb/faucet@${NPM_VERSION} would be published ==="
else
  echo "=== Done! @faucetdb/faucet@${NPM_VERSION} and ${DONE_COUNT} platform packages are on npm ==="
  echo "Install: npx @faucetdb/faucet@${NPM_VERSION} --help"
fi
