#!/usr/bin/env bash
# Check parity between docs/en and docs/ru:
#   - same set of files (relative paths)
#   - same ordered sequence of Markdown heading levels (#{1,6}) per file
#
# Exit codes:
#   0 - OK
#   1 - mismatch found
#   2 - usage / IO error
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EN_DIR="${ROOT}/docs/en"
RU_DIR="${ROOT}/docs/ru"

if [[ ! -d "${EN_DIR}" || ! -d "${RU_DIR}" ]]; then
	echo "error: docs/en or docs/ru not found under ${ROOT}" >&2
	exit 2
fi

status=0

list_files() {
	(cd "$1" && find . -type f -name '*.md' | sed 's|^\./||' | sort)
}

en_files="$(list_files "${EN_DIR}")"
ru_files="$(list_files "${RU_DIR}")"

only_en="$(comm -23 <(echo "${en_files}") <(echo "${ru_files}"))"
only_ru="$(comm -13 <(echo "${en_files}") <(echo "${ru_files}"))"

if [[ -n "${only_en}" ]]; then
	echo "Files present in docs/en but missing in docs/ru:"
	echo "${only_en}" | sed 's/^/  - /'
	status=1
fi

if [[ -n "${only_ru}" ]]; then
	echo "Files present in docs/ru but missing in docs/en:"
	echo "${only_ru}" | sed 's/^/  - /'
	status=1
fi

# Compare heading structure (levels only, not text) for common files.
common="$(comm -12 <(echo "${en_files}") <(echo "${ru_files}"))"

heading_levels() {
	# Print one line per heading: number of leading '#' chars.
	grep -E '^#{1,6} ' "$1" 2>/dev/null | sed -E 's/^(#+).*/\1/' | awk '{print length}'
}

while IFS= read -r rel; do
	[[ -z "${rel}" ]] && continue
	en_h="$(heading_levels "${EN_DIR}/${rel}")"
	ru_h="$(heading_levels "${RU_DIR}/${rel}")"
	if [[ "${en_h}" != "${ru_h}" ]]; then
		en_count=$(echo "${en_h}" | grep -c . || true)
		ru_count=$(echo "${ru_h}" | grep -c . || true)
		echo "Heading structure differs: ${rel} (en=${en_count}, ru=${ru_count})"
		status=1
	fi
done <<<"${common}"

if [[ ${status} -eq 0 ]]; then
	echo "OK: docs/en and docs/ru are consistent"
fi

exit ${status}
