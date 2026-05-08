#!/usr/bin/env bash
#
# Fetch UD treebanks from the UniversalDependencies GitHub org and run
# cmd/importud over each train/dev/test split, producing parser-eval gold
# JSON files. Plan C / PR 1 — see docs/CHANGELOG.md 2026-05-06c.
#
# Treebanks fetched (clone --depth 1, ~50 MB each):
#
#   FI:
#     UD_Finnish-TDT — CC BY-SA 4.0      — train + dev + test
#     UD_Finnish-FTB — CC BY 4.0         — train + dev + test
#     UD_Finnish-PUD — CC BY-SA 3.0      — test only
#     UD_Finnish-OOD — CC BY-SA 4.0      — test only
#
#   ET:
#     UD_Estonian-EDT — CC BY-NC-SA 4.0  — train + dev + test
#     UD_Estonian-EWT — CC BY-NC-SA 4.0  — train + dev + test
#
# Output:
#
#   testdata/parser-eval/fi/gold/ud-fi-<treebank>-<split>-v1.json.gz (committed; CC BY / CC BY-SA)
#   localdata/parser-eval/et/gold/ud-et-<treebank>-<split>-v1.json   (gitignored; CC BY-NC-SA)
#
# Train splits and the cloned UD treebank cache live entirely under
# localdata/ — see docs/ARTIFACT_POLICY.md and docs/data_enhancement.md.
# Auto-discovery in scripts/parser-comparison{,-et}.sh now globs both
# testdata/parser-eval/<lang>/gold/ AND localdata/parser-eval/<lang>/gold/
# so the local-only files are picked up too.
#
# Single-folder bootstrap: zipping localdata/ + finnestdb.db captures
# every fetched/derived artifact this script produces.
#
# Usage:
#
#   scripts/fetch-and-import-ud.sh fi          # fetch + import FI only
#   scripts/fetch-and-import-ud.sh et          # fetch + import ET only
#   scripts/fetch-and-import-ud.sh both        # both
#   scripts/fetch-and-import-ud.sh both --no-fetch  # use existing data/ud-cache/
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

WHICH="${1:-both}"
NO_FETCH=false
shift || true
while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-fetch) NO_FETCH=true; shift ;;
        *) echo "unknown flag: $1" >&2; exit 1 ;;
    esac
done

UD_CACHE="$ROOT/localdata/ud-cache"
mkdir -p "$UD_CACHE"

clone_or_update() {
    local repo="$1"
    local target="$UD_CACHE/$repo"
    if $NO_FETCH; then
        if [[ ! -d "$target" ]]; then
            echo "missing $target and --no-fetch set" >&2
            exit 1
        fi
        return 0
    fi
    if [[ -d "$target/.git" ]]; then
        echo ">> updating $repo" >&2
        git -C "$target" pull --ff-only --quiet
    else
        echo ">> cloning $repo" >&2
        git clone --depth 1 --quiet "https://github.com/UniversalDependencies/$repo.git" "$target"
    fi
}

# import_split TREEBANK SPLIT LANG OUTDIR
#
# Looks for $UD_CACHE/$TREEBANK/<lang-prefix>_<treebank-suffix>-ud-$SPLIT.conllu
# and runs cmd/importud over it. Skips silently if the file isn't present
# (some treebanks ship test only).
import_split() {
    local treebank="$1"  # UD_Finnish-TDT
    local split="$2"     # train|dev|test
    local lang="$3"      # FI|ET
    local outdir="$4"

    # Treebank repos use a "<langlower>_<suffix>-ud-<split>.conllu" naming
    # convention. UD_Finnish-TDT → fi_tdt-ud-test.conllu. Compute the
    # filename prefix algorithmically.
    local suffix="${treebank#UD_*-}"
    local lang_lower
    lang_lower="$(echo "$lang" | tr '[:upper:]' '[:lower:]')"
    local file_stem="${lang_lower}_$(echo "$suffix" | tr '[:upper:]' '[:lower:]')-ud-${split}.conllu"
    local infile="$UD_CACHE/$treebank/$file_stem"
    if [[ ! -f "$infile" ]]; then
        echo ">> skip: $treebank $split — $file_stem not present" >&2
        return 0
    fi

    # ud-fi-tdt-test-v1.json
    local name="ud-${lang_lower}-$(echo "$suffix" | tr '[:upper:]' '[:lower:]')-${split}-v1"
    local outfile="$outdir/$name.json"
    local final_out="$outfile"
    if [[ "$outdir" == "testdata/parser-eval/fi/gold" ]]; then
        final_out="$outfile.gz"
    fi
    mkdir -p "$outdir"
    echo ">> import $infile → $final_out" >&2
    go run ./cmd/importud \
        -input "$infile" \
        -output "$outfile" \
        -name "$name" \
        -version v1 \
        -lang "$lang"
    if [[ "$final_out" != "$outfile" ]]; then
        gzip -n -9 -f "$outfile"
    fi
}

import_fi() {
    for tb in UD_Finnish-TDT UD_Finnish-FTB; do
        clone_or_update "$tb"
        # Dev/test go under testdata/ (committed gold). Train splits are
        # 12k–15k cases each — kept under localdata/ so they don't bloat
        # headline parser-comparison runs (held-out discipline) and so a
        # single-folder zip of localdata/ captures every local artifact.
        import_split "$tb" train FI "localdata/parser-eval/fi/gold-train"
        import_split "$tb" dev   FI "testdata/parser-eval/fi/gold"
        import_split "$tb" test  FI "testdata/parser-eval/fi/gold"
    done
    for tb in UD_Finnish-PUD UD_Finnish-OOD; do
        clone_or_update "$tb"
        # PUD / OOD ship test only.
        import_split "$tb" test FI "testdata/parser-eval/fi/gold"
    done
}

import_et() {
    for tb in UD_Estonian-EDT UD_Estonian-EWT; do
        clone_or_update "$tb"
        # ET treebanks are CC BY-NC-SA — derived gold cannot be
        # redistributed from this permissively-licensed code repo, so
        # all three splits live under localdata/ (gitignored).
        import_split "$tb" train ET "localdata/parser-eval/et/gold-train"
        import_split "$tb" dev   ET "localdata/parser-eval/et/gold"
        import_split "$tb" test  ET "localdata/parser-eval/et/gold"
    done
}

case "$WHICH" in
    fi)   import_fi ;;
    et)   import_et ;;
    both) import_fi; import_et ;;
    *)    echo "usage: $0 [fi|et|both] [--no-fetch]" >&2; exit 1 ;;
esac

echo ">> done." >&2
