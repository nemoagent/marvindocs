#!/usr/bin/env bash
# Build one release archive: scripts/release-build.sh VERSION GOOS GOARCH [CC]
# Writes dist/release/marvindocs_VERSION_GOOS_GOARCH.{tar.gz,zip}.
# CC defaults to the host C compiler; CI passes "zig cc -target ..." to
# cross-compile Linux and Windows (mattn/go-sqlite3 needs cgo).
set -euo pipefail

version=$1 goos=$2 goarch=$3 cc=${4:-}
name=marvindocs_${version}_${goos}_${goarch}
out=dist/release
work=$out/$name
ext=
[ "$goos" = windows ] && ext=.exe

ldflags="-s -w -X main.version=$version"
# Static binaries for Linux (musl) and Windows: nothing to install besides the file.
[ "$goos" != darwin ] && ldflags="$ldflags -extldflags=-static"

rm -rf "$work"
mkdir -p "$work"
[ -n "$cc" ] && export CC="$cc"
CGO_ENABLED=1 GOOS=$goos GOARCH=$goarch \
	go build -tags sqlite_fts5 -trimpath -ldflags "$ldflags" \
	-o "$work/marvindocs$ext" ./cmd/marvindocs
cp -r README.md README.ru.md docs "$work/"

cd "$out"
if [ "$goos" = windows ]; then
	rm -f "$name.zip"
	zip -qr "$name.zip" "$name"
else
	tar -czf "$name.tar.gz" "$name"
fi
rm -rf "$name"
ls -l "$name".*
