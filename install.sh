#!/usr/bin/env bash
set -euo pipefail

REPO="pablontiv/backscroll"
INSTALL_DIR="${BACKSCROLL_INSTALL_DIR:-${HOME}/.local/bin}"
main() {
	local os arch goos goarch asset_name

	os="$(uname -s)"
	arch="$(uname -m)"

	case "${os}" in
	Linux)
		goos="linux"
		case "${arch}" in
		x86_64) goarch="amd64" ;;
		*) error "Unsupported Linux architecture: ${arch}. Only x86_64 is supported." ;;
		esac
		;;
	Darwin)
		goos="darwin"
		case "${arch}" in
		arm64 | aarch64) goarch="arm64" ;;
		*) error "Unsupported macOS architecture: ${arch}. Only aarch64 (Apple Silicon) is supported." ;;
		esac
		;;
	*)
		error "Unsupported operating system: ${os}. Only Linux and macOS are supported."
		;;
	esac

	echo "Detected platform: ${os} ${arch}"

	local tag_name version download_url tmp_dir
	tag_name="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" |
		grep '"tag_name"' | head -1 | cut -d'"' -f4)"

	if [ -z "${tag_name}" ]; then
		error "Failed to determine latest release tag."
	fi

	version="${tag_name#v}"
	asset_name="backscroll_${version}_${goos}_${goarch}.tar.gz"
	download_url="https://github.com/${REPO}/releases/download/${tag_name}/${asset_name}"

	echo "Latest release: ${tag_name}"
	echo "Asset: ${asset_name}"
	echo "Downloading ${download_url}..."

	tmp_dir="$(mktemp -d)"
	trap 'rm -rf "${tmp_dir:-}"' EXIT

	curl -fsSL "${download_url}" -o "${tmp_dir}/asset.tar.gz"
	tar -xzf "${tmp_dir}/asset.tar.gz" -C "${tmp_dir}"

	if [ ! -f "${tmp_dir}/backscroll" ]; then
		error "Archive ${asset_name} did not contain a 'backscroll' binary."
	fi

	mkdir -p "${INSTALL_DIR}"
	mv "${tmp_dir}/backscroll" "${INSTALL_DIR}/backscroll"
	chmod +x "${INSTALL_DIR}/backscroll"

	echo ""
	echo "Installed backscroll to ${INSTALL_DIR}/backscroll"

	install_input_presets

	if command -v backscroll &>/dev/null; then
		echo "Version: $(backscroll --version)"
	else
		echo ""
		echo "Add ${INSTALL_DIR} to your PATH:"
		echo "  export PATH=\"${INSTALL_DIR}:\${PATH}\""
	fi
}

install_input_presets() {
	local -a args=(presets install)

	if [ -n "${BACKSCROLL_CONFIG_DIR:-}" ]; then
		args+=(--config-home "${BACKSCROLL_CONFIG_DIR}")
	fi
	if [ "${BACKSCROLL_FORCE_INPUTS:-0}" = "1" ]; then
		args+=(--overwrite)
	fi

	echo ""
	echo "Installing input presets with the native Backscroll command"
	"${INSTALL_DIR}/backscroll" "${args[@]}"
}

error() {
	echo "Error: $1" >&2
	exit 1
}

main "$@"
