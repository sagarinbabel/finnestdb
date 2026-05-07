.PHONY: all build clean parser server frontend run run-local setup-local \
        import-dict-fi import-dict-et import-dict import-dict-et-ekilex import-ekilex-et \
        import-ekilex-details-et import-dict-et-recommended import-kotus-fi import-dict-fi-recommended \
        fetch-ekilex-refresh fetch-ekilex-sample fetch-ekilex \
        reduce-ekilex \
        gen-lemmatizer-tables-fi \
        reimport-dict-fi reimport-dict-et reimport-dict verify-dict \
        setup-omorfi setup-estnltk eval eval-check compare-parsers compare-parsers-et \
        import-ud-gold import-ud-gold-fi import-ud-gold-et \
        scrape-gutenberg-fi

# Default target
all: build

# Build Rust parser library
parser:
	cd parser && cargo build --release
	@echo "Rust parser built successfully"

# Build Go server
server: parser
	@echo "Building Go server..."
	go build -o finnestdb ./cmd/server

# Build everything
build: server
	@echo "Build complete! Run './finnestdb' to start the server"

# Run the server (builds if needed). Dictionary imports are optional and can be
# started separately with make import-dict-fi / make import-dict-et.
run: build
	./finnestdb

# Clean build artifacts
clean:
	cd parser && cargo clean
	rm -f finnestdb
	rm -f finnestdb.db
	rm -f web/app.js
	@echo "Clean complete"

# Install dependencies
deps:
	go mod download
	cd parser && cargo fetch

# ── FST-derived tables (no blobs in git, no derived tables in git) ────────────
#
# Policy: per docs/ARTIFACT_POLICY.md, neither upstream transducer blobs
# nor the factual tables generated from them are tracked. The runtime
# loads tables from localdata/lemmatizer-fi-et/tables/ on New(); this
# target generates them from a local mor.vfst.
#
# Example:
#   make gen-lemmatizer-tables-fi VFST_PATH=/path/to/mor.vfst
VFST_PATH ?=
gen-lemmatizer-tables-fi:
	@if [ -z "$(VFST_PATH)" ]; then \
		echo "VFST_PATH is required (local path to mor.vfst; do not commit)."; \
		exit 1; \
	fi
	@mkdir -p localdata/lemmatizer-fi-et/tables
	go run ./cmd/genlemmatizertables -lang fi -vfst "$(VFST_PATH)" \
	  -wordlist cmd/genlemmatizertables/wordlists/fi_smoke.txt \
	  -out localdata/lemmatizer-fi-et/tables/fi_min.json

# ── Dictionary import targets ──────────────────────────────────────────────────
# First-time import: skips if the table already has rows for that lang.
# Use reimport-dict-* to force a full refresh (drops existing rows first).

import-dict-fi:
	@if [ "$$(sqlite3 finnestdb.db "SELECT 1 FROM sqlite_master WHERE type='table' AND name='forms'" 2>/dev/null)" != "1" ] || \
	    [ "$$(sqlite3 finnestdb.db "SELECT COUNT(*) FROM forms WHERE lang='FI'" 2>/dev/null)" = "0" ]; then \
		echo "Importing Finnish dictionary from kaikki.org..."; \
		go run ./cmd/importdict -lang fi -db finnestdb.db; \
	else \
		echo "Finnish dictionary already imported. Run 'make reimport-dict-fi' to force refresh."; \
	fi

import-dict-et:
	@if [ "$$(sqlite3 finnestdb.db "SELECT 1 FROM sqlite_master WHERE type='table' AND name='forms'" 2>/dev/null)" != "1" ] || \
	    [ "$$(sqlite3 finnestdb.db "SELECT COUNT(*) FROM forms WHERE lang='ET'" 2>/dev/null)" = "0" ]; then \
		echo "Importing Estonian dictionary from kaikki.org..."; \
		go run ./cmd/importdict -lang et -db finnestdb.db; \
	else \
		echo "Estonian dictionary already imported. Run 'make reimport-dict-et' to force refresh."; \
	fi

