#!/bin/sh
set -e

REPO="faucetdb/faucet"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="faucet"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log()   { printf "\033[1;34m==>\033[0m %s\n" "$*"; }
err()   { printf "\033[1;31merror:\033[0m %s\n" "$*" >&2; exit 1; }

need_cmd() {
    if ! command -v "$1" > /dev/null 2>&1; then
        err "need '$1' (command not found)"
    fi
}

# ---------------------------------------------------------------------------
# Detect platform
# ---------------------------------------------------------------------------

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux"  ;;
        Darwin*) echo "darwin" ;;
        *)       err "unsupported OS: $(uname -s)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64"  ;;
        aarch64|arm64)  echo "arm64"  ;;
        *)              err "unsupported architecture: $(uname -m)" ;;
    esac
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
    need_cmd curl
    need_cmd tar
    need_cmd uname

    OS="$(detect_os)"
    ARCH="$(detect_arch)"

    log "Detected platform: ${OS}/${ARCH}"

    # Fetch latest release tag from GitHub API
    log "Fetching latest release from github.com/${REPO} ..."
    LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
    RELEASE_JSON="$(curl -sSL "$LATEST_URL")" || err "failed to fetch latest release info"

    TAG="$(printf '%s' "$RELEASE_JSON" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')"
    if [ -z "$TAG" ]; then
        err "could not determine latest release tag"
    fi
    log "Latest release: ${TAG}"

    # Build the expected asset name
    # Convention: faucet_<version>_<os>_<arch>.tar.gz  (strip leading 'v' for version)
    VERSION="${TAG#v}"
    ASSET_NAME="${BINARY_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET_NAME}"

    # Download
    TMP_DIR="$(mktemp -d)"
    trap 'rm -rf "$TMP_DIR"' EXIT

    log "Downloading ${DOWNLOAD_URL} ..."
    HTTP_CODE="$(curl -sSL -w "%{http_code}" -o "${TMP_DIR}/${ASSET_NAME}" "$DOWNLOAD_URL")"
    if [ "$HTTP_CODE" != "200" ]; then
        err "download failed (HTTP ${HTTP_CODE}). Asset may not exist for this platform."
    fi

    # Extract
    log "Extracting ${ASSET_NAME} ..."
    tar -xzf "${TMP_DIR}/${ASSET_NAME}" -C "$TMP_DIR"

    # Locate the binary inside the extracted contents
    if [ -f "${TMP_DIR}/${BINARY_NAME}" ]; then
        BIN_PATH="${TMP_DIR}/${BINARY_NAME}"
    elif [ -f "${TMP_DIR}/${BINARY_NAME}_${VERSION}_${OS}_${ARCH}/${BINARY_NAME}" ]; then
        BIN_PATH="${TMP_DIR}/${BINARY_NAME}_${VERSION}_${OS}_${ARCH}/${BINARY_NAME}"
    else
        # Search for it
        BIN_PATH="$(find "$TMP_DIR" -name "$BINARY_NAME" -type f | head -1)"
        if [ -z "$BIN_PATH" ]; then
            err "could not find '${BINARY_NAME}' binary in the archive"
        fi
    fi

    chmod +x "$BIN_PATH"

    # Install
    log "Installing ${BINARY_NAME} to ${INSTALL_DIR} ..."
    if [ -w "$INSTALL_DIR" ]; then
        mv "$BIN_PATH" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        sudo mv "$BIN_PATH" "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    # Verify
    INSTALLED_VERSION="$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || true)"
    if [ -n "$INSTALLED_VERSION" ]; then
        log "Installed: ${INSTALLED_VERSION}"
    else
        log "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
    fi

    log "Done."
}

main "$@"
