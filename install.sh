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
    RELEASES_URL="https://github.com/${REPO}/releases/latest"

    if command -v curl &>/dev/null; then
        REDIRECT_URL=$(curl -sI -o /dev/null -w '%{redirect_url}' "$RELEASES_URL")
    elif command -v wget &>/dev/null; then
        REDIRECT_URL=$(wget --spider -S "$RELEASES_URL" 2>&1 | grep -i 'Location:' | tail -1 | awk '{print $2}' | tr -d '\r')
    else
        error "curl or wget is required"
    fi

    VERSION="${REDIRECT_URL##*/}"
    if [ -z "$VERSION" ]; then
        error "Failed to determine latest version"
    fi
    info "Latest version: ${VERSION}"
}

download_binary() {
    ASSET_NAME="pkv_${OS}_${ARCH}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"
    CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.sha256"

    info "Downloading ${ASSET_NAME}..."

    TMP_FILE=$(mktemp)
    CHECKSUMS_FILE=$(mktemp)
    trap 'rm -f "$TMP_FILE" "$CHECKSUMS_FILE"' EXIT

    if command -v curl &>/dev/null; then
        curl -fsSL -o "$CHECKSUMS_FILE" "$CHECKSUMS_URL" || error "Failed to download checksums file"
        curl -fsSL -o "$TMP_FILE" "$DOWNLOAD_URL" || error "Download failed. Check if release asset exists: ${ASSET_NAME}"
    else
        wget -qO "$CHECKSUMS_FILE" "$CHECKSUMS_URL" || error "Failed to download checksums file"
        wget -qO "$TMP_FILE" "$DOWNLOAD_URL" || error "Download failed. Check if release asset exists: ${ASSET_NAME}"
    fi

    chmod +x "$TMP_FILE"
}

verify_checksum() {
    info "Verifying checksum..."
    EXPECTED_HASH=$(grep "${ASSET_NAME}" "$CHECKSUMS_FILE" | awk '{print $1}')
    if [ -z "$EXPECTED_HASH" ]; then
        error "No checksum found for ${ASSET_NAME}"
    fi

    if command -v sha256sum &>/dev/null; then
        ACTUAL_HASH=$(sha256sum "$TMP_FILE" | awk '{print $1}')
    elif command -v shasum &>/dev/null; then
        ACTUAL_HASH=$(shasum -a 256 "$TMP_FILE" | awk '{print $1}')
    else
        error "sha256sum or shasum is required for checksum verification"
    fi

    if [ "$EXPECTED_HASH" != "$ACTUAL_HASH" ]; then
        error "Checksum verification failed!\n  Expected: ${EXPECTED_HASH}\n  Actual:   ${ACTUAL_HASH}"
    fi
    info "Checksum verified."
}

install_binary() {
    mkdir -p "$INSTALL_DIR"
    mv "$TMP_FILE" "${INSTALL_DIR}/pkv"
    # Prevent EXIT trap from removing the installed file
    trap 'rm -f "$CHECKSUMS_FILE"' EXIT

    # Remove macOS quarantine attribute
    if [ "$OS" = "darwin" ]; then
        xattr -cr "${INSTALL_DIR}/pkv" 2>/dev/null || true
    fi

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
    verify_checksum
    install_binary
    check_path

    echo ""
    info "Done! Run 'pkv --version' to verify."
}

main "$@"
