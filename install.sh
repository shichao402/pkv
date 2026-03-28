#!/usr/bin/env bash
set -euo pipefail

REPO="shichao402/pkv"
INSTALL_DIR="${HOME}/.local/bin"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"

    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      error "Unsupported OS: $OS" ;;
    esac

    case "$ARCH" in
        x86_64|amd64)  ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *)             error "Unsupported architecture: $ARCH" ;;
    esac
}

get_latest_version() {
    info "Fetching latest release..."
    LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"

    if command -v curl &>/dev/null; then
        RELEASE_JSON=$(curl -fsSL "$LATEST_URL")
    elif command -v wget &>/dev/null; then
        RELEASE_JSON=$(wget -qO- "$LATEST_URL")
    else
        error "curl or wget is required"
    fi

    VERSION=$(echo "$RELEASE_JSON" | grep '"tag_name"' | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/')
    if [ -z "$VERSION" ]; then
        error "Failed to determine latest version"
    fi
    info "Latest version: ${VERSION}"
}

download_binary() {
    ASSET_NAME="pkv_${OS}_${ARCH}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"

    info "Downloading ${ASSET_NAME}..."

    TMP_FILE=$(mktemp)
    trap 'rm -f "$TMP_FILE"' EXIT

    if command -v curl &>/dev/null; then
        curl -fsSL -o "$TMP_FILE" "$DOWNLOAD_URL" || error "Download failed. Check if release asset exists: ${ASSET_NAME}"
    else
        wget -qO "$TMP_FILE" "$DOWNLOAD_URL" || error "Download failed. Check if release asset exists: ${ASSET_NAME}"
    fi

    chmod +x "$TMP_FILE"
}

install_binary() {
    mkdir -p "$INSTALL_DIR"
    mv "$TMP_FILE" "${INSTALL_DIR}/pkv"
    # Prevent EXIT trap from removing the installed file
    trap - EXIT
    info "Installed pkv to ${INSTALL_DIR}/pkv"
}

check_path() {
    if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
        warn "${INSTALL_DIR} is not in your PATH."
        echo ""
        echo "Add the following to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
        echo ""
        echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        echo ""
    fi
}

main() {
    echo "=== PKV Installer ==="
    echo ""

    detect_platform
    info "Platform: ${OS}/${ARCH}"

    get_latest_version
    download_binary
    install_binary
    check_path

    echo ""
    info "Done! Run 'pkv --version' to verify."
}

main "$@"
