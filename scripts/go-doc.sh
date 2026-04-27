#!/usr/bin/env bash
set -euo pipefail

GO_IMAGE="${GO_IMAGE:-golang:1.25}"

if [ "$#" -lt 1 ]; then
	cat <<'USAGE'
Usage:
  scripts/go-doc.sh PACKAGE_OR_SYMBOL [GO_DOC_ARGS...]

Examples:
  scripts/go-doc.sh fmt
  scripts/go-doc.sh io.Reader
  scripts/go-doc.sh -all net/http

Environment:
  GO_IMAGE   Docker image with Go toolchain (default: golang:1.25)
USAGE
	exit 1
fi

exec docker compose run --rm --no-deps godoc "$@"
