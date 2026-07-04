.PHONY: all build clean parser server frontend run run-local setup-local \
        import-dict-fi import-dict-et import-dict import-dict-et-ekilex import-ekilex-et \
        import-ekilex-details-et import-dict-et-recommended import-kotus-fi import-dict-fi-recommended \
        fetch-ekilex-refresh fetch-ekilex-sample fetch-ekilex \
        reduce-ekilex \
        gen-lemmatizer-tables-fi gen-lemmatizer-tables-et gen-lemmatizer-wordlist-fi \
        gen-catalog gen-catalog-check \
        reimport-dict-fi reimport-dict-et reimport-dict verify-dict db-invariants \
        setup-omorfi setup-estnltk eval eval-watch eval-check compare-parsers compare-parsers-et \
        compare-ambiguity \
        import-ud-gold import-ud-gold-fi import-ud-gold-et \
        scrape-gutenberg-fi \
        fetch-frequency-baselines \
        doctor live-api-smoke purge-parse-context \
        import-gold-surfaces export-gold-candidates \
        first-experience-rc

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

# ── doctor: one-shot health report for a working copy ────────────────────────
#
# Reports DB presence + per-source row counts, FST table presence, analyzer
# venv presence (omorfi / estnltk), Ekilex shard presence, UD cache,
# frequency baselines, and the Rust parser shared library. Returns 0 unless
# the DB or the FI/ET dictionary is missing entirely. Everything else is
# informational so the user understands the *degraded modes* their setup
# implies, rather than discovering them from surprise eval numbers.
doctor:
	@go run ./cmd/doctor

# ── FST-derived tables (no blobs in git, no derived tables in git) ────────────
#
# Policy: per docs/ARTIFACT_POLICY.md, neither upstream transducer blobs
# nor the factual tables generated from them are tracked. The runtime
# loads tables from localdata/lemmatizer-fi-et/tables/ on New(); this
# target generates them from a local mor.vfst.
#
# Example:
#   make gen-lemmatizer-tables-fi VFST_PATH=/path/to/mor.vfst
#   make gen-lemmatizer-tables-et
#   make gen-lemmatizer-tables-et HFSTOL_PATH=/path/to/analyser-gt-desc.hfstol
VFST_PATH ?=
HFSTOL_PATH ?= localdata/lemmatizer-fi-et/analyser-gt-desc.hfstol
FI_WORDLIST ?= localdata/lemmatizer-fi-et/wordlists/fi.txt
gen-catalog:
	@if [ ! -f finnestdb.db ]; then \
		echo "finnestdb.db is required (run scripts/setup-local.sh first)."; \
		exit 1; \
	fi
	go run ./cmd/gencatalog -specs internal/catalog/specs.json -data internal/catalog/data -db finnestdb.db -freq-dir localdata/frequency -reviews internal/catalog/reviews.json -out internal/catalog/data/catalog.json

# Reproducibility guard: fails if the checked-in catalog drifts from a fresh
# regeneration (ignores only the "generated" date).
gen-catalog-check:
	@if [ ! -f finnestdb.db ]; then \
		echo "finnestdb.db is required (run scripts/setup-local.sh first)."; \
		exit 1; \
	fi
	go run ./cmd/gencatalog -specs internal/catalog/specs.json -data internal/catalog/data -db finnestdb.db -freq-dir localdata/frequency -reviews internal/catalog/reviews.json -out internal/catalog/data/catalog.json -check

gen-lemmatizer-wordlist-fi:
	@if [ ! -f finnestdb.db ]; then \
		echo "finnestdb.db is required (run scripts/setup-local.sh first)."; \
		exit 1; \
	fi
	@mkdir -p localdata/lemmatizer-fi-et/wordlists
	go run ./cmd/genlemmatizerwordlist -db finnestdb.db -lang fi -out "$(FI_WORDLIST)"

gen-lemmatizer-tables-fi:
	@if [ -z "$(VFST_PATH)" ]; then \
		echo "VFST_PATH is required (local path to mor.vfst; do not commit)."; \
		exit 1; \
	fi
	@if [ ! -f "$(FI_WORDLIST)" ]; then \
		echo "FI wordlist missing at $(FI_WORDLIST); generating from finnestdb.db…"; \
		$(MAKE) gen-lemmatizer-wordlist-fi; \
	fi
	@mkdir -p localdata/lemmatizer-fi-et/tables
	go run ./cmd/genlemmatizertables -lang fi -vfst "$(VFST_PATH)" \
	  -wordlist "$(FI_WORDLIST)" \
	  -out localdata/lemmatizer-fi-et/tables/fi_min.json

