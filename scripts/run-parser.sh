#!/usr/bin/env bash
# Run one or all parsers defined in parsers/registry.json.
#
# Usage:
#   ./scripts/run-parser.sh <slug>           # run one slug
#   ./scripts/run-parser.sh --all            # run every slug (continues past failures)
#   ./scripts/run-parser.sh --list           # print slug | parser | description
#   ./scripts/run-parser.sh --check <slug>   # print resolved command without running
#
# Registry entries with "versions" are templates: slug "go-std~{v}" accepts any
# version, e.g. `run-parser.sh go-std~1.26`. After a successful run of a version
# not yet listed, it is appended to parsers/registry.json. Every successful run
# writes <out>/.parser (the parser name): sync-docs.sh never overwrites such dirs.
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
	echo "  $(basename "$0") --all            run every slug (continues past failures)" >&2
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

# Expand templates over their versions: one JSON object per concrete slug.
CONCRETE_JQ='.parsers[] | . as $e |
	if .versions then
		.versions[] as $v | $e + {slug: ($e.slug | gsub("\\{v\\}"; $v)), description: ($e.description | gsub("\\{v\\}"; $v))}
	else . end'

# Print the slug list
cmd_list() {
	jq -r "${CONCRETE_JQ} | \"\(.slug)\t\(.parser)\t\(.description)\"" "${REGISTRY}" |
		column -t -s $'\t'
}

# Look up a slug (fixed first, then version templates) and resolve its command
# into the global CMD array; sets PARSER, TMPL_IDX and VERSION (empty for
# fixed slugs). Exits 2 if slug not found or binary missing.
CMD=()
PARSER=""
TMPL_IDX=""
VERSION=""
resolve_cmd() {
	local slug="$1"
	local idx="" v="" i tmpl prefix suffix

	idx="$(jq -r --arg s "${slug}" '.parsers | map(.versions == null and .slug == $s) | index(true) // empty' "${REGISTRY}")"
	if [[ -z "${idx}" ]]; then
		while IFS=$'\t' read -r i tmpl; do
			prefix="${tmpl%%"{v}"*}"
			suffix="${tmpl#*"{v}"}"
			if [[ "${slug}" == "${prefix}"?*"${suffix}" ]]; then
				v="${slug#"${prefix}"}"
				v="${v%"${suffix}"}"
				idx="${i}"
				TMPL_IDX="${i}"
				break
			fi
		done < <(jq -r '.parsers | to_entries[] | select(.value.versions) | "\(.key)\t\(.value.slug)"' "${REGISTRY}")
	fi
	if [[ -z "${idx}" ]]; then
		echo "error: slug '${slug}' not found in ${REGISTRY}" >&2
		echo "hint: versioned slugs: $(jq -r '[.parsers[] | select(.versions) | .slug] | join(", ")' "${REGISTRY}")" >&2
		exit 2
	fi
	if [[ -n "${v}" && ! "${v}" =~ ^[A-Za-z0-9._-]+$ ]]; then
		echo "error: invalid version '${v}' in slug '${slug}'" >&2
		exit 2
	fi
	VERSION="${v}"

	PARSER="$(jq -r --argjson i "${idx}" '.parsers[$i].parser' "${REGISTRY}")"

	local binary="${ROOT}/bin/${PARSER}-parser"
	if [[ ! -x "${binary}" ]]; then
		echo "error: binary not found or not executable: ${binary}" >&2
		echo "hint:  run 'make parsers' to build all parser binaries" >&2
		exit 2
	fi

	local -a extra_args
	mapfile -t extra_args < <(jq -r --argjson i "${idx}" --arg v "${v}" '.parsers[$i].args[]? | gsub("\\{v\\}"; $v)' "${REGISTRY}")

	CMD=("${binary}" "${extra_args[@]}" "--out" "${DOCS_PATH}/${slug}")
	[[ "${USE_FORCE}" -eq 1 ]] && CMD+=("--force")
	return 0
}

# Append VERSION to the template TMPL_IDX in the registry if it is new.
register_version() {
	local slug="$1" tmp
	[[ -z "${TMPL_IDX}" ]] && return 0
	if jq -e --argjson i "${TMPL_IDX}" --arg v "${VERSION}" '.parsers[$i].versions | index($v)' "${REGISTRY}" >/dev/null; then
		return 0
	fi
	tmp="$(mktemp "${REGISTRY}.XXXXXX")"
	if jq --argjson i "${TMPL_IDX}" --arg v "${VERSION}" '.parsers[$i].versions += [$v]' "${REGISTRY}" >"${tmp}"; then
		mv "${tmp}" "${REGISTRY}"
		echo "registered ${slug} in parsers/registry.json"
	else
		rm -f "${tmp}"
		echo "warning: could not register ${slug} in registry" >&2
	fi
}

# Run a single slug; returns 1 if the parser fails
run_one() {
	local slug="$1"
	resolve_cmd "${slug}"
	echo "+ ${CMD[*]}"
	"${CMD[@]}" || return 1
	echo "${PARSER}" >"${DOCS_PATH}/${slug}/.parser"
	register_version "${slug}"
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
	run_one "${SLUG}" || exit 1
	"${ROOT}/scripts/rebuild-catalog.sh" "${DOCS_PATH}"
	;;
all)
	ok=0
	failed=()
	while IFS= read -r slug; do
		echo "==> ${slug}"
		# subshell: a lookup/binary error (exit 2) counts as a failure too
		if (run_one "${slug}"); then
			ok=$((ok + 1))
		else
			failed+=("${slug}")
		fi
	done < <(jq -r "${CONCRETE_JQ} | .slug" "${REGISTRY}")
	echo "summary: ${ok} ok, ${#failed[@]} failed"
	if [[ ${ok} -gt 0 ]]; then
		"${ROOT}/scripts/rebuild-catalog.sh" "${DOCS_PATH}"
	fi
	if [[ ${#failed[@]} -gt 0 ]]; then
		echo "FAILED: ${failed[*]}" >&2
		exit 1
	fi
	;;
esac
