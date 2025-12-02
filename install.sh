#!/bin/sh
# CapyCut Installation Script
# Supports: Linux, macOS (both Intel and Apple Silicon)
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/harmonyvt/capycut/main/install.sh | sh
#   wget -qO- https://raw.githubusercontent.com/harmonyvt/capycut/main/install.sh | sh

set -e

# Configuration
REPO_OWNER="harmonyvt"
REPO_NAME="capycut"
BINARY_NAME="capycut"
GITHUB_API="https://api.github.com"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Print functions
info() {
    printf "${CYAN}[INFO]${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}[SUCCESS]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1"
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     OS="linux";;
        Darwin*)    OS="darwin";;
        CYGWIN*|MINGW*|MSYS*) OS="windows";;
        *)          error "Unsupported operating system: $(uname -s)";;
    esac
    echo "$OS"
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   ARCH="amd64";;
        arm64|aarch64)  ARCH="arm64";;
        armv7l)         ARCH="arm";;
        i386|i686)      ARCH="386";;
        *)              error "Unsupported architecture: $(uname -m)";;
    esac
    echo "$ARCH"
}

# Get the latest release version from GitHub
get_latest_version() {
    if command -v curl > /dev/null 2>&1; then
        VERSION=$(curl -fsSL "$GITHUB_API/repos/$REPO_OWNER/$REPO_NAME/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    elif command -v wget > /dev/null 2>&1; then
        VERSION=$(wget -qO- "$GITHUB_API/repos/$REPO_OWNER/$REPO_NAME/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
    
    if [ -z "$VERSION" ]; then
        error "Failed to get latest version. Please check your internet connection."
    fi
    
    echo "$VERSION"
}

# Download file
download() {
    URL="$1"
    OUTPUT="$2"
    
    if command -v curl > /dev/null 2>&1; then
        curl -fsSL "$URL" -o "$OUTPUT"
    elif command -v wget > /dev/null 2>&1; then
        wget -q "$URL" -O "$OUTPUT"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Get install directory
get_install_dir() {
    # Check if running as root
    if [ "$(id -u)" = "0" ]; then
        echo "/usr/local/bin"
    else
        # Use local bin directory for non-root users
        LOCAL_BIN="$HOME/.local/bin"
        mkdir -p "$LOCAL_BIN"
        echo "$LOCAL_BIN"
    fi
}

# Check if directory is in PATH
check_path() {
    INSTALL_DIR="$1"
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) return 0;;
        *) return 1;;
    esac
}

# Main installation function
main() {
    printf "\n"
    printf "${CYAN}  🦫 CapyCut Installer${NC}\n"
    printf "  ────────────────────\n\n"
    
    # Detect platform
    OS=$(detect_os)
    ARCH=$(detect_arch)
    info "Detected platform: $OS/$ARCH"
    
    # Get latest version
    info "Fetching latest version..."
    VERSION=$(get_latest_version)
    VERSION_NUM="${VERSION#v}"  # Remove 'v' prefix if present
    info "Latest version: $VERSION"
    
    # Determine archive extension
    if [ "$OS" = "windows" ]; then
        EXT="zip"
    else
        EXT="tar.gz"
    fi
    
    # Construct download URL
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS}_${ARCH}.${EXT}"
    DOWNLOAD_URL="https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/$VERSION/$ARCHIVE_NAME"
    
    info "Downloading $ARCHIVE_NAME..."
    
    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT
    
    # Download archive
    download "$DOWNLOAD_URL" "$TMP_DIR/$ARCHIVE_NAME"
    
    # Extract archive
    info "Extracting..."
    cd "$TMP_DIR"
    if [ "$EXT" = "zip" ]; then
        unzip -q "$ARCHIVE_NAME"
    else
        tar -xzf "$ARCHIVE_NAME"
    fi
    
    # Find and install binary
    INSTALL_DIR=$(get_install_dir)
    info "Installing to $INSTALL_DIR..."
    
    # The binary should be in the extracted directory or directly extracted
    if [ -f "$BINARY_NAME" ]; then
        mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
    elif [ -f "${BINARY_NAME}_${VERSION_NUM}_${OS}_${ARCH}/$BINARY_NAME" ]; then
        mv "${BINARY_NAME}_${VERSION_NUM}_${OS}_${ARCH}/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
    else
        # Search for the binary
        FOUND_BINARY=$(find . -name "$BINARY_NAME" -type f | head -1)
        if [ -n "$FOUND_BINARY" ]; then
            mv "$FOUND_BINARY" "$INSTALL_DIR/$BINARY_NAME"
        else
            error "Could not find $BINARY_NAME binary in archive"
        fi
    fi
    
    # Make executable
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    
    printf "\n"
    success "CapyCut $VERSION installed successfully!"
    printf "\n"
    
    # Check if install directory is in PATH
    if ! check_path "$INSTALL_DIR"; then
        warn "NOTE: $INSTALL_DIR is not in your PATH"
        printf "\n"
        printf "  Add it to your PATH by adding this to your shell profile:\n"
        printf "\n"
        printf "    ${CYAN}export PATH=\"\$PATH:$INSTALL_DIR\"${NC}\n"
        printf "\n"
        printf "  Or run capycut directly:\n"
        printf "\n"
        printf "    ${CYAN}$INSTALL_DIR/$BINARY_NAME${NC}\n"
    else
        printf "  Run ${CYAN}capycut --help${NC} to get started\n"
        printf "  Run ${CYAN}capycut --setup${NC} to configure your LLM provider\n"
    fi
    
    printf "\n"
    printf "  To update in the future, simply run: ${CYAN}capycut --update${NC}\n"
    printf "\n"
}

# Run main function
main
