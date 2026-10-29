#!/usr/bin/env bash
# Build .deb and .rpm from a Linux release archive:
# scripts/release-package.sh VERSION GOARCH
# Needs dist/release/marvindocs_VERSION_linux_GOARCH.tar.gz from
# scripts/release-build.sh and nfpm in PATH. Writes the packages to dist/release.
set -euo pipefail

version=$1 goarch=$2
name=marvindocs_${version}_linux_${goarch}
# nfpm does not expand env vars in file paths: stage the binary at a fixed one.
rm -rf dist/pkg
mkdir -p dist/pkg
tar -xzf "dist/release/$name.tar.gz" -C dist/pkg --strip-components=1
export VERSION=${version#v} ARCH=$goarch
for p in deb rpm; do
	nfpm package -f packaging/nfpm.yaml -p "$p" -t dist/release/
done
rm -rf dist/pkg