import-dict-et-ekilex:
	@if [ -z "$$EKILEX_API_KEY" ]; then \
		echo "EKILEX_API_KEY is required. Create one in your Ekilex user profile, then export EKILEX_API_KEY=..."; \
		exit 1; \
	fi
	@echo "NOTE: Ekilex API import is best-effort and can be slow/flaky."
	@echo "Recommended ET workflow is: make import-dict-et && make import-ekilex-et && make import-ekilex-details-et"
	@echo "This target defaults to a small smoke import. Override EKILEX_LIMIT=... (or EKILEX_WORDS=...) if needed."
	go run ./cmd/importdict -lang et -source-key ekilex -source-priority 20 -db finnestdb.db \
		-source-name "EKI/Ekilex/Sõnaveeb" \
		-source-url "https://ekilex.ee" \
		-source-license "CC BY 4.0" \
		-source-attribution "Eesti Keele Instituut; EKI sõnastiku- ja terminibaasisüsteem Ekilex; Sõnaveeb" \
		-changes-note "Normalized to FinEstDB lemma/form/POS schema; monolingual definitions and translations flattened into gloss text" \
		-ekilex-limit $(EKILEX_LIMIT) \
		-ekilex-timeout $(EKILEX_TIMEOUT) \
		-ekilex-retries $(EKILEX_RETRIES) \
		$(if $(EKILEX_WORDS),-ekilex-words "$(EKILEX_WORDS)",)

# Defaults for the Ekilex API smoke import target.
EKILEX_LIMIT ?= 200
EKILEX_TIMEOUT ?= 90s
EKILEX_RETRIES ?= 3

# Import both languages (first-time only).
import-dict: import-dict-fi import-dict-et

# Recommended, reliable Estonian import for local/dev:
# - kaikki.org base dictionary
# - tracked compact Ekilex public headwords snapshot
# - tracked reduced Ekilex details drop (~6.2M forms)
import-dict-et-recommended: import-dict-et import-ekilex-et import-ekilex-details-et

# Recommended Finnish setup. Layered:
# - kaikki.org base dictionary (translations, glosses, forms)
# - Kotus Nykysuomen sanalista (paradigm_class for ~104k headwords; CC BY 4.0)
# Voikko paradigm seed (Phase 4) lands later — see docs/FINNISH_LEXICAL_PLAN.md.
import-dict-fi-recommended: import-dict-fi import-kotus-fi

# One-command local setup: build dictionaries if missing, then run.
# Kept separate from `make run` so running the server doesn't implicitly start
# multi-minute downloads/imports in CI or quick-dev loops.
# setup-local is the single bootstrap entry point for a fresh clone:
# fetches every third-party artifact into localdata/ (gitignored, see
# docs/ARTIFACT_POLICY.md), then imports everything into finnestdb.db.
# Pass SKIP_EKILEX_DETAILS=1 / SKIP_SILVER=1 / SKIP_UD=1 to opt out of
# the slow steps.
setup-local:
	@bash scripts/setup-local.sh

run-local: setup-local run

# Adds missing Estonian EKI ühendsõnastik 2026 public headwords from the
# tracked compact Ekilex snapshot. Existing Kaikki rows are preserved.
import-ekilex-et:
	go run ./cmd/importekilex -db finnestdb.db -file localdata/ekilex/eki-public-words-2026-et.jsonl

# Imports the rich Ekilex data drop (definitions/*.jsonl + forms/*.tsv,
# produced by `make reduce-ekilex`) into lemmas + forms. Loads ~178k lemma
# rows and ~6.2M form rows; runtime ~15s on a fast SSD. Empty-gloss guard
# preserves any pre-existing kaikki English glosses. POS attribution uses
# the form's morph_code to disambiguate homonyms — see
# cmd/importekilexdetails for the table.
import-ekilex-details-et:
	go run ./cmd/importekilexdetails -db finnestdb.db -data localdata/ekilex

