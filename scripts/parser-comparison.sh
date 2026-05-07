#!/usr/bin/env bash
#
# Run a side-by-side comparison of basic, custom, and omorfi parsers across
# the Finnish gold datasets, then assemble the results into a markdown
# comparison report.
#
# Omorfi is **required by default** — every Finnish baseline report must show
# the analyzer column so dict-only numbers are never read in isolation. Run
# `make setup-omorfi` once on a fresh machine to install it. To bypass on a
# machine where omorfi cannot be installed, pass --allow-missing-baseline
# (intended for ad-hoc local experiments only — never for committed reports).
#
# Usage:
#   scripts/parser-comparison.sh                                       # default datasets, stdout
#   scripts/parser-comparison.sh -o report.md                          # write to file
#   scripts/parser-comparison.sh DS1.json DS2.json ...                 # custom datasets
#   scripts/parser-comparison.sh --allow-missing-baseline ...          # skip omorfi (escape hatch)
#   scripts/parser-comparison.sh --stratified ...                      # add UPOS/OOV/compound breakdowns
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
    # Default discovery: every fi/gold/*.json EXCEPT dev splits. Test sets
    # are the held-out headline; dev is for per-commit "watch" eval (run
    # explicitly with -dataset). Train splits live under gold-train/ and
    # never auto-discover. See docs/PARSER_EVAL_DATASETS.md for the
    # held-out discipline introduced in PR Plan-C/1.
    #
    # We glob both testdata/parser-eval/fi/gold/ (committed FI gold) and
    # localdata/parser-eval/fi/gold/ (any local-only FI gold a teammate
    # has dropped in via setup-local.sh). The same merge applies on the
    # ET side — see scripts/parser-comparison-et.sh.
    while IFS= read -r f; do DATASETS+=("$f"); done \
        < <(ls testdata/parser-eval/fi/gold/*.json localdata/parser-eval/fi/gold/*.json 2>/dev/null \
            | grep -v -- '-dev-v' | sort)
fi

if [[ ${#DATASETS[@]} -eq 0 ]]; then
    echo "no datasets to compare" >&2
    exit 1
fi

PARSERS="basic,custom"

# Auto-detect omorfi. Run modes the user might have configured:
#   - $FINNESTDB_OMORFI_CMD set (explicit adapter override) — wins over everything
#   - .venv-omorfi/bin/python + scripts/omorfi_adapter_example.py (the
#     setup that `make setup-omorfi` produces; mirrors how
#     scripts/parser-comparison-et.sh detects .venv-estnltk)
#   - $OMORFI_ANALYSE_HFST set and pointing at a real file (custom model
#     location — the adapter itself looks here first)
#   - Repo-local cache:   ./.cache/omorfi/omorfi.analyse.hfst
#   - User-level cache:   ~/.cache/omorfi/omorfi.analyse.hfst
omorfi_available=false
if [[ -n "${FINNESTDB_OMORFI_CMD:-}" ]]; then
    omorfi_available=true
elif [[ -x .venv-omorfi/bin/python && -f scripts/omorfi_adapter_example.py ]]; then
    export FINNESTDB_OMORFI_CMD="$ROOT/.venv-omorfi/bin/python $ROOT/scripts/omorfi_adapter_example.py"
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
elif $ALLOW_MISSING; then
    echo ">> WARNING: omorfi missing, --allow-missing-baseline set — running dict-only" >&2
    echo ">>          Do NOT commit reports produced this way; analyzer parity is required." >&2
else
    cat >&2 <<'EOF'
ERROR: omorfi is required for Finnish parser comparison but is not available.

Every committed FI baseline must include the omorfi column so dict-only
numbers (basic/custom) are never read without the analyzer upper bound.

To install:
    make setup-omorfi

If you genuinely need a dict-only run for local experimentation, pass:
    --allow-missing-baseline

Other override paths (any one is enough):
    export FINNESTDB_OMORFI_CMD="..."
    export OMORFI_ANALYSE_HFST=/abs/path/to/omorfi.analyse.hfst
    place the file at ./.cache/omorfi/omorfi.analyse.hfst
    place the file at $HOME/.cache/omorfi/omorfi.analyse.hfst
EOF
    exit 2
fi

export LD_LIBRARY_PATH="$ROOT/parser/target/release:${LD_LIBRARY_PATH:-}"

REPORTS_DIR="$ROOT/reports/parser-eval"
mkdir -p "$REPORTS_DIR"

# Track which reports came out of *this* run so parser-compare doesn't pick up
# stale ones from previous runs.
THIS_RUN_REPORTS=()
RUN_TS="$(date -u +%Y%m%dT%H%M%SZ)"

PARSERTEST_EXTRA=()
COMPARE_EXTRA=()
if $STRATIFIED; then
    PARSERTEST_EXTRA+=(-stratified)
    COMPARE_EXTRA+=(-stratified)
fi

for ds in "${DATASETS[@]}"; do
    # Slug from the dataset *filename* (not the JSON `name` field).
    # Multiple gold files can share the same internal `name` (e.g.
    # fi-manual-v1.json and fi-manual-v2.json both declare
    # name="fi-manual"); using the field as the slug would silently
    # overwrite the first report with the second. Filenames are unique
    # by definition. Slugify so a path with spaces or weird chars can't
    # escape REPORTS_DIR.
    base="$(basename "$ds" .json)"
    slug="$(printf '%s' "$base" | python3 -c "
import re, sys
raw = sys.stdin.read().strip()
slug = re.sub(r'[^A-Za-z0-9._-]+', '-', raw).strip('-') or 'unnamed'
print(slug[:80])
")"
    out="$REPORTS_DIR/${RUN_TS}-${slug}.json"
    echo ">> $ds → $slug" >&2
    go run ./cmd/parsertest -dataset "$ds" -parsers "$PARSERS" -warmup 2 -repeat 5 -out "$out" "${PARSERTEST_EXTRA[@]}" >&2
    THIS_RUN_REPORTS+=("$out")
done

ARGS=(-title "Parser comparison ($RUN_TS, parsers: $PARSERS)")
ARGS+=("${COMPARE_EXTRA[@]}")
ARGS+=("${THIS_RUN_REPORTS[@]}")

if [[ -n "$OUT" ]]; then
    go run ./cmd/parser-compare "${ARGS[@]}" > "$OUT"
    echo ">> Wrote $OUT" >&2
else
    go run ./cmd/parser-compare "${ARGS[@]}"
fi
