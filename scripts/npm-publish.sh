#!/usr/bin/env bash
set -euo pipefail

# Publish Faucet npm packages for a given release version.
# Usage: ./scripts/npm-publish.sh v0.1.8
#
# Expects:
#   - GoReleaser archives in dist/ OR will download from GitHub release
#   - NPM_TOKEN environment variable is set
#   - npm is available

VERSION="${1:?Usage: npm-publish.sh <version>}"
# Strip leading 'v' for npm (v0.1.8 -> 0.1.8)
NPM_VERSION="${VERSION#v}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
NPM_DIR="$ROOT_DIR/npm"
DIST_DIR="$ROOT_DIR/dist"

echo "=== Publishing Faucet npm packages v${NPM_VERSION} ==="

# If dist/ doesn't have the archives, download from GitHub release
SAMPLE_ARCHIVE="$DIST_DIR/faucet_${NPM_VERSION}_linux_amd64.tar.gz"
if [[ ! -f "$SAMPLE_ARCHIVE" ]]; then
  echo "Archives not found in dist/, downloading from GitHub release..."
  mkdir -p "$DIST_DIR"
  gh release download "$VERSION" --dir "$DIST_DIR" --pattern '*.tar.gz' --pattern '*.zip' --clobber
fi

# Configure npm auth.
# In CI, setup-node with registry-url creates .npmrc at NPM_CONFIG_USERCONFIG
# and uses NODE_AUTH_TOKEN. Only write our own .npmrc for local usage.
if [[ -z "${NPM_CONFIG_USERCONFIG:-}" ]]; then
  echo "//registry.npmjs.org/:_authToken=${NPM_TOKEN}" > ~/.npmrc
  trap 'rm -f ~/.npmrc' EXIT
fi

# Platform mappings (bash 3 compatible — no associative arrays)
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

# Extract binaries from GoReleaser archives into npm package dirs
for goreleaser_key in $GORELEASER_KEYS; do
  npm_pkg="$(npm_pkg_for "$goreleaser_key")"
  pkg_dir="$NPM_DIR/$npm_pkg"

  mkdir -p "$pkg_dir/bin"

  # Determine archive name and binary name
  if [[ "$goreleaser_key" == windows_* ]]; then
    archive="$DIST_DIR/faucet_${NPM_VERSION}_${goreleaser_key}.zip"
    binary="faucet.exe"
  else
    archive="$DIST_DIR/faucet_${NPM_VERSION}_${goreleaser_key}.tar.gz"
    binary="faucet"
  fi

  if [[ ! -f "$archive" ]]; then
    echo "WARNING: Archive not found: $archive (skipping $npm_pkg)"
    continue
  fi

  echo "Extracting $binary from $archive -> $pkg_dir/bin/"

  if [[ "$archive" == *.zip ]]; then
    unzip -o -j "$archive" "$binary" -d "$pkg_dir/bin/"
  else
    tar -xzf "$archive" -C "$pkg_dir/bin/" "$binary"
  fi

  chmod +x "$pkg_dir/bin/$binary"

  echo "Setting version $NPM_VERSION in $pkg_dir/package.json"
  node -e "
    const fs = require('fs');
    const pkg = JSON.parse(fs.readFileSync('$pkg_dir/package.json', 'utf8'));
    pkg.version = '$NPM_VERSION';
    fs.writeFileSync('$pkg_dir/package.json', JSON.stringify(pkg, null, 2) + '\n');
  "
done

# Update main package version and dependency versions
echo "Updating main @faucetdb/faucet package to v${NPM_VERSION}"
node -e "
  const fs = require('fs');
  const pkgPath = '$NPM_DIR/faucet/package.json';
  const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
  pkg.version = '$NPM_VERSION';
  for (const dep of Object.keys(pkg.optionalDependencies || {})) {
    pkg.optionalDependencies[dep] = '$NPM_VERSION';
  }
  fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n');
"

# Publish platform packages first
PUBLISHED_COUNT=0
for npm_pkg in linux-x64 linux-arm64 darwin-x64 darwin-arm64 win32-x64 win32-arm64; do
  pkg_dir="$NPM_DIR/$npm_pkg"
  if [[ ! -d "$pkg_dir/bin" ]] || [[ -z "$(ls -A "$pkg_dir/bin/" 2>/dev/null)" ]]; then
    echo "Skipping @faucetdb/$npm_pkg (no binary found)"
    continue
  fi
  echo "Publishing @faucetdb/$npm_pkg@${NPM_VERSION}"
  cd "$pkg_dir"
  npm publish --access public
  cd "$ROOT_DIR"
  PUBLISHED_COUNT=$((PUBLISHED_COUNT + 1))
done

if [[ "$PUBLISHED_COUNT" -eq 0 ]]; then
  echo "ERROR: No platform packages were published. Aborting."
  exit 1
fi

# Publish main package last (depends on platform packages)
echo "Publishing @faucetdb/faucet@${NPM_VERSION}"
cd "$NPM_DIR/faucet"
npm publish --access public
cd "$ROOT_DIR"

# Clean up extracted binaries (don't commit them)
for npm_pkg in linux-x64 linux-arm64 darwin-x64 darwin-arm64 win32-x64 win32-arm64; do
  rm -rf "$NPM_DIR/$npm_pkg/bin"
done

echo "=== Done! Published @faucetdb/faucet@${NPM_VERSION} ==="
echo "Install: npx @faucetdb/faucet@${NPM_VERSION} --help"
