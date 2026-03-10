#!/usr/bin/env bash
set -euo pipefail

REPO="pablontiv/backscroll"
INSTALL_DIR="${BACKSCROLL_INSTALL_DIR:-${HOME}/.local/bin}"

main() {
    local os arch asset_name

    os="$(uname -s)"
    arch="$(uname -m)"

    case "${os}" in
        Linux)
            case "${arch}" in
                x86_64) asset_name="backscroll-linux-x86_64" ;;
                *) error "Unsupported Linux architecture: ${arch}. Only x86_64 is supported." ;;
            esac
            ;;
        Darwin)
            case "${arch}" in
                arm64|aarch64) asset_name="backscroll-macos-aarch64" ;;
                *) error "Unsupported macOS architecture: ${arch}. Only aarch64 (Apple Silicon) is supported." ;;
            esac
            ;;
        *)
            error "Unsupported operating system: ${os}. Only Linux and macOS are supported."
            ;;
    esac

    echo "Detected platform: ${os} ${arch}"
    echo "Asset: ${asset_name}"

    local tag_name download_url
    tag_name="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | head -1 | cut -d'"' -f4)"

    if [ -z "${tag_name}" ]; then
        error "Failed to determine latest release tag."
    fi

    echo "Latest release: ${tag_name}"

    download_url="https://github.com/${REPO}/releases/download/${tag_name}/${asset_name}"

    echo "Downloading ${download_url}..."
    mkdir -p "${INSTALL_DIR}"
    curl -fsSL "${download_url}" -o "${INSTALL_DIR}/backscroll"
    chmod +x "${INSTALL_DIR}/backscroll"

    echo ""
    echo "Installed backscroll to ${INSTALL_DIR}/backscroll"

    if command -v backscroll &>/dev/null; then
        echo "Version: $(backscroll --version)"
    else
        echo ""
        echo "Add ${INSTALL_DIR} to your PATH:"
        echo "  export PATH=\"${INSTALL_DIR}:\${PATH}\""
    fi
}

error() {
    echo "Error: $1" >&2
    exit 1
}

main "$@"