# ── Kotus Nykysuomen sanalista (Finnish inflection class metadata) ────────────
# Phase 3 of docs/FINNISH_LEXICAL_PLAN.md. Reads the tracked TSV under
# localdata/kotus/ (CC BY 4.0; see localdata/kotus/NOTICE.md) and fills paradigm_class
# on existing FI lemmas. Existing rows from kaikki keep their source/gloss;
# only paradigm_class is set. Kotus-only headwords are inserted at
# source='kotus', priority=10. See cmd/importkotus for the full conflict
# policy.
import-kotus-fi:
	go run ./cmd/importkotus -db finnestdb.db -file localdata/kotus/nykysuomensanalista2024.txt

# ── Ekilex /api/word/details enrichment scrape ────────────────────────────────
# Requires EKILEX_API_KEY in the environment. Raw responses land under
# localdata/ekilex/details/ (gitignored). See cmd/fetchekilex.

# Refreshes localdata/ekilex/eki-public-words-2026-et.jsonl from /api/public_word/eki
# only if the headword set has changed.
fetch-ekilex-refresh:
	go run ./cmd/fetchekilex refresh-queue \
	  -out localdata/ekilex/eki-public-words-2026-et.jsonl

# Fetches a small spread of headwords with both /eki and the unfiltered
# variant so we can compare payload size/content before committing the full run.
fetch-ekilex-sample:
	go run ./cmd/fetchekilex sample \
	  -queue localdata/ekilex/eki-public-words-2026-et.jsonl \
	  -out-dir localdata/ekilex/details/samples

# Full resumable scrape. -rps is a *global* request rate cap shared across
# workers; -workers should be ~2x rps to keep request latency from becoming
# the bottleneck. Circuit breaker pauses workers after 10 consecutive failures
# and probes word_id=183007 (koer) every 5 minutes until the API recovers.
# Override any flag per invocation, e.g.:
#   make fetch-ekilex EKILEX_RPS=24 EKILEX_WORKERS=24
EKILEX_WORKERS ?= 16
EKILEX_RPS ?= 16
fetch-ekilex:
	go run ./cmd/fetchekilex fetch \
	  -queue localdata/ekilex/eki-public-words-2026-et.jsonl \
	  -out-dir localdata/ekilex/details \
	  -workers=$(EKILEX_WORKERS) -rps=$(EKILEX_RPS) -max-attempts=3

# Reduces the gzipped raw payloads under localdata/ekilex/details/raw/ into
# two sharded committable artifacts under localdata/ekilex/:
#   - definitions/<letter>.jsonl: per-word lemma + morphology + meanings
#   - forms/<letter>.tsv:         one row per inflected form (lemma, form, morph_code)
# Sharding is by first lowercase letter of the lemma (Estonian alphabet plus
# "_other"). Tests cover one fixture per inflection class encountered so far —
# run `go test ./cmd/reduceekilex/` to verify, or `go test ./cmd/reduceekilex/
# -update-golden` to refresh fixtures after intentional reducer changes.
reduce-ekilex:
	go run ./cmd/reduceekilex \
	  -raw-dir localdata/ekilex/details/raw \
	  -out-compact-dir localdata/ekilex/definitions \
	  -out-forms-dir localdata/ekilex/forms
# Full refresh: drops existing entries then re-imports.
reimport-dict-fi:
	go run ./cmd/importdict -lang fi -db finnestdb.db -reimport

reimport-dict-et:
	go run ./cmd/importdict -lang et -db finnestdb.db -reimport

reimport-dict: reimport-dict-fi reimport-dict-et

