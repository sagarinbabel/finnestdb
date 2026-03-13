.PHONY: all build clean parser server frontend run \
        import-dict-fi import-dict-et import-dict \
        reimport-dict-fi reimport-dict-et reimport-dict

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
	@if [ "$$(sqlite3 finnestdb.db 'SELECT COUNT(*) FROM forms WHERE lang="FI" LIMIT 1' 2>/dev/null)" = "0" ]; then \
		echo "Importing Finnish dictionary from kaikki.org..."; \
		go run ./cmd/importdict -lang fi -db finnestdb.db; \
	else \
		echo "Finnish dictionary already imported. Run 'make reimport-dict-fi' to force refresh."; \
	fi

import-dict-et:
	@if [ "$$(sqlite3 finnestdb.db 'SELECT COUNT(*) FROM forms WHERE lang="ET" LIMIT 1' 2>/dev/null)" = "0" ]; then \
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
