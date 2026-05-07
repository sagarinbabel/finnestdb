#!/usr/bin/env bash
#
# scripts/setup-local.sh — single-entry-point bootstrap for a fresh clone.
#
# Fetches every third-party data artifact into ./localdata/ (gitignored),
# generates derived tables, and populates ./finnestdb.db so that
# `./finnestdb` can serve immediately.
#
# Per docs/ARTIFACT_POLICY.md, none of the data this script writes is
# tracked in git. Re-running is idempotent — sources that are already
# present are skipped.
#
# Steps:
#   1.  Build the Rust parser shared library (`make parser`)
#   2.  mkdir localdata/ subtree
#   3.  Fetch Kotus Nykysuomen sanalista (FI, CC BY 4.0, ~2.7 MB)
#   4.  Fetch Ekilex public-word snapshot (ET, CC BY 4.0, ~16 MB)
#   5.  IF $EKILEX_API_KEY set: scrape Ekilex /api/word/details (long;
#       budget several hours) → reduce → ~287 MB of shards under
#       localdata/ekilex/{definitions,forms}/.
#       IF NOT set: skip with a clear note. Setup still succeeds; the
#       parser just runs without the rich Ekilex enrichment until
#       someone re-runs with a key.
#   6.  Fetch UD treebanks (FI committed, ET local-only) via
#       scripts/fetch-and-import-ud.sh.
#   7.  Scrape Gutenberg-FI silver corpus (~4 MB) via
#       cmd/scrapegutenberg.
#   8.  Import all of the above into finnestdb.db.
#
# Usage:
#   scripts/setup-local.sh                   # full setup
#   SKIP_EKILEX_DETAILS=1 scripts/setup-local.sh   # skip step 5 even with key
#   SKIP_SILVER=1 scripts/setup-local.sh           # skip step 7
#   SKIP_UD=1 scripts/setup-local.sh               # skip step 6
#   EKILEX_RPS=24 EKILEX_WORKERS=24 scripts/setup-local.sh   # tune scrape
#
# After this finishes, you can hand a teammate a fast-bootstrap zip:
#   tar czf finnestdb-bootstrap.tgz localdata/ finnestdb.db
# They drop it next to the repo, untar, and skip every fetch.
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

log()  { printf '\033[1;34m[setup-local]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[setup-local]\033[0m %s\n' "$*" >&2; }
fail() { printf '\033[1;31m[setup-local]\033[0m %s\n' "$*" >&2; exit 1; }

# ── prereqs ───────────────────────────────────────────────────────────────────
command -v go      >/dev/null || fail "go not found in PATH"
command -v curl    >/dev/null || fail "curl not found in PATH"
command -v sqlite3 >/dev/null || fail "sqlite3 not found in PATH"
command -v cargo   >/dev/null || fail "cargo not found in PATH (needed for the Rust parser)"

# ── plan summary (so the user knows what's about to happen) ───────────────────
log "── plan ────────────────────────────────────────────────────────────────"
log "Will fetch into ./localdata/ (gitignored), populate ./finnestdb.db."
if [[ -n "${EKILEX_API_KEY:-}" ]]; then
    log "  EKILEX_API_KEY: set    → full Ekilex /api/word/details scrape (multi-hour)."
else
    log "  EKILEX_API_KEY: unset  → skipping Ekilex details. Server still runs;"
    log "                          ET coverage reduced. Get a free key at"
    log "                          https://ekilex.ee/ and re-run when ready."
fi
[[ -n "${SKIP_EKILEX_DETAILS:-}" ]] && log "  SKIP_EKILEX_DETAILS=1  → forced-skip Ekilex details."
[[ -n "${SKIP_UD:-}"             ]] && log "  SKIP_UD=1              → skipping UD treebanks (parser-eval only)."
[[ -n "${SKIP_SILVER:-}"         ]] && log "  SKIP_SILVER=1          → skipping Gutenberg-FI silver (parser-eval only)."
log ""
log "Tip: if you only need the dictionary path running and don't care about"
log "     parser-eval datasets, re-run with: SKIP_UD=1 SKIP_SILVER=1"
log "──────────────────────────────────────────────────────────────────────────"

# ── 1. build Rust parser ──────────────────────────────────────────────────────
log "Building Rust parser…"
make parser

# ── 2. localdata/ tree ────────────────────────────────────────────────────────
log "Preparing localdata/ subtree"
mkdir -p localdata/{kotus,ekilex/{details/raw,definitions,forms},silver-fi/raw}

# ── 3. Kotus Nykysuomen sanalista ─────────────────────────────────────────────
KOTUS_FILE="localdata/kotus/nykysuomensanalista2024.txt"
if [[ -f "$KOTUS_FILE" ]]; then
    log "Kotus sanalista already present at $KOTUS_FILE — skipping fetch."
else
    log "Fetching Kotus Nykysuomen sanalista 2024 (CC BY 4.0)…"
    curl -fsSL --max-time 120 \
        -o "$KOTUS_FILE" \
        https://kaino.kotus.fi/lataa/nykysuomensanalista2024.txt
    log "  → $(wc -l < "$KOTUS_FILE") lines."
fi

