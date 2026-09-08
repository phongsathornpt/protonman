#!/bin/sh
set -eu

REPOSITORY="phongsathornpt/protonman"
GITHUB_URL="https://github.com"
API_URL="https://api.github.com"
BINARY_NAME="protonman"
VERSION="${PROTONMAN_VERSION:-}"
BIN_DIR="${PROTONMAN_INSTALL_DIR:-${HOME:-}/.local/bin}"
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
TMP_DIR=""
INSTALL_TMP=""
RELEASE_JSON=""

usage() {
	cat <<'USAGE'
Install Protonman from GitHub Releases.

Usage:
  install.sh [--version v1.2.3] [--bin-dir PATH]

Options:
  --version VERSION  Install an exact release tag. Defaults to latest stable.
  --bin-dir PATH     Install directory. Defaults to $HOME/.local/bin.
  -h, --help         Show this help.

Environment:
  PROTONMAN_VERSION       Default release tag when --version is omitted.
  PROTONMAN_INSTALL_DIR   Default install directory when --bin-dir is omitted.
  GITHUB_TOKEN, GH_TOKEN  Optional token for private GitHub releases.
USAGE
}

die() {
	printf 'protonman installer: %s\n' "$*" >&2
	exit 1
}

cleanup() {
	if [ -n "$INSTALL_TMP" ]; then
		rm -f "$INSTALL_TMP"
	fi
	if [ -n "$TMP_DIR" ]; then
		rm -rf "$TMP_DIR"
	fi
}

trap cleanup EXIT HUP INT TERM

curl_request() {
	if [ -n "$TOKEN" ]; then
		curl -H "Authorization: Bearer $TOKEN" "$@"
	else
		curl "$@"
	fi
}

require_command() {
	command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

validate_version() {
	printf '%s\n' "$1" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$' ||
		die "invalid release version: $1"
}

github_api_json() {
	curl_request -fsSL --retry 3 --connect-timeout 10 \
		-H "Accept: application/vnd.github+json" "$1"
}

resolve_latest_version() {
	if [ -n "$TOKEN" ]; then
		latest_json=$(github_api_json "$API_URL/repos/$REPOSITORY/releases/latest") ||
			die "could not resolve the latest Protonman release"
		latest_version=$(printf '%s' "$latest_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
	else
		latest_url=$(curl_request -fsSL --retry 3 --connect-timeout 10 \
			-o /dev/null -w '%{url_effective}' "$GITHUB_URL/$REPOSITORY/releases/latest") ||
			die "could not resolve the latest Protonman release"
		latest_version=${latest_url##*/}
	fi
	[ -n "$latest_version" ] || die "latest Protonman release did not contain a version tag"
	validate_version "$latest_version"
	printf '%s\n' "$latest_version"
}

load_release_json() {
	[ -n "$TOKEN" ] || return 0
	if [ -z "$RELEASE_JSON" ]; then
		RELEASE_JSON=$(github_api_json "$API_URL/repos/$REPOSITORY/releases/tags/$VERSION") ||
			die "could not load release metadata for $VERSION"
	fi
}

release_asset_api_url() {
	asset_name=$1
	load_release_json
	asset_url=$(printf '%s' "$RELEASE_JSON" | tr '{' '\n' | \
		grep -F "\"name\":\"$asset_name\"" | \
		sed -n 's/.*"url":"\([^"]*\/releases\/assets\/[0-9][0-9]*\)".*/\1/p' | head -n 1)
	[ -n "$asset_url" ] || die "release asset not found: $asset_name"
	printf '%s\n' "$asset_url"
}

detect_os() {
	case "$(uname -s)" in
	Linux) printf '%s\n' linux ;;
	Darwin) printf '%s\n' darwin ;;
	*) die "unsupported operating system: $(uname -s)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) printf '%s\n' amd64 ;;
	arm64 | aarch64) printf '%s\n' arm64 ;;
	*) die "unsupported architecture: $(uname -m)" ;;
	esac
}

download_release_asset() {
	asset_name=$1
	destination=$2
	if [ -n "$TOKEN" ]; then
		asset_url=$(release_asset_api_url "$asset_name")
		curl_request -fsSL --retry 3 --connect-timeout 10 \
			-H "Accept: application/octet-stream" -o "$destination" "$asset_url" ||
			die "download failed: $asset_name"
	else
		url="$GITHUB_URL/$REPOSITORY/releases/download/$VERSION/$asset_name"
		curl_request -fsSL --retry 3 --connect-timeout 10 -o "$destination" "$url" ||
			die "download failed: $url"
	fi
}

