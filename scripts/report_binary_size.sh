#!/bin/sh

set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
report_dir=$(mktemp -d "${TMPDIR:-/tmp}/protonman-size.XXXXXX")
trap 'rm -rf "$report_dir"' EXIT HUP INT TERM

goos=${GOOS:-$(go env GOOS)}
goarch=${GOARCH:-$(go env GOARCH)}
include_desktop=${SIZE_INCLUDE_DESKTOP:-0}
version=${VERSION:-size-report}
ldflags="-s -w -X github.com/phongsathornpt/protonman/internal/base/buildinfo.version=$version"

build_cli() {
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
		-trimpath -ldflags "$ldflags" \
		-o "$report_dir/protonman" ./cmd/protonman
}

print_size() {
	name=$1
	path=$2
	bytes=$(wc -c < "$path" | tr -d '[:space:]')
	compressed=$(gzip -c "$path" | wc -c | tr -d '[:space:]')
	awk -v name="$name" -v bytes="$bytes" -v compressed="$compressed" \
		'BEGIN { printf "%-24s %10d bytes (%6.2f MiB), gzip %10d bytes (%6.2f MiB)\n", name, bytes, bytes / 1048576, compressed, compressed / 1048576 }'
}

print_archive_size() {
	name=$1
	path=$2
	bytes=$(wc -c < "$path" | tr -d '[:space:]')
	awk -v name="$name" -v bytes="$bytes" \
		'BEGIN { printf "%-24s %10d bytes (%6.2f MiB)\n", name, bytes, bytes / 1048576 }'
}

cd "$repo_root"
printf 'target: %s/%s\n' "$goos" "$goarch"
build_cli
print_size cli "$report_dir/protonman"

if [ "$include_desktop" = 1 ]; then
	CGO_ENABLED=1 GOOS="$goos" GOARCH="$goarch" go build \
		-tags 'desktop,webkit2_41' -trimpath -ldflags "$ldflags" \
		-o "$report_dir/protonman-desktop" ./cmd/protonman-desktop
	print_size desktop "$report_dir/protonman-desktop"
	mkdir "$report_dir/package"
	cp "$report_dir/protonman" "$report_dir/package/protonman"
	cp "$report_dir/protonman-desktop" "$report_dir/package/protonman-desktop"
	tar -czf "$report_dir/desktop.tar.gz" -C "$report_dir/package" protonman protonman-desktop
	print_archive_size desktop-package "$report_dir/desktop.tar.gz"
fi