# ── 4. Ekilex public-word snapshot ────────────────────────────────────────────
EKILEX_QUEUE="localdata/ekilex/eki-public-words-2026-et.jsonl"
if [[ -f "$EKILEX_QUEUE" ]]; then
    log "Ekilex public-word snapshot already present — skipping refresh."
else
    log "Fetching Ekilex public-word snapshot (ET, /api/public_word/eki)…"
    # Note: the no-key path is announced up front in the plan header. If the
    # endpoint rejects an unauthenticated request we just skip — the parser
    # still works without ET enrichment.
    make fetch-ekilex-refresh || true
    if [[ ! -f "$EKILEX_QUEUE" ]]; then
        warn "Ekilex public-word snapshot not retrieved (this is expected without"
        warn "  EKILEX_API_KEY). Setup continues; ET headword enrichment is skipped."
    fi
fi

# ── 5. Ekilex /api/word/details (the long part) ───────────────────────────────
if [[ -n "${SKIP_EKILEX_DETAILS:-}" ]]; then
    warn "SKIP_EKILEX_DETAILS set — skipping Ekilex details scrape + reduce."
elif [[ -z "${EKILEX_API_KEY:-}" ]]; then
    warn "EKILEX_API_KEY not set — skipping Ekilex details scrape."
    warn "   Without it, parser still runs but lacks rich ET morphology + glosses."
    warn "   Get a free key at https://ekilex.ee/, then re-run this script."
elif [[ ! -f "$EKILEX_QUEUE" ]]; then
    warn "Ekilex queue missing at $EKILEX_QUEUE — skipping details scrape."
    warn "   Re-run after fetch-ekilex-refresh succeeds."
elif [[ -d "localdata/ekilex/definitions" && -n "$(ls -A localdata/ekilex/definitions 2>/dev/null)" ]]; then
    log "Ekilex reduced shards already present at localdata/ekilex/{definitions,forms}/ — skipping fetch."
else
    log "Scraping Ekilex /api/word/details (this can take several hours)…"
    log "  Tune via EKILEX_WORKERS / EKILEX_RPS env vars."
    make fetch-ekilex
    log "Reducing raw payloads → sharded definitions/forms…"
    make reduce-ekilex
fi

# ── 6. UD treebanks ───────────────────────────────────────────────────────────
if [[ -n "${SKIP_UD:-}" ]]; then
    warn "SKIP_UD set — skipping UD treebank fetch."
elif [[ -d "data/ud-cache" && -n "$(ls -A data/ud-cache 2>/dev/null)" ]]; then
    log "UD treebanks already present at data/ud-cache/ — skipping fetch."
else
    log "Fetching UD treebanks (FI + ET)…"
    bash scripts/fetch-and-import-ud.sh both
fi

# ── 7. Gutenberg-FI silver ────────────────────────────────────────────────────
if [[ -n "${SKIP_SILVER:-}" ]]; then
    warn "SKIP_SILVER set — skipping Gutenberg silver scrape."
elif [[ -f "localdata/silver-fi/manifest.jsonl" ]]; then
    log "Gutenberg-FI silver already present — skipping scrape."
else
    log "Scraping Gutenberg-FI silver corpus (~500k tokens, ~4 MB)…"
    make scrape-gutenberg-fi
fi

# ── 8. populate finnestdb.db ──────────────────────────────────────────────────
log "Importing dictionaries into finnestdb.db…"
make import-dict-fi-recommended
make import-dict-et
if [[ -f "$EKILEX_QUEUE" ]]; then
    make import-ekilex-et
else
    warn "Skipping import-ekilex-et because $EKILEX_QUEUE is missing."
fi
if [[ -d "localdata/ekilex/definitions" && -n "$(ls -A localdata/ekilex/definitions 2>/dev/null)" && \
      -d "localdata/ekilex/forms" && -n "$(ls -A localdata/ekilex/forms 2>/dev/null)" ]]; then
    make import-ekilex-details-et
else
    warn "Skipping import-ekilex-details-et because reduced Ekilex shards are missing."
fi

# ── done ──────────────────────────────────────────────────────────────────────
log ""
log "Setup complete."
log ""
log "Repo state:"
log "  finnestdb.db        — populated SQLite, ~$(du -h finnestdb.db 2>/dev/null | awk '{print $1}')"
log "  localdata/kotus/    — Kotus sanalista, $(du -sh localdata/kotus 2>/dev/null | awk '{print $1}')"
log "  localdata/ekilex/   — Ekilex shards, $(du -sh localdata/ekilex 2>/dev/null | awk '{print $1}')"
log "  localdata/silver-fi/ — Gutenberg silver, $(du -sh localdata/silver-fi 2>/dev/null | awk '{print $1}')"
log "  data/ud-cache/      — UD treebank clones, $(du -sh data/ud-cache 2>/dev/null | awk '{print $1}')"
log ""
log "To bootstrap a teammate fast (skip every fetch):"
log "  tar czf finnestdb-bootstrap.tgz localdata/ finnestdb.db"
log "  # send the .tgz; they untar in repo root and run ./finnestdb"
log ""
log "Next: 'make build && ./finnestdb' to start the server."