sha256_file() {
	file=$1
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$file" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$file" | awk '{print $1}'
	elif command -v openssl >/dev/null 2>&1; then
		openssl dgst -sha256 "$file" | awk '{print $NF}'
	else
		die "no SHA-256 verification tool found (sha256sum, shasum, or openssl required)"
	fi
}

verify_checksum() {
	archive=$1
	checksums=$2
	archive_name=$3
	expected=$(awk -v name="$archive_name" '$2 == name {print $1; exit}' "$checksums")
	[ -n "$expected" ] || die "checksum entry not found for $archive_name"
	actual=$(sha256_file "$archive")
	[ "$actual" = "$expected" ] || die "checksum mismatch for $archive_name"
}

verify_archive_layout() {
	archive=$1
	entries=$(tar -tzf "$archive") || die "could not inspect release archive"
	[ "$entries" = "$BINARY_NAME" ] || die "unexpected archive contents: $entries"
}

install_binary() {
	source=$1
	destination_dir=$2
	mkdir -p "$destination_dir" || die "could not create install directory: $destination_dir"
	[ -d "$destination_dir" ] || die "install path is not a directory: $destination_dir"
	[ -w "$destination_dir" ] || die "install directory is not writable: $destination_dir (use --bin-dir)"

	INSTALL_TMP="$destination_dir/.${BINARY_NAME}.install.$$"
	if command -v install >/dev/null 2>&1; then
		install -m 0755 "$source" "$INSTALL_TMP" || die "could not stage Protonman binary"
	else
		cp "$source" "$INSTALL_TMP" || die "could not stage Protonman binary"
		chmod 0755 "$INSTALL_TMP" || die "could not make Protonman executable"
	fi
	mv -f "$INSTALL_TMP" "$destination_dir/$BINARY_NAME" || die "could not install Protonman"
	INSTALL_TMP=""
RELEASE_JSON=""
}

parse_args() {
	while [ "$#" -gt 0 ]; do
		case "$1" in
		--version)
			[ "$#" -ge 2 ] || die "--version requires a value"
			VERSION=$2
			shift 2
			;;
		--bin-dir)
			[ "$#" -ge 2 ] || die "--bin-dir requires a value"
			BIN_DIR=$2
			shift 2
			;;
		-h | --help)
			usage
			exit 0
			;;
		*) die "unknown argument: $1" ;;
		esac
	done
}

main() {
	parse_args "$@"
	require_command curl
	require_command tar
	require_command awk
	require_command grep
	require_command uname
	require_command mktemp

	[ -n "$BIN_DIR" ] || die "install directory is empty; set --bin-dir or PROTONMAN_INSTALL_DIR"
	if [ -z "$VERSION" ]; then
		VERSION=$(resolve_latest_version)
	fi
	validate_version "$VERSION"

	os=$(detect_os)
	arch=$(detect_arch)
	plain_version=${VERSION#v}
	archive_name="protonman_${plain_version}_${os}_${arch}.tar.gz"
	TMP_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t protonman-install) || die "could not create temporary directory"
	archive_path="$TMP_DIR/$archive_name"
	checksums_path="$TMP_DIR/checksums.txt"

	printf 'Installing Protonman %s for %s/%s...\n' "$plain_version" "$os" "$arch"
	download_release_asset "$archive_name" "$archive_path"
	download_release_asset "checksums.txt" "$checksums_path"
	verify_checksum "$archive_path" "$checksums_path" "$archive_name"
	verify_archive_layout "$archive_path"
	tar -xzf "$archive_path" -C "$TMP_DIR" "$BINARY_NAME" || die "could not extract Protonman"

	extracted="$TMP_DIR/$BINARY_NAME"
	[ -f "$extracted" ] || die "release archive did not contain $BINARY_NAME"
	chmod 0755 "$extracted" || die "could not make downloaded binary executable"
	expected_version="Protonman $plain_version"
	actual_version=$($extracted --version 2>/dev/null) || die "downloaded binary failed version verification"
	[ "$actual_version" = "$expected_version" ] ||
		die "downloaded binary version mismatch: got '$actual_version', want '$expected_version'"

	install_binary "$extracted" "$BIN_DIR"
	installed_version=$($BIN_DIR/$BINARY_NAME --version 2>/dev/null) || die "installed binary failed verification"
	[ "$installed_version" = "$expected_version" ] || die "installed binary version mismatch: $installed_version"

	printf 'Installed %s to %s/%s\n' "$expected_version" "$BIN_DIR" "$BINARY_NAME"
	case ":${PATH:-}:" in
	*":$BIN_DIR:"*) ;;
	*) printf 'Add %s to PATH to run protonman from your shell.\n' "$BIN_DIR" ;;
	esac
}

if [ "${PROTONMAN_INSTALLER_SOURCE_ONLY:-0}" != "1" ]; then
	main "$@"
fi
