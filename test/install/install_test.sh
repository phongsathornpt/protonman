#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
INSTALLER="$ROOT/install.sh"
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT HUP INT TERM

fail() {
	printf 'install_test: %s\n' "$*" >&2
	exit 1
}

make_fake_tools() {
	fake_bin=$1
	mkdir -p "$fake_bin"
	cat > "$fake_bin/curl" <<'SCRIPT'
#!/bin/sh
set -eu
out=""
write_format=""
url=""
while [ "$#" -gt 0 ]; do
	case "$1" in
	-o)
		out=$2
		shift 2
		;;
	-w)
		write_format=$2
		shift 2
		;;
	http://* | https://*)
		url=$1
		shift
		;;
	*) shift ;;
	esac
done
if [ -n "$write_format" ]; then
	printf '%s' "${FAKE_LATEST_URL:-https://github.com/phongsathornpt/protonman/releases/tag/v1.2.3}"
	exit 0
fi
case "$url" in
*/releases/latest)
	printf '%s' '{"tag_name":"v1.2.3"}'
	exit 0
	;;
*/releases/tags/*)
	archive_name=${FAKE_ARCHIVE_NAME:-protonman_1.2.3_linux_amd64.tar.gz}
	printf '{\n  "assets": [\n    {\n      "url": "https://api.github.com/repos/phongsathornpt/protonman/releases/assets/101",\n      "id": 101,\n      "name": "%s"\n    },\n    {\n      "url": "https://api.github.com/repos/phongsathornpt/protonman/releases/assets/102",\n      "id": 102,\n      "name": "checksums.txt"\n    }\n  ]\n}' "$archive_name"
	exit 0
	;;
*/releases/assets/101)
	[ -n "$out" ] || exit 2
	cp "$FAKE_RELEASE_DIR/${FAKE_ARCHIVE_NAME:-protonman_1.2.3_linux_amd64.tar.gz}" "$out"
	exit 0
	;;
*/releases/assets/102)
	[ -n "$out" ] || exit 2
	cp "$FAKE_RELEASE_DIR/checksums.txt" "$out"
	exit 0
	;;
esac
[ -n "$out" ] || exit 2
name=${url##*/}
cp "$FAKE_RELEASE_DIR/$name" "$out"
SCRIPT
	cat > "$fake_bin/uname" <<'SCRIPT'
#!/bin/sh
case "${1:-}" in
-s) printf '%s\n' "${FAKE_UNAME_S:-Linux}" ;;
-m) printf '%s\n' "${FAKE_UNAME_M:-x86_64}" ;;
*) printf '%s\n' "${FAKE_UNAME_S:-Linux}" ;;
esac
SCRIPT
	chmod +x "$fake_bin/curl" "$fake_bin/uname"
}

make_release() {
	release_dir=$1
	version=$2
	os_name=${3:-linux}
	arch=${4:-amd64}
	mkdir -p "$release_dir/package"
	cat > "$release_dir/package/protonman" <<SCRIPT
#!/bin/sh
if [ "\${1:-}" = "--version" ]; then
	printf '%s\n' 'Protonman $version'
	exit 0
fi
exit 0
SCRIPT
	chmod +x "$release_dir/package/protonman"
	archive="protonman_${version}_${os_name}_${arch}.tar.gz"
	(cd "$release_dir/package" && tar -czf "../$archive" protonman)
	(cd "$release_dir" && sha256sum "$archive" > checksums.txt)
}

run_installer() {
	fake_bin=$1
	release_dir=$2
	home=$3
	shift 3
	PATH="$fake_bin:$PATH" FAKE_RELEASE_DIR="$release_dir" HOME="$home" \
		sh "$INSTALLER" "$@"
}

printf '%s\n' 'test: successful verified install'
case_dir="$TEST_ROOT/success"
mkdir -p "$case_dir"
make_fake_tools "$case_dir/bin"
make_release "$case_dir/release" 1.2.3
mkdir -p "$case_dir/home"
run_installer "$case_dir/bin" "$case_dir/release" "$case_dir/home" \
	--version v1.2.3 --bin-dir "$case_dir/install" >/dev/null
[ -x "$case_dir/install/protonman" ] || fail 'installed binary missing'
[ "$("$case_dir/install/protonman" --version)" = 'Protonman 1.2.3' ] || fail 'installed version mismatch'

