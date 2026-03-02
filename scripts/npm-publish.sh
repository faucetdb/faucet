#!/usr/bin/env bash
set -euo pipefail

# Publish Faucet npm packages for a given release version.
# Usage: ./scripts/npm-publish.sh v0.1.7
#
# Expects:
#   - GoReleaser has already run and produced dist/ with archives
#   - NPM_TOKEN environment variable is set
#   - npm is available

VERSION="${1:?Usage: npm-publish.sh <version>}"
# Strip leading 'v' for npm (v0.1.7 -> 0.1.7)
NPM_VERSION="${VERSION#v}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
NPM_DIR="$ROOT_DIR/npm"
DIST_DIR="$ROOT_DIR/dist"

echo "=== Publishing Faucet npm packages v${NPM_VERSION} ==="

# Configure npm auth
echo "//registry.npmjs.org/:_authToken=${NPM_TOKEN}" > ~/.npmrc

# Map GoReleaser archive names to npm package dirs
# GoReleaser names: faucet_{version}_{os}_{arch}.tar.gz (or .zip for windows)
declare -A PLATFORM_MAP=(
  ["linux_amd64"]="linux-x64"
  ["linux_arm64"]="linux-arm64"
  ["darwin_amd64"]="darwin-x64"
  ["darwin_arm64"]="darwin-arm64"
  ["windows_amd64"]="win32-x64"
  ["windows_arm64"]="win32-arm64"
)

# Extract binaries from GoReleaser archives into npm package dirs
for goreleaser_key in "${!PLATFORM_MAP[@]}"; do
  npm_pkg="${PLATFORM_MAP[$goreleaser_key]}"
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
  # Use node to update version (portable across platforms)
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
done

# Publish main package last (depends on platform packages)
echo "Publishing @faucetdb/faucet@${NPM_VERSION}"
cd "$NPM_DIR/faucet"
npm publish --access public
cd "$ROOT_DIR"

# Clean up extracted binaries (don't commit them)
for npm_pkg in linux-x64 linux-arm64 darwin-x64 darwin-arm64 win32-x64 win32-arm64; do
  rm -rf "$NPM_DIR/$npm_pkg/bin"
done

# Clean up npmrc
rm -f ~/.npmrc

echo "=== Done! Published @faucetdb/faucet@${NPM_VERSION} ==="
echo "Install: npx @faucetdb/faucet --help"