# verify-dict prints row counts per (table, lang, source) so you can quickly
# confirm a re-import populated what it should. Phase 2 (translations table)
# is hard to verify by eyeballing the UI — this gives a one-glance answer to
# "did kaikki translations land?" / "did Ekilex translations land?".
verify-dict:
	@echo "── lemmas ──"
	@sqlite3 -header -column finnestdb.db \
		"SELECT lang, source, source_priority, COUNT(*) AS rows FROM lemmas GROUP BY lang, source, source_priority ORDER BY lang, source_priority DESC, source"
	@echo
	@echo "── forms ──"
	@sqlite3 -header -column finnestdb.db \
		"SELECT lang, source, source_priority, COUNT(*) AS rows FROM forms GROUP BY lang, source, source_priority ORDER BY lang, source_priority DESC, source"
	@echo
	@echo "── translations ──"
	@if [ "$$(sqlite3 finnestdb.db "SELECT 1 FROM sqlite_master WHERE type='table' AND name='translations'" 2>/dev/null)" = "1" ]; then \
		sqlite3 -header -column finnestdb.db \
			"SELECT lang, target_lang, source, COUNT(*) AS rows FROM translations GROUP BY lang, target_lang, source ORDER BY lang, source"; \
	else \
		echo "(empty)"; \
	fi
	@echo
	@echo "── definitions ──"
	@if [ "$$(sqlite3 finnestdb.db "SELECT 1 FROM sqlite_master WHERE type='table' AND name='definitions'" 2>/dev/null)" = "1" ]; then \
		sqlite3 -header -column finnestdb.db \
			"SELECT lang, source, COUNT(*) AS rows FROM definitions GROUP BY lang, source ORDER BY lang, source"; \
	else \
		echo "(empty)"; \
	fi
	@echo
	@echo "── dict_metadata ──"
	@if [ "$$(sqlite3 finnestdb.db "SELECT 1 FROM sqlite_master WHERE type='table' AND name='dict_metadata'" 2>/dev/null)" = "1" ]; then \
		if [ "$$(sqlite3 finnestdb.db "SELECT 1 FROM pragma_table_info('dict_metadata') WHERE name='source_name'" 2>/dev/null)" = "1" ]; then \
			sqlite3 -header -column finnestdb.db \
				"SELECT lang, source, source_name, source_version, imported_at FROM dict_metadata ORDER BY lang, source"; \
		else \
			sqlite3 -header -column finnestdb.db \
				"SELECT lang, source, imported_at FROM dict_metadata ORDER BY lang, source"; \
		fi; \
	else \
		echo "(empty)"; \
	fi

# ── Omorfi comparison setup ────────────────────────────────────────────────────
#
# Installs the Helsinki Finite-State Transducer toolkit, the omorfi Python
# package, and downloads the analyser model into ~/.cache/omorfi/. After
# this target completes, the omorfi parser is available end-to-end:
#
#     go run ./cmd/parsertest -dataset DS.json -parsers basic,custom,omorfi
#
# No environment variables need to be exported — the bundled adapter at
# scripts/omorfi_adapter_example.py auto-discovers the model, and parsecore
# auto-defaults FINNESTDB_OMORFI_CMD when the script is present.

OMORFI_VERSION := 0.9.12
OMORFI_CACHE   := $(HOME)/.cache/omorfi
OMORFI_MODEL   := $(OMORFI_CACHE)/omorfi.analyse.hfst

setup-omorfi: $(OMORFI_MODEL)
	@python3 -c "from omorfi import Omorfi; o = Omorfi(); o.load_analyser('$(OMORFI_MODEL)'); print('omorfi: OK')"

$(OMORFI_MODEL):
	@echo "Installing HFST + omorfi…"
	@command -v apt-get >/dev/null && apt-get install -y python3-hfst hfst >/dev/null || true
	@pip3 install --quiet omorfi
	@mkdir -p $(OMORFI_CACHE)
	@if [ ! -f $(OMORFI_CACHE)/models.tar.xz ]; then \
		echo "Downloading omorfi $(OMORFI_VERSION) HFST models…"; \
		curl -sL --max-time 600 -o $(OMORFI_CACHE)/models.tar.xz \
		    https://github.com/flammie/omorfi/releases/download/v$(OMORFI_VERSION)/omorfi-hfst-models-$(OMORFI_VERSION).tar.xz; \
	fi
	@cd $(OMORFI_CACHE) && tar xf models.tar.xz
	@echo "Models installed at $(OMORFI_CACHE)"

# ── Side-by-side parser comparison ─────────────────────────────────────────────
#
# Runs basic, custom, and (if setup-omorfi has been done) omorfi against the
# Finnish gold datasets and produces a markdown comparison table.

compare-parsers: parser
	@bash scripts/parser-comparison.sh

