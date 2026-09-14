#!/bin/sh
set -eu

version=3.5.1
filename=SymbolsNerdFontMono-Regular.ttf
blob_sha=3b5a184756f6bc44fe77e7c884ec9fda24084505
url="https://raw.githubusercontent.com/ryanoasis/nerd-fonts/v${version}/patched-fonts/NerdFontsSymbolsOnly/${filename}"
out=${1:-"bin/share/fonts/${filename}"}

mkdir -p "$(dirname "$out")"
if [ -f "$out" ] && [ "$(git hash-object "$out")" = "$blob_sha" ]; then
    exit 0
fi

tmp="${out}.tmp.$$"
trap 'rm -f "$tmp"' EXIT INT TERM HUP
curl -fL --retry 3 --retry-delay 1 "$url" -o "$tmp"
actual=$(git hash-object "$tmp")
if [ "$actual" != "$blob_sha" ]; then
    echo "Nerd Font checksum mismatch: got $actual, want $blob_sha" >&2
    exit 1
fi
mv "$tmp" "$out"
trap - EXIT INT TERM HUP
