#!/usr/bin/env bash
#
# Run a side-by-side comparison of basic, custom, and (when available) estnltk
# parsers across the Estonian gold datasets, then assemble the results into a
# markdown comparison report.
#
# Usage:
#   scripts/parser-comparison-et.sh
#   scripts/parser-comparison-et.sh -o docs/baselines/latest-et-comparison.md
#   scripts/parser-comparison-et.sh DS1.json DS2.json ...
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
            sed -n '2,13p' "$0"
            exit 0 ;;
        *) DATASETS+=("$1"); shift ;;
    esac
done

if [[ ${#DATASETS[@]} -eq 0 ]]; then
    while IFS= read -r f; do DATASETS+=("$f"); done \
        < <(ls testdata/parser-eval/et/gold/*.json 2>/dev/null | sort)
fi

if [[ ${#DATASETS[@]} -eq 0 ]]; then
    echo "no datasets to compare" >&2
    exit 1
fi

PARSERS="basic,custom"
estnltk_available=false
if [[ -n "${FINNESTDB_ESTNLTK_CMD:-}" ]]; then
    estnltk_available=true
elif [[ -x .venv-estnltk/bin/python && -f scripts/estnltk_adapter_example.py ]]; then
    estnltk_available=true
fi

if $estnltk_available; then
    PARSERS="basic,custom,estnltk"
    echo ">> Including estnltk (adapter auto-detected)" >&2
else
    echo ">> Skipping estnltk (run 'make setup-estnltk' to enable)" >&2
fi

export LD_LIBRARY_PATH="$ROOT/parser/target/release:${LD_LIBRARY_PATH:-}"

REPORTS_DIR="$ROOT/reports/parser-eval"
mkdir -p "$REPORTS_DIR"

THIS_RUN_REPORTS=()
RUN_TS="$(date -u +%Y%m%dT%H%M%SZ)"

for ds in "${DATASETS[@]}"; do
    name="$(python3 -c "
import json, re, sys
raw = json.load(open(sys.argv[1])).get('name', 'unnamed')
slug = re.sub(r'[^A-Za-z0-9._-]+', '-', str(raw)).strip('-') or 'unnamed'
print(slug[:80])
" "$ds")"
    out="$REPORTS_DIR/${RUN_TS}-${name}.json"
    echo ">> $ds -> $name" >&2
    go run ./cmd/parsertest -dataset "$ds" -parsers "$PARSERS" -warmup 1 -repeat 3 -out "$out" >&2
    THIS_RUN_REPORTS+=("$out")
done

ARGS=(-title "Estonian parser comparison ($RUN_TS, parsers: $PARSERS)")
ARGS+=("${THIS_RUN_REPORTS[@]}")

if [[ -n "$OUT" ]]; then
    go run ./cmd/parser-compare "${ARGS[@]}" > "$OUT"
    echo ">> Wrote $OUT" >&2
else
    go run ./cmd/parser-compare "${ARGS[@]}"
fi