# Estonian: same policy as FI. Source analyser is Giellalt's
# lang-est-x-utee analyser-gt-desc.hfstol, kept locally; the generated
# table goes into localdata/lemmatizer-fi-et/tables/et_min.json and is
# not committed.
HFSTOL_PATH ?= localdata/lemmatizer-fi-et/analyser-gt-desc.hfstol
gen-lemmatizer-tables-et:
	@if [ -z "$(HFSTOL_PATH)" ] || [ ! -f "$(HFSTOL_PATH)" ]; then \
		echo "HFSTOL_PATH must point to analyser-gt-desc.hfstol (local-only; do not commit)."; \
		echo "Default checked: localdata/lemmatizer-fi-et/analyser-gt-desc.hfstol"; \
		echo "Run 'make doctor' and read docs/LOCAL_TOOLING.md before assuming the analyser is absent."; \
		exit 1; \
	fi
	@mkdir -p localdata/lemmatizer-fi-et/tables
	go run ./cmd/genlemmatizertables -lang et -hfstol "$(HFSTOL_PATH)" \
	  -wordlist cmd/genlemmatizertables/wordlists/et_smoke.txt \
	  -out localdata/lemmatizer-fi-et/tables/et_min.json

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
		-changes-note "Normalized to FinnEst lemma/form/POS schema; monolingual definitions and translations flattened into gloss text" \
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

db-invariants:
	@bash scripts/db-invariants.sh finnestdb.db

live-api-smoke:
	@node scripts/live-api-smoke.mjs

RETENTION_DAYS ?= 30
PURGE_PARSE_CONTEXT_FLAGS ?=
purge-parse-context:
	go run ./cmd/purgeparsecontext -db finnestdb.db -older-than-days "$(RETENTION_DAYS)" $(PURGE_PARSE_CONTEXT_FLAGS)

# Correction-loop Phase 4 guard data: load the frozen gold sets so accepting
# a correction that contradicts them is refused. Re-run after gold changes.
import-gold-surfaces:
	go run ./cmd/importgoldsurfaces -db finnestdb.db

# Correction-loop Phase 3 review: print pending gold-case promotions.
export-gold-candidates:
	go run ./cmd/exportgoldcandidates -db finnestdb.db

# ── NLP tool setup (unified venv) ─────────────────────────────────────────────
#
# Installs omorfi (Finnish morphological analyzer) and EstNLTK (Estonian
# Vabamorf-backed analyzer) into a single shared `.venv/` at the project
# root, plus downloads the omorfi HFST models into ~/.cache/omorfi/.
#
# After `make setup-nlp`:
#     go run ./cmd/parsertest -dataset DS.json -parsers basic,custom,omorfi
#     go run ./cmd/parsertest -dataset DS.json -parsers basic,custom,estnltk
#
# evalparsers.go auto-discovers .venv/bin/python at runtime, so no env vars
# are needed. Legacy .venv-omorfi/ and .venv-estnltk/ names are also
# checked for backward compat.

NLP_VENV       := .venv
OMORFI_VERSION := 0.9.12
OMORFI_CACHE   := $(HOME)/.cache/omorfi
OMORFI_MODEL   := $(OMORFI_CACHE)/omorfi.analyse.hfst

setup-nlp: $(OMORFI_MODEL)
	@if [ ! -x $(NLP_VENV)/bin/python ]; then \
		echo "Creating $(NLP_VENV)/ …"; \
		python3 -m venv $(NLP_VENV); \
		$(NLP_VENV)/bin/python -m pip install --quiet --upgrade pip; \
	fi
	@echo "Installing omorfi + estnltk …"
	@$(NLP_VENV)/bin/python -m pip install --quiet omorfi estnltk
	@mkdir -p .cache/nltk_data .cache/matplotlib
	@NLTK_DATA="$$(pwd)/.cache/nltk_data" $(NLP_VENV)/bin/python -m nltk.downloader -d .cache/nltk_data punkt_tab >/dev/null
	@$(NLP_VENV)/bin/python -c "from omorfi import Omorfi; o = Omorfi(); o.load_analyser('$(OMORFI_MODEL)'); print('omorfi: OK')"
	@NLTK_DATA="$$(pwd)/.cache/nltk_data" MPLCONFIGDIR="$$(pwd)/.cache/matplotlib" $(NLP_VENV)/bin/python -c "from estnltk import Text; t = Text('Poes ootasin sõpra.'); t.tag_layer(['words', 'morph_analysis']); print('estnltk: OK')"

# Legacy aliases — point at the unified venv.
setup-omorfi: setup-nlp
setup-estnltk: setup-nlp

$(OMORFI_MODEL):
	@echo "Installing HFST + omorfi model cache…"
	@command -v apt-get >/dev/null && apt-get install -y python3-hfst hfst >/dev/null || true
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

compare-parsers-et: parser
	@bash scripts/parser-comparison-et.sh

# ── Ambiguity and meaning-check calibration eval ──────────────────────────────
#
# Runs cmd/ambiguityeval over the committed ambiguity gold slice
# (testdata/parser-eval/*-ambiguity/*.json) against the production DB and
# reports candidate-inclusion / selection-accuracy / proxy-stratified
# accuracy per ambiguity_class. See docs/PARSER_EVAL_METHODOLOGY.md
# §"Ambiguity and meaning-check calibration" for what this measures and the
# threshold rule it feeds.

