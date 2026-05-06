.PHONY: all build clean parser server frontend run \
        import-dict-fi import-dict-et import-dict import-ekilex-et \
        fetch-ekilex-refresh fetch-ekilex-sample fetch-ekilex \
        reduce-ekilex \
        reimport-dict-fi reimport-dict-et reimport-dict \
        setup-omorfi eval eval-check compare-parsers

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

# Import both languages (first-time only).
import-dict: import-dict-fi import-dict-et

# Adds missing Estonian EKI ühendsõnastik 2026 public headwords from the
# tracked compact Ekilex snapshot. Existing Kaikki rows are preserved.
import-ekilex-et:
	go run ./cmd/importekilex -db finnestdb.db -file data/ekilex/eki-public-words-2026-et.jsonl

# ── Ekilex /api/word/details enrichment scrape ────────────────────────────────
# Requires EKILEX_API_KEY in the environment. Raw responses land under
# localdata/ekilex/details/ (gitignored). See cmd/fetchekilex.

# Refreshes data/ekilex/eki-public-words-2026-et.jsonl from /api/public_word/eki
# only if the headword set has changed.
fetch-ekilex-refresh:
	go run ./cmd/fetchekilex refresh-queue

# Fetches a small spread of headwords with both /eki and the unfiltered
# variant so we can compare payload size/content before committing the full run.
fetch-ekilex-sample:
	go run ./cmd/fetchekilex sample

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
	  -workers=$(EKILEX_WORKERS) -rps=$(EKILEX_RPS) -max-attempts=3

# Reduces the gzipped raw payloads under localdata/ekilex/details/raw/ into
# two committable artifacts under localdata/ekilex/details/:
#   - details-compact.jsonl: per-word lemma + morphology + meanings
#   - forms.tsv:             one row per inflected form (lemma, form, morph_code)
# Outputs stay in localdata/ until reviewed; tests cover one fixture per
# inflection class encountered so far. Run `go test ./cmd/reduceekilex/` to
# verify the reducer or `go test ./cmd/reduceekilex/ -update-golden` to refresh
# fixtures after intentional reducer changes.
reduce-ekilex:
	go run ./cmd/reduceekilex

# Full refresh: drops existing entries then re-imports.
reimport-dict-fi:
	go run ./cmd/importdict -lang fi -db finnestdb.db -reimport

reimport-dict-et:
	go run ./cmd/importdict -lang et -db finnestdb.db -reimport

reimport-dict: reimport-dict-fi reimport-dict-et

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

# Run the standard local parser eval sweep without requiring external baselines.
eval: parser
	@export LD_LIBRARY_PATH="$$(pwd)/parser/target/release:$${LD_LIBRARY_PATH:-}"; \
	for ds in testdata/parser-eval/*/gold/*.json; do \
		[ -f "$$ds" ] || continue; \
		echo "== $$ds =="; \
		go run ./cmd/parsertest -dataset "$$ds" -parsers basic,custom -warmup 1 -repeat 3; \
	done

# CI-friendly alias for the same eval sweep documented in the alpha checklist.
eval-check: eval