printf '%s\n' 'test: checksum mismatch preserves existing binary'
case_dir="$TEST_ROOT/checksum"
mkdir -p "$case_dir"
make_fake_tools "$case_dir/bin"
make_release "$case_dir/release" 1.2.3
printf '%s\n' 'deadbeef  protonman_1.2.3_linux_amd64.tar.gz' > "$case_dir/release/checksums.txt"
mkdir -p "$case_dir/home" "$case_dir/install"
printf '#!/bin/sh\necho existing\n' > "$case_dir/install/protonman"
chmod +x "$case_dir/install/protonman"
if run_installer "$case_dir/bin" "$case_dir/release" "$case_dir/home" \
	--version v1.2.3 --bin-dir "$case_dir/install" >/dev/null 2>&1; then
	fail 'checksum mismatch unexpectedly succeeded'
fi
[ "$("$case_dir/install/protonman")" = existing ] || fail 'failed install replaced existing binary'

printf '%s\n' 'test: unexpected archive layout is rejected'
case_dir="$TEST_ROOT/layout"
mkdir -p "$case_dir"
make_fake_tools "$case_dir/bin"
make_release "$case_dir/release" 1.2.3
printf 'extra\n' > "$case_dir/release/package/extra.txt"
(cd "$case_dir/release/package" && tar -czf ../protonman_1.2.3_linux_amd64.tar.gz protonman extra.txt)
(cd "$case_dir/release" && sha256sum protonman_1.2.3_linux_amd64.tar.gz > checksums.txt)
mkdir -p "$case_dir/home"
if run_installer "$case_dir/bin" "$case_dir/release" "$case_dir/home" \
	--version v1.2.3 --bin-dir "$case_dir/install" >/dev/null 2>&1; then
	fail 'unexpected archive layout succeeded'
fi

printf '%s\n' 'test: unsupported published platforms fail closed'
for platform in 'Linux aarch64' 'Darwin x86_64'; do
	set -- $platform
	case_dir="$TEST_ROOT/unsupported-$1-$2"
	mkdir -p "$case_dir"
	make_fake_tools "$case_dir/bin"
	make_release "$case_dir/release" 1.2.3
	mkdir -p "$case_dir/home"
	if PATH="$case_dir/bin:$PATH" FAKE_RELEASE_DIR="$case_dir/release" FAKE_UNAME_S="$1" FAKE_UNAME_M="$2" HOME="$case_dir/home" \
		sh "$INSTALLER" --version v1.2.3 --bin-dir "$case_dir/install" >/dev/null 2>&1; then
		fail "unsupported platform $1/$2 unexpectedly succeeded"
	fi
done

printf '%s\n' 'test: unsupported architecture fails closed'
case_dir="$TEST_ROOT/arch"
mkdir -p "$case_dir"
make_fake_tools "$case_dir/bin"
make_release "$case_dir/release" 1.2.3
mkdir -p "$case_dir/home"
if PATH="$case_dir/bin:$PATH" FAKE_RELEASE_DIR="$case_dir/release" FAKE_UNAME_M=mips64 HOME="$case_dir/home" \
	sh "$INSTALLER" --version v1.2.3 --bin-dir "$case_dir/install" >/dev/null 2>&1; then
	fail 'unsupported architecture unexpectedly succeeded'
fi

printf '%s\n' 'test: authenticated private release uses GitHub asset API'
case_dir="$TEST_ROOT/private"
mkdir -p "$case_dir"
make_fake_tools "$case_dir/bin"
make_release "$case_dir/release" 1.2.3
mkdir -p "$case_dir/home"
PATH="$case_dir/bin:$PATH" FAKE_RELEASE_DIR="$case_dir/release" \
	FAKE_ARCHIVE_NAME='protonman_1.2.3_linux_amd64.tar.gz' GITHUB_TOKEN='test-token' HOME="$case_dir/home" \
	sh "$INSTALLER" --version v1.2.3 --bin-dir "$case_dir/install" >/dev/null
[ "$("$case_dir/install/protonman" --version)" = 'Protonman 1.2.3' ] || fail 'private release install version mismatch'

printf '%s\n' 'test: latest stable tag is resolved from redirect'
case_dir="$TEST_ROOT/latest"
mkdir -p "$case_dir"
make_fake_tools "$case_dir/bin"
latest=$(PATH="$case_dir/bin:$PATH" FAKE_LATEST_URL='https://github.com/phongsathornpt/protonman/releases/tag/v2.4.6' \
	PROTONMAN_INSTALLER_SOURCE_ONLY=1 sh -c '. "$1"; resolve_latest_version' sh "$INSTALLER")
[ "$latest" = v2.4.6 ] || fail "latest version = $latest, want v2.4.6"

printf '%s\n' 'install tests: PASS'
