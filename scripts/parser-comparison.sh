#!/usr/bin/env bash
#
# Run a side-by-side comparison of basic, custom, and (optionally) omorfi
# parsers across the Finnish gold datasets, then assemble the results into
# a markdown comparison report.
#
# Omorfi is included automatically iff $FINNESTDB_OMORFI_CMD is set (see
# docs/OMORFI_COMPARISON.md for setup). Without it the comparison runs
# basic vs custom only.
#
# Usage:
#   scripts/parser-comparison.sh                          # default datasets, stdout
#   scripts/parser-comparison.sh -o report.md             # write to file
#   scripts/parser-comparison.sh DS1.json DS2.json ...    # custom datasets
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
            sed -n '2,15p' "$0"
            exit 0 ;;
        *) DATASETS+=("$1"); shift ;;
    esac
done

if [[ ${#DATASETS[@]} -eq 0 ]]; then
    # Default: every fi/gold/*.json that's actually present.
    while IFS= read -r f; do DATASETS+=("$f"); done \
        < <(ls testdata/parser-eval/fi/gold/*.json 2>/dev/null | sort)
fi

if [[ ${#DATASETS[@]} -eq 0 ]]; then
    echo "no datasets to compare" >&2
    exit 1
fi

PARSERS="basic,custom"

# Auto-detect omorfi. Run modes the user might have configured:
#   - $FINNESTDB_OMORFI_CMD set (explicit adapter override)
#   - $OMORFI_ANALYSE_HFST set and pointing at a real file (custom model
#     location — the adapter itself looks here first)
#   - Repo-local cache:   ./.cache/omorfi/omorfi.analyse.hfst
#   - User-level cache:   ~/.cache/omorfi/omorfi.analyse.hfst (populated
#     by `make setup-omorfi`)
omorfi_available=false
if [[ -n "${FINNESTDB_OMORFI_CMD:-}" ]]; then
    omorfi_available=true
elif [[ -n "${OMORFI_ANALYSE_HFST:-}" && -f "${OMORFI_ANALYSE_HFST}" ]]; then
    omorfi_available=true
elif [[ -f .cache/omorfi/omorfi.analyse.hfst ]]; then
    omorfi_available=true
elif [[ -f "$HOME/.cache/omorfi/omorfi.analyse.hfst" ]]; then
    omorfi_available=true
fi

if $omorfi_available; then
    PARSERS="basic,custom,omorfi"
    echo ">> Including omorfi (model auto-detected)" >&2
else
    echo ">> Skipping omorfi (run 'make setup-omorfi' to enable)" >&2
fi

export LD_LIBRARY_PATH="$ROOT/parser/target/release:${LD_LIBRARY_PATH:-}"

REPORTS_DIR="$ROOT/reports/parser-eval"
mkdir -p "$REPORTS_DIR"

# Track which reports came out of *this* run so parser-compare doesn't pick up
# stale ones from previous runs.
THIS_RUN_REPORTS=()
RUN_TS="$(date -u +%Y%m%dT%H%M%SZ)"

for ds in "${DATASETS[@]}"; do
    # Read name and slugify it: only [A-Za-z0-9._-] allowed in the report
    # filename, so a maliciously-crafted dataset name (e.g. with path
    # separators or shell metacharacters) can't escape REPORTS_DIR.
    name="$(python3 -c "
import json, re, sys
raw = json.load(open(sys.argv[1])).get('name', 'unnamed')
slug = re.sub(r'[^A-Za-z0-9._-]+', '-', str(raw)).strip('-') or 'unnamed'
print(slug[:80])
" "$ds")"
    out="$REPORTS_DIR/${RUN_TS}-${name}.json"
    echo ">> $ds → $name" >&2
    go run ./cmd/parsertest -dataset "$ds" -parsers "$PARSERS" -warmup 2 -repeat 5 -out "$out" >&2
    THIS_RUN_REPORTS+=("$out")
done

ARGS=(-title "Parser comparison ($RUN_TS, parsers: $PARSERS)")
ARGS+=("${THIS_RUN_REPORTS[@]}")

if [[ -n "$OUT" ]]; then
    go run ./cmd/parser-compare "${ARGS[@]}" > "$OUT"
    echo ">> Wrote $OUT" >&2
else
    go run ./cmd/parser-compare "${ARGS[@]}"
fi
