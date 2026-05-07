#!/usr/bin/env bash
#
# Run a side-by-side comparison of basic, custom, and estnltk (Vabamorf)
# parsers across the Estonian gold datasets, then assemble the results into a
# markdown comparison report.
#
# EstNLTK/Vabamorf is **required by default** — every Estonian baseline must
# show the analyzer column so dict-only numbers are never read in isolation.
# Run `make setup-estnltk` once on a fresh machine. To bypass on a machine
# where estnltk cannot be installed, pass --allow-missing-baseline (ad-hoc
# local use only — never commit reports produced this way).
#
# Usage:
#   scripts/parser-comparison-et.sh
#   scripts/parser-comparison-et.sh -o docs/baselines/latest-et-comparison.md
#   scripts/parser-comparison-et.sh DS1.json DS2.json ...
#   scripts/parser-comparison-et.sh --allow-missing-baseline ...
#   scripts/parser-comparison-et.sh --stratified ...
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

OUT=""
DATASETS=()
ALLOW_MISSING=false
STRATIFIED=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        -o|--output) OUT="$2"; shift 2 ;;
        --allow-missing-baseline) ALLOW_MISSING=true; shift ;;
        --stratified) STRATIFIED=true; shift ;;
        -h|--help)
            sed -n '2,18p' "$0"
            exit 0 ;;
        *) DATASETS+=("$1"); shift ;;
    esac
done

if [[ ${#DATASETS[@]} -eq 0 ]]; then
    # Default discovery: every et/gold/*.json EXCEPT dev splits (held-out
    # discipline — see scripts/parser-comparison.sh comment). Globs both
    # testdata/parser-eval/et/gold/ (committed ET gold — manual + grammar
    # only) and localdata/parser-eval/et/gold/ (the NC-licensed UD-ET
    # dev/test files written by scripts/fetch-and-import-ud.sh). Without
    # the localdata glob, fresh clones would only see ~50 ET cases.
    while IFS= read -r f; do DATASETS+=("$f"); done \
        < <(ls testdata/parser-eval/et/gold/*.json localdata/parser-eval/et/gold/*.json 2>/dev/null \
            | grep -v -- '-dev-v' | sort)
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
elif $ALLOW_MISSING; then
    echo ">> WARNING: estnltk missing, --allow-missing-baseline set — running dict-only" >&2
    echo ">>          Do NOT commit reports produced this way; analyzer parity is required." >&2
else
    cat >&2 <<'EOF'
ERROR: estnltk is required for Estonian parser comparison but is not available.

Every committed ET baseline must include the estnltk column so dict-only
numbers (basic/custom) are never read without the Vabamorf upper bound.

To install:
    make setup-estnltk

If you genuinely need a dict-only run for local experimentation, pass:
    --allow-missing-baseline

Override path:
    export FINNESTDB_ESTNLTK_CMD="..."
EOF
    exit 2
fi

export LD_LIBRARY_PATH="$ROOT/parser/target/release:${LD_LIBRARY_PATH:-}"

REPORTS_DIR="$ROOT/reports/parser-eval"
mkdir -p "$REPORTS_DIR"

THIS_RUN_REPORTS=()
RUN_TS="$(date -u +%Y%m%dT%H%M%SZ)"

PARSERTEST_EXTRA=()
COMPARE_EXTRA=()
if $STRATIFIED; then
    PARSERTEST_EXTRA+=(-stratified)
    COMPARE_EXTRA+=(-stratified)
fi

for ds in "${DATASETS[@]}"; do
    # Slug from the dataset *filename* (not the JSON `name` field) —
    # see scripts/parser-comparison.sh for the rationale.
    base="$(basename "$ds" .json)"
    slug="$(printf '%s' "$base" | python3 -c "
import re, sys
raw = sys.stdin.read().strip()
slug = re.sub(r'[^A-Za-z0-9._-]+', '-', raw).strip('-') or 'unnamed'
print(slug[:80])
")"
    out="$REPORTS_DIR/${RUN_TS}-${slug}.json"
    echo ">> $ds -> $slug" >&2
    go run ./cmd/parsertest -dataset "$ds" -parsers "$PARSERS" -warmup 1 -repeat 3 -out "$out" "${PARSERTEST_EXTRA[@]}" >&2
    THIS_RUN_REPORTS+=("$out")
done

ARGS=(-title "Estonian parser comparison ($RUN_TS, parsers: $PARSERS)")
ARGS+=("${COMPARE_EXTRA[@]}")
ARGS+=("${THIS_RUN_REPORTS[@]}")

if [[ -n "$OUT" ]]; then
    go run ./cmd/parser-compare "${ARGS[@]}" > "$OUT"
    echo ">> Wrote $OUT" >&2
else
    go run ./cmd/parser-compare "${ARGS[@]}"
fi
