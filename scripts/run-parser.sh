#!/usr/bin/env bash
# Run one or all parsers defined in parsers/registry.json.
#
# Usage:
#   ./scripts/run-parser.sh <slug>           # run one slug
#   ./scripts/run-parser.sh --all            # run every slug from registry
#   ./scripts/run-parser.sh --list           # print slug | parser | description
#   ./scripts/run-parser.sh --check <slug>   # print resolved command without running
#
# Options:
#   --no-force          skip --force flag (default: --force is passed)
#   --docs-path DIR     override output root (default: data/public/docs)
#
# Exit codes: 0 OK, 1 parser failed, 2 usage/lookup/missing binary
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REGISTRY="${ROOT}/parsers/registry.json"
DOCS_PATH="${ROOT}/data/public/docs"
USE_FORCE=1
MODE=""
SLUG=""

usage() {
	echo "Usage:" >&2
	echo "  $(basename "$0") <slug>           run one slug" >&2
	echo "  $(basename "$0") --all            run every slug from registry" >&2
	echo "  $(basename "$0") --list           print slug | parser | description" >&2
	echo "  $(basename "$0") --check <slug>   print resolved command without running" >&2
	echo "" >&2
	echo "Options:" >&2
	echo "  --no-force          skip --force flag" >&2
	echo "  --docs-path DIR     override output root (default: data/public/docs)" >&2
	exit 2
}

# Parse arguments
while [[ $# -gt 0 ]]; do
	case "$1" in
	--all)
		MODE="all"
		shift
		;;
	--list)
		MODE="list"
		shift
		;;
	--check)
		MODE="check"
		shift
		[[ $# -eq 0 ]] && {
			echo "error: --check requires a <slug>" >&2
			exit 2
		}
		SLUG="$1"
		shift
		;;
	--no-force)
		USE_FORCE=0
		shift
		;;
	--docs-path)
		shift
		[[ $# -eq 0 ]] && {
			echo "error: --docs-path requires a DIR" >&2
			exit 2
		}
		DOCS_PATH="$1"
		shift
		;;
	-*)
		echo "error: unknown option: $1" >&2
		usage
		;;
	*)
		if [[ -z "${MODE}" && -z "${SLUG}" ]]; then
			MODE="one"
			SLUG="$1"
		else
			echo "error: unexpected argument: $1" >&2
			usage
		fi
		shift
		;;
	esac
done

[[ -z "${MODE}" ]] && usage

if ! command -v jq &>/dev/null; then
	echo "error: jq is required but not found in PATH" >&2
	exit 2
fi

if [[ ! -f "${REGISTRY}" ]]; then
	echo "error: registry not found: ${REGISTRY}" >&2
	exit 2
fi

# Print the slug list
cmd_list() {
	jq -r '.docs[] | "\(.slug)\t\(.parser)\t\(.description)"' "${REGISTRY}" |
		column -t -s $'\t'
}

# Look up a slug and resolve its command into the global CMD array.
# Exits 2 if slug not found or binary missing.
CMD=()
resolve_cmd() {
	local slug="$1"

	local entry
	entry="$(jq -c --arg s "${slug}" '.docs[] | select(.slug == $s)' "${REGISTRY}")"
	if [[ -z "${entry}" ]]; then
		echo "error: slug '${slug}' not found in ${REGISTRY}" >&2
		exit 2
	fi

	local parser
	parser="$(jq -r '.parser' <<<"${entry}")"

	local binary="${ROOT}/bin/${parser}-parser"
	if [[ ! -x "${binary}" ]]; then
		echo "error: binary not found or not executable: ${binary}" >&2
		echo "hint:  run 'make parsers' to build all parser binaries" >&2
		exit 2
	fi

	local -a extra_args
	mapfile -t extra_args < <(jq -r '.args[]?' <<<"${entry}")

	CMD=("${binary}" "${extra_args[@]}" "--out" "${DOCS_PATH}/${slug}")
	[[ "${USE_FORCE}" -eq 1 ]] && CMD+=("--force")
	return 0
}

# Run a single slug
run_one() {
	resolve_cmd "$1"
	echo "+ ${CMD[*]}"
	"${CMD[@]}" || exit 1
}

case "${MODE}" in
list)
	cmd_list
	;;
check)
	resolve_cmd "${SLUG}"
	printf '%q ' "${CMD[@]}"
	echo
	;;
one)
	run_one "${SLUG}"
	;;
all)
	slugs="$(jq -r '.docs[].slug' "${REGISTRY}")"
	while IFS= read -r slug; do
		echo "==> ${slug}"
		run_one "${slug}"
	done <<<"${slugs}"
	;;
esac
