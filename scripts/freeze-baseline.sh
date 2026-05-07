#!/usr/bin/env bash
#
# scripts/freeze-baseline.sh — copy a parser-eval run from
# reports/parser-eval/ into docs/baselines/ under the canonical filename
# convention and PARSER_EVOLUTION.md/SYSTEM_VERSIONING.md cross-reference.
#
# Filename produced:
#   docs/baselines/YYYY-MM-DD<rev>-THHMMZ-<dataset>.<ext>
#
# Where:
#   <rev>     parser-version letter from internal/parsecore/parsecore.go's
#             ParserVersion constant (e.g. "2026.05.07k" → "k")
#   YYYY-MM-DD UTC date derived from the comparison-script RUN_TS
#   THHMMZ    UTC hour+minute derived from RUN_TS
#   <dataset> the gold-set slug each report was produced under (matches
#             scripts/parser-comparison{,-et}.sh's slug-from-file-basename
#             post-PR #146)
#
# **Append-only.** This script REFUSES to overwrite an existing target
# file in docs/baselines/. Older baselines stay valid cross-references
# (PR descriptions, commit messages, PARSER_EVOLUTION.md entries all
# point at past filenames). If you need to re-freeze the same run for a
# different reason, re-run the comparison and pass the new RUN_TS.
#
# Usage:
#   scripts/freeze-baseline.sh <RUN_TS>
#
# Where <RUN_TS> is the YYYYMMDDTHHMMSSZ stamp the comparison scripts
# emitted (visible as the prefix on the report filenames they wrote
# under reports/parser-eval/). Look at the most recent
# reports/parser-eval/<RUN_TS>-*.json filenames to find it.
#
# Override the parser-version letter (e.g. when freezing a measurement
# you ran *before* a parser-version bump): -rev <letter>.
#
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

REV_OVERRIDE=""
RUN_TS=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        -rev|--rev)        REV_OVERRIDE="$2"; shift 2 ;;
        -h|--help)         sed -n '2,32p' "$0"; exit 0 ;;
        *)                 RUN_TS="$1"; shift ;;
    esac
done

if [[ -z "$RUN_TS" ]]; then
    echo "usage: scripts/freeze-baseline.sh <RUN_TS like 20260507T111800Z> [-rev <letter>]" >&2
    exit 64
fi

# RUN_TS shape produced by the comparison scripts: `date -u +%Y%m%dT%H%M%SZ`
# → YYYYMMDDTHHMMSSZ (no internal dashes). Validate before slicing.
if ! [[ "$RUN_TS" =~ ^[0-9]{8}T[0-9]{6}Z$ ]]; then
    echo "fatal: RUN_TS must be YYYYMMDDTHHMMSSZ (e.g. 20260507T111800Z), got: $RUN_TS" >&2
    exit 64
fi

DATE_FMT="${RUN_TS:0:4}-${RUN_TS:4:2}-${RUN_TS:6:2}"   # 20260507 → 2026-05-07
TIME_HHMM="${RUN_TS:9:4}"                              # T1118SSZ → 1118
T_STAMP="T${TIME_HHMM}Z"                               # → T1118Z

if [[ -n "$REV_OVERRIDE" ]]; then
    REV="$REV_OVERRIDE"
else
    # ParserVersion = "2026.05.07k" → k (single trailing lowercase letter).
    REV="$(grep -oE 'ParserVersion = "[0-9.]+[a-z]"' internal/parsecore/parsecore.go \
            | grep -oE '"[0-9.]+[a-z]"' \
            | sed -E 's/.*([a-z])"$/\1/' || true)"
fi

if [[ -z "$REV" ]] || ! [[ "$REV" =~ ^[a-z]$ ]]; then
    echo "fatal: could not parse a single-letter version suffix from internal/parsecore/parsecore.go" >&2
    echo "       Override with: scripts/freeze-baseline.sh $RUN_TS -rev <letter>" >&2
    exit 1
fi

PREFIX="${DATE_FMT}${REV}-${T_STAMP}"
mkdir -p docs/baselines

echo ">> Freezing reports/parser-eval/${RUN_TS}-* → docs/baselines/${PREFIX}-*" >&2

shopt -s nullglob
copied=0

# Per-dataset JSONs.
for f in "reports/parser-eval/${RUN_TS}"-*.json; do
    base="$(basename "$f")"
    dataset="${base#${RUN_TS}-}"             # strip RUN_TS prefix
    out="docs/baselines/${PREFIX}-${dataset}"
    if [[ -e "$out" ]]; then
        echo "fatal: $out already exists; baselines are append-only." >&2
        echo "       Re-run the comparison with a fresh RUN_TS instead." >&2
        exit 1
    fi
    cp "$f" "$out"
    echo "   $out" >&2
    copied=$((copied + 1))
done

# Cross-language summary md files. The comparison scripts emit them only
# when invoked with -o; tolerate either presence pattern. Try both the
# DATE-based name (legacy) and any -fi.md/-et.md the scripts wrote.
for lang in fi et; do
    for cand in \
            "reports/parser-eval/${DATE_FMT}-${lang}.md" \
            "reports/parser-eval/${RUN_TS}-${lang}.md"; do
        if [[ -f "$cand" ]]; then
            out="docs/baselines/${PREFIX}-${lang}.md"
            if [[ -e "$out" ]]; then
                echo "fatal: $out already exists; baselines are append-only." >&2
                exit 1
            fi
            cp "$cand" "$out"
            echo "   $out" >&2
            copied=$((copied + 1))
            break
        fi
    done
done

if [[ "$copied" -eq 0 ]]; then
    echo "warn: no input files matched. Did you pass the right RUN_TS, and did the comparison scripts actually run?" >&2
    exit 2
fi

echo >&2
echo "Done — froze $copied file(s) under prefix ${PREFIX}." >&2
echo "Next steps:" >&2
echo "  1. Add a row to docs/PARSER_EVOLUTION.md trend table + a ### ${DATE_FMT}${REV}-${T_STAMP} entry." >&2
echo "  2. Add a row at the top of docs/SYSTEM_VERSIONING.md § Parser evaluation baseline history." >&2
echo "  3. git add docs/baselines/${PREFIX}-* and commit." >&2