# ── Estonian analyzer comparison setup ────────────────────────────────────────
#
# Installs EstNLTK, which provides Vabamorf-backed Estonian morphological
# analysis. After this target completes, the estnltk parser is available:
#
#     go run ./cmd/parsertest -dataset testdata/parser-eval/et/gold/et-manual-v1.json -parsers basic,custom,estnltk
#
# The bundled adapter at scripts/estnltk_adapter_example.py is auto-discovered,
# or can be overridden with FINNESTDB_ESTNLTK_CMD.

setup-estnltk:
	@python3 -m venv .venv-estnltk
	@.venv-estnltk/bin/python -m pip install --quiet --upgrade pip
	@.venv-estnltk/bin/python -m pip install --quiet estnltk
	@mkdir -p .cache/nltk_data .cache/matplotlib
	@NLTK_DATA="$$(pwd)/.cache/nltk_data" .venv-estnltk/bin/python -m nltk.downloader -d .cache/nltk_data punkt_tab >/dev/null
	@NLTK_DATA="$$(pwd)/.cache/nltk_data" MPLCONFIGDIR="$$(pwd)/.cache/matplotlib" .venv-estnltk/bin/python -c "from estnltk import Text; t = Text('Poes ootasin sõpra.'); t.tag_layer(['words', 'morph_analysis']); print('estnltk: OK')"

compare-parsers-et: parser
	@bash scripts/parser-comparison-et.sh

# ── UD treebank gold-set ingest (Plan C / PR 1) ──────────────────────────────
#
# Clone Universal Dependencies treebanks and project them into our parser-eval
# gold JSON format. ~1M gold tokens combined (FI + ET, train+dev+test).
#
# FI dev/test files are committed under testdata/parser-eval/fi/gold/ (CC BY
# / CC BY-SA). ET dev/test and all train splits live under
# localdata/parser-eval/{fi,et}/{gold,gold-train}/ — gitignored. ET sources
# are CC BY-NC-SA so derivatives can't be redistributed; train splits are
# kept local because they're large (12k–25k cases each) and only used for
# OOV/coverage analysis. See docs/data_enhancement.md for the full ledger.
#
# Re-running is idempotent and fast (~30s) — the heavy step is the initial
# clone (~50 MB per treebank, cached under localdata/ud-cache/).
import-ud-gold-fi:
	@bash scripts/fetch-and-import-ud.sh fi

import-ud-gold-et:
	@bash scripts/fetch-and-import-ud.sh et

import-ud-gold: import-ud-gold-fi import-ud-gold-et

# Run the standard local parser eval sweep without requiring external baselines.
# Globs both the committed gold (testdata/parser-eval/) and any local-only
# gold a fresh setup-local.sh has produced (localdata/parser-eval/).
eval: parser
	@export LD_LIBRARY_PATH="$$(pwd)/parser/target/release:$${LD_LIBRARY_PATH:-}"; \
	for ds in testdata/parser-eval/*/gold/*.json localdata/parser-eval/*/gold/*.json; do \
		[ -f "$$ds" ] || continue; \
		echo "== $$ds =="; \
		go run ./cmd/parsertest -dataset "$$ds" -parsers basic,custom -warmup 1 -repeat 3; \
	done

# CI-friendly alias for the same eval sweep documented in the alpha checklist.
eval-check: eval

# ── Silver corpus scraping (Plan C / PR 3) ───────────────────────────────────
#
# Fetches Finnish-language books from Project Gutenberg into the silver-tier
# corpus at localdata/silver-fi/. Polite scraper (1.5s between requests).
# Idempotent — re-running skips books already in the manifest.
#
# The default target (500k tokens) covers ~14 books from the most popular
# l.fi search results. Override with TARGET_TOKENS=N to fetch more or less.
TARGET_TOKENS ?= 500000
scrape-gutenberg-fi:
	go run ./cmd/scrapegutenberg \
	    -lang fi \
	    -target-tokens $(TARGET_TOKENS) \
	    -out localdata/silver-fi/raw \
	    -manifest localdata/silver-fi/manifest.jsonl
