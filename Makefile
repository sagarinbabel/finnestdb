.PHONY: all build clean parser server frontend run \
        import-dict-fi import-dict-et import-dict \
        reimport-dict-fi reimport-dict-et reimport-dict \
        setup-omorfi compare-parsers

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
