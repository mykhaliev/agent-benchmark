#!/usr/bin/env bash

set -e

# Configuration
TOOL_NAME="agent-benchmark"
GITHUB_REPO="mykhaliev/agent-benchmark"
INSTALL_DIR="${HOME}/.local/bin"
USE_UPX=false  # Regular version by default

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
info() {
    echo -e "${GREEN}==>${NC} $1"
}

warn() {
    echo -e "${YELLOW}Warning:${NC} $1"
}

error() {
    echo -e "${RED}Error:${NC} $1"
    exit 1
}

# Detect OS and architecture
detect_platform() {
    local os arch

    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux*) os="linux" ;;
        darwin*) os="darwin" ;;
        msys*|mingw*|cygwin*) os="windows" ;;
        *) error "Unsupported operating system: $os" ;;
    esac

    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        armv7l) arch="armv7" ;;
        i386|i686) arch="386" ;;
        *) error "Unsupported architecture: $arch" ;;
    esac

    echo "${os}_${arch}"
}

# Get latest release version from GitHub
get_latest_version() {
    local version

    # Try to get the latest release (most recent non-prerelease)
    version=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    # If no "latest" release exists, get the most recent release from the list
    if [ -z "$version" ]; then
        version=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases" | grep '"tag_name":' | head -n 1 | sed -E 's/.*"([^"]+)".*/\1/')
    fi

    if [ -z "$version" ]; then
        error "Failed to fetch latest version. Please check your repository releases."
    fi

    echo "$version"
}

# Download and install binary
install_binary() {
    local version platform binary_name download_url

    platform=$(detect_platform)
    version=$(get_latest_version)

    # Check if already installed
    if [ -f "${INSTALL_DIR}/${TOOL_NAME}" ]; then
        info "Updating ${TOOL_NAME} to ${version}..."
    else
        info "Installing ${TOOL_NAME} ${version}..."
    fi

    # Construct download URL (adjust based on your release naming convention)
    binary_name="${TOOL_NAME}_${version}_${platform}"

    if [ "$USE_UPX" = true ]; then
        binary_name="${binary_name}_upx"
        info "Downloading UPX compressed version (smaller size)..."
    fi

    download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${binary_name}.tar.gz"

    # Create temporary directory
    tmp_dir=$(mktemp -d)
    trap "rm -rf $tmp_dir" EXIT

    # Download archive
    info "Downloading from ${download_url}..."
    if ! curl -fsSL "$download_url" -o "${tmp_dir}/${TOOL_NAME}.tar.gz"; then
        error "Failed to download binary"
    fi

    # Extract
    info "Extracting..."
    tar -xzf "${tmp_dir}/${TOOL_NAME}.tar.gz" -C "$tmp_dir"

    # Find the binary (could be agent-benchmark or agent-benchmark_upx)
    binary_file=$(find "$tmp_dir" -type f -name "${TOOL_NAME}*" ! -name "*.tar.gz" | head -n 1)

    if [ -z "$binary_file" ]; then
        error "Binary not found in archive"
    fi

    # Create install directory if it doesn't exist
    mkdir -p "$INSTALL_DIR"

    # Move binary to install directory
    mv "$binary_file" "${INSTALL_DIR}/${TOOL_NAME}"
    chmod +x "${INSTALL_DIR}/${TOOL_NAME}"

    if [ "$USE_UPX" = true ]; then
        info "Successfully installed ${TOOL_NAME} ${version} (UPX compressed)!"
    else
        info "Successfully installed ${TOOL_NAME} ${version}!"
    fi
}

# Add to PATH
setup_path() {
    local shell_config

    # Check if install directory is already in PATH
    if [[ ":$PATH:" == *":${INSTALL_DIR}:"* ]]; then
        return
    fi

    # Detect shell configuration file
    if [ -n "$BASH_VERSION" ]; then
        if [ -f "$HOME/.bashrc" ]; then
            shell_config="$HOME/.bashrc"
        elif [ -f "$HOME/.bash_profile" ]; then
            shell_config="$HOME/.bash_profile"
        fi
    elif [ -n "$ZSH_VERSION" ]; then
        shell_config="$HOME/.zshrc"
    elif [ -f "$HOME/.profile" ]; then
        shell_config="$HOME/.profile"
    fi

    # Add to PATH in shell config
    if [ -n "$shell_config" ]; then
        echo "" >> "$shell_config"
        echo "# Added by ${TOOL_NAME} installer" >> "$shell_config"
        echo "export PATH=\"\$PATH:${INSTALL_DIR}\"" >> "$shell_config"
        warn "Please restart your shell or run: source ${shell_config}"
    else
        warn "Could not detect shell configuration file."
        warn "Please manually add ${INSTALL_DIR} to your PATH."
    fi
}

# Main installation process
main() {
    # Check for required commands
    for cmd in curl tar; do
        if ! command -v "$cmd" &> /dev/null; then
            error "$cmd is required but not installed"
        fi
    done

    install_binary
    setup_path

    echo ""
    info "Installation complete!"
    info "Run '${TOOL_NAME} --version' to verify"
    echo ""
}

main "$@"