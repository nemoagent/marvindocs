#!/usr/bin/env bash
# Validate integrity of a devdocs dump.
#
# Checks per slug listed in docs.json:
#   - index.json, db.zip, meta.json exist and are non-empty
#   - the json files parse; db.zip is a readable zip archive
#   - each file is valid JSON
# Also reports orphan directories present on disk but absent from docs.json.
#
# Exit codes:
#   0 - all OK
#   1 - issues found
#   2 - usage / IO error (missing dump root or docs.json)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCS_PATH="${ROOT}/data/public/docs"

usage() {
	echo "Usage: $(basename "$0") [--docs-path DIR]" >&2
	exit 2
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--docs-path)
		[[ $# -lt 2 ]] && usage
		DOCS_PATH="$2"
		shift 2
		;;
	-h | --help) usage ;;
	*)
		echo "error: unknown option: $1" >&2
		usage
		;;
	esac
done

if [[ ! -d "${DOCS_PATH}" ]]; then
	echo "error: dump root not found: ${DOCS_PATH}" >&2
	exit 2
fi

DOCS_JSON="${DOCS_PATH}/docs.json"
if [[ ! -f "${DOCS_JSON}" ]]; then
	echo "error: docs.json not found: ${DOCS_JSON}" >&2
	exit 2
fi

for tool in jq unzip; do
	if ! command -v "$tool" &>/dev/null; then
		echo "error: $tool is required but not found in PATH" >&2
		exit 2
	fi
done

# Validate docs.json itself
if ! jq empty "${DOCS_JSON}" 2>/dev/null; then
	echo "error: docs.json is not valid JSON: ${DOCS_JSON}" >&2
	exit 2
fi

status=0
count_ok=0
count_missing=0
count_invalid=0
count_orphan=0

REQUIRED_FILES=("index.json" "db.zip" "meta.json")

# Read slugs from docs.json
mapfile -t slugs < <(jq -r '.[].slug' "${DOCS_JSON}")

for slug in "${slugs[@]}"; do
	slug_dir="${DOCS_PATH}/${slug}"
	slug_ok=true

	for fname in "${REQUIRED_FILES[@]}"; do
		fpath="${slug_dir}/${fname}"

		if [[ ! -f "${fpath}" ]]; then
			echo "MISSING  ${slug}/${fname}"
			((count_missing++)) || true
			slug_ok=false
			continue
		fi

		if [[ ! -s "${fpath}" ]]; then
			echo "EMPTY    ${slug}/${fname}"
			((count_missing++)) || true
			slug_ok=false
			continue
		fi

		if [[ "${fname}" == *.zip ]]; then
			valid=$(unzip -tq "${fpath}" >/dev/null 2>&1 && echo yes || echo no)
		else
			valid=$(jq empty "${fpath}" 2>/dev/null && echo yes || echo no)
		fi
		if [[ "${valid}" != yes ]]; then
			echo "INVALID  ${slug}/${fname}"
			((count_invalid++)) || true
			slug_ok=false
		fi
	done

	if "${slug_ok}"; then
		((count_ok++)) || true
	fi
done

# Find orphan directories (on disk but not in docs.json)
# Build a lookup set from the slugs array
declare -A slug_set
for slug in "${slugs[@]}"; do
	slug_set["${slug}"]=1
done

while IFS= read -r dir; do
	basename_dir="$(basename "${dir}")"
	if [[ -z "${slug_set[${basename_dir}]+_}" ]]; then
		echo "ORPHAN   ${basename_dir}"
		((count_orphan++)) || true
		status=1
	fi
done < <(find "${DOCS_PATH}" -mindepth 1 -maxdepth 1 -type d | sort)

# Determine overall status
if [[ ${count_missing} -gt 0 || ${count_invalid} -gt 0 ]]; then
	status=1
fi

echo ""
echo "Summary:"
echo "  OK slugs:       ${count_ok}"
echo "  Missing/empty:  ${count_missing}"
echo "  Invalid JSON:   ${count_invalid}"
echo "  Orphan dirs:    ${count_orphan}"

if [[ ${status} -eq 0 ]]; then
	echo ""
	echo "OK: dump integrity check passed"
fi

exit ${status}
