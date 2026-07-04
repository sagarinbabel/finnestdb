#!/usr/bin/env bash
#
# Run cmd/ambiguityeval over the committed ambiguity gold slice
# (testdata/parser-eval/*-ambiguity/*.json) and write a dated markdown
# report, parallel to scripts/parser-comparison{,-et}.sh for the headline
# eval. See docs/PARSER_EVAL_METHODOLOGY.md §"Ambiguity and meaning-check
# calibration" for what this measures.
#
# Usage:
#   scripts/compare-ambiguity.sh
#   scripts/compare-ambiguity.sh -o reports/parser-eval/ambiguity.md
#   scripts/compare-ambiguity.sh path/to/one-gold-file.json
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

OUT=""
DATASETS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        -o|--output) OUT="$2"; shift 2 ;;
        -h|--help)
            sed -n '2,14p' "$0"
            exit 0 ;;
        *) DATASETS+=("$1"); shift ;;
    esac
done

if [[ ${#DATASETS[@]} -eq 0 ]]; then
    while IFS= read -r f; do DATASETS+=("$f"); done \
        < <(ls testdata/parser-eval/*-ambiguity/*.json 2>/dev/null | sort)
fi

if [[ ${#DATASETS[@]} -eq 0 ]]; then
    echo "no ambiguity gold files found under testdata/parser-eval/*-ambiguity/*.json" >&2
    exit 1
fi

export LD_LIBRARY_PATH="$ROOT/parser/target/release:${LD_LIBRARY_PATH:-}"

REPORTS_DIR="$ROOT/reports/parser-eval"
mkdir -p "$REPORTS_DIR"
RUN_TS="$(date -u +%Y%m%dT%H%M%SZ)"

# cmd/ambiguityeval accepts multiple gold files in one invocation and
# reports one section per dataset, so a single run covers both FI and ET.
JSON_OUT="$REPORTS_DIR/${RUN_TS}-ambiguity.json"

if [[ -n "$OUT" ]]; then
    go run ./cmd/ambiguityeval -db finnestdb.db -out "$JSON_OUT" "${DATASETS[@]}" | tee "$OUT"
    echo ">> Wrote $OUT" >&2
else
    go run ./cmd/ambiguityeval -db finnestdb.db -out "$JSON_OUT" "${DATASETS[@]}"
fi