compare-ambiguity: parser
	@bash scripts/compare-ambiguity.sh

# ── First-experience release-candidate pack ──────────────────────────────────
#
# Runs the automated half of the first-experience RC pack described in
# docs/GO_LIVE_CHECKLIST.md "First-experience quality check": the Go runner
# (parser fixture checks) followed by the RC Playwright spec (browser
# checks). Both consume the single canonical manifest at
# testdata/first-experience-rc/manifest.json. Finishes by pointing at the
# manual walkthrough instructions, which live in that checklist section, not
# in a separate doc.
#
# Nonzero exit only if an automated case fails; pending/manual cases don't
# fail the run (see cmd/firstexperiencerc).
first-experience-rc: parser
	@export LD_LIBRARY_PATH="$$(pwd)/parser/target/release:$${LD_LIBRARY_PATH:-}"; \
	go run ./cmd/firstexperiencerc
	@export LD_LIBRARY_PATH="$$(pwd)/parser/target/release:$${LD_LIBRARY_PATH:-}"; \
	cd web && npx playwright test tests/first-experience-rc.spec.ts
	@echo "Manual walkthrough: see docs/GO_LIVE_CHECKLIST.md 'First-experience quality check' — instructions live there (no separate doc)."

# ── Manual gold UD FEATS enrichment ──────────────────────────────────────────
#
# Seeds UD FEATS into the manual gold sets by running omorfi (FI) and estnltk
# (ET) on each case's text and overlaying per-token FEATS, then deterministically
# anchoring Case= to the gold's existing grammar_label. Updates the gold
# JSONs in place and writes a `.diff.md` next to each file flagging tokens
# that need a manual look (OOV compounds, case disagreements).
#
# Re-runnable: existing FEATS are overwritten, so this is the single source
# of truth for refreshing FEATS after improvements to the adapters or maps.
#
# Requires the unified NLP venv: `make setup-nlp` first.

enrich-gold-feats: parser
	@go run ./cmd/enrichgoldfeats -all

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

# ── Local parser eval sweeps ──────────────────────────────────────────────────
#
# Two flavors:
#
#   make eval        — held-out test sets only (baseline discipline). Skips
#                      gold/<name>-dev-v*.json files the same way
#                      `make compare-parsers{,-et}` do. This is the right
#                      target for CI and "is my change ready to land" checks.
#
#   make eval-watch  — test + dev splits. Use this in the per-commit watch
#                      loop while iterating on a fix; dev sets are noisier
#                      but catch regressions earlier. Don't quote numbers
#                      from this in PR bodies — they include unfrozen dev
#                      data.
#
# Both glob the committed gold under testdata/parser-eval/ and any local-only
# gold a fresh setup-local.sh has produced under localdata/parser-eval/. See
# docs/PARSER_EVAL_METHODOLOGY.md "Held-out discipline" for why dev/test are
# split.
eval: parser
	@export LD_LIBRARY_PATH="$$(pwd)/parser/target/release:$${LD_LIBRARY_PATH:-}"; \
	for ds in testdata/parser-eval/*/gold/*.json testdata/parser-eval/*/gold/*.json.gz localdata/parser-eval/*/gold/*.json localdata/parser-eval/*/gold/*.json.gz; do \
		[ -f "$$ds" ] || continue; \
		case "$$ds" in *-dev-v*.json|*-dev-v*.json.gz) continue ;; esac; \
		echo "== $$ds =="; \
		go run ./cmd/parsertest -dataset "$$ds" -parsers basic,custom -warmup 1 -repeat 3; \
	done

eval-watch: parser
	@export LD_LIBRARY_PATH="$$(pwd)/parser/target/release:$${LD_LIBRARY_PATH:-}"; \
	for ds in testdata/parser-eval/*/gold/*.json testdata/parser-eval/*/gold/*.json.gz localdata/parser-eval/*/gold/*.json localdata/parser-eval/*/gold/*.json.gz; do \
		[ -f "$$ds" ] || continue; \
		echo "== $$ds =="; \
		go run ./cmd/parsertest -dataset "$$ds" -parsers basic,custom -warmup 1 -repeat 3; \
	done

# CI-friendly alias. Matches `eval` (test-only) so a green CI run never
# depends on dev-set numbers.
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

# Fetch public Finnish/Estonian frequency baselines into localdata/frequency/.
# Hermit Dave OpenSubtitles 2018 (CC BY-SA 4.0) + UD-FI-TDT (CC BY-SA 4.0) +
# UD-ET-EDT (CC BY-NC-SA 4.0). Idempotent: skip files that exist.
# Methodology, coverage curves, and license attribution: docs/FREQUENCY_BASELINES.md.
fetch-frequency-baselines:
	go run ./cmd/fetchfrequency
