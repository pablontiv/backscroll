#!/usr/bin/env bash
set -euo pipefail

REPO="pablontiv/backscroll"
INSTALL_DIR="${BACKSCROLL_INSTALL_DIR:-${HOME}/.local/bin}"
INPUT_PRESETS=("claude.inputs.toml")

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

    install_input_presets "${tag_name}"

    if command -v backscroll &>/dev/null; then
        echo "Version: $(backscroll --version)"
    else
        echo ""
        echo "Add ${INSTALL_DIR} to your PATH:"
        echo "  export PATH=\"${INSTALL_DIR}:\${PATH}\""
    fi
}

get_config_dir() {
    if [ -n "${BACKSCROLL_CONFIG_DIR:-}" ]; then
        echo "${BACKSCROLL_CONFIG_DIR}"
        return
    fi

    case "$(uname -s)" in
        Darwin) echo "${HOME}/Library/Application Support" ;;
        *) echo "${XDG_CONFIG_HOME:-${HOME}/.config}" ;;
    esac
}

get_local_inputs_dir() {
    if [ -n "${BACKSCROLL_INPUTS_SOURCE_DIR:-}" ]; then
        echo "${BACKSCROLL_INPUTS_SOURCE_DIR}"
        return
    fi

    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd)"
    if [ -n "${script_dir}" ]; then
        echo "${script_dir}/inputs"
    fi
}

install_input_presets() {
    local version="${1:-main}"
    local config_dir inputs_dir local_inputs_dir preset dest tmp raw_url

    config_dir="$(get_config_dir)"
    inputs_dir="${config_dir}/backscroll/inputs"
    local_inputs_dir="$(get_local_inputs_dir)"
    mkdir -p "${inputs_dir}"

    echo ""
    echo "Installing input presets to ${inputs_dir}"

    for preset in "${INPUT_PRESETS[@]}"; do
        dest="${inputs_dir}/${preset}"
        if [ -e "${dest}" ] && [ "${BACKSCROLL_FORCE_INPUTS:-0}" != "1" ]; then
            echo "${dest} exists, skipping"
            continue
        fi

        if [ -n "${local_inputs_dir}" ] && [ -f "${local_inputs_dir}/${preset}" ]; then
            cp "${local_inputs_dir}/${preset}" "${dest}"
        else
            tmp="${dest}.tmp"
            raw_url="https://raw.githubusercontent.com/${REPO}/${version}/inputs/${preset}"
            curl -fsSL "${raw_url}" -o "${tmp}"
            mv "${tmp}" "${dest}"
        fi
        echo "Installed ${preset}"
    done
}

error() {
    echo "Error: $1" >&2
    exit 1
}

main "$@"
