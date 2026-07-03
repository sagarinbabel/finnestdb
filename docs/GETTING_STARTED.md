# Getting Started with FinnEst

This guide will help you set up and run the current role-aware FinnEst alpha
app locally.

If you are Michael (or an agent working for him): the condensed run/test path
plus a question-routing table lives in [`FOR_MICHAEL.md`](FOR_MICHAEL.md).
For a fully populated dictionary database on a fresh clone, use
`make run-local` (bootstrap script) or copy an existing `finnestdb.db`.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation Steps](#installation-steps)
  - [1. Clone the Repository](#1-clone-the-repository)
  - [2. Install Dependencies](#2-install-dependencies)
  - [3. Build the Project](#3-build-the-project)
- [Running the Server](#running-the-server)
  - [Using Make](#using-make)
  - [Manual Run](#manual-run)
- [Accessing the Application](#accessing-the-application)
- [Using the Application](#using-the-application)
  - [Parsing Text](#parsing-text)
  - [Theme Toggle](#theme-toggle)
- [Project Structure](#project-structure)
- [Troubleshooting](#troubleshooting)
  - [Build Errors](#build-errors)
  - [Runtime Errors](#runtime-errors)
  - [Browser Issues](#browser-issues)
- [Development Notes](#development-notes)
  - [Alpha Limitations](#alpha-limitations)
  - [Next Steps](#next-steps)
- [Support](#support)
- [License](#license)

## Prerequisites

Before you begin, ensure you have the following installed:

1. **Go** (version 1.21 or later)
   - Download from: https://go.dev/dl/
   - Verify installation: `go version`

2. **Rust** (latest stable version)
   - Install via: https://rustup.rs/
   - Verify installation: `rustc --version` and `cargo --version`

3. **SQLite** (usually pre-installed on macOS/Linux)
   - Verify installation: `sqlite3 --version`
   - On Windows, download from: https://www.sqlite.org/download.html

4. **Make** (optional, for using Makefile)
   - macOS: Pre-installed or via Xcode Command Line Tools
   - Linux: `sudo apt-get install build-essential` (Ubuntu/Debian)
   - Windows: Use WSL or build manually (see below)

## Installation Steps

### 1. Clone the Repository

```bash
git clone <repository-url>
cd finnestdb
```

### 2. Install Dependencies

#### Go Dependencies
```bash
go mod download
```

#### Rust Dependencies
```bash
cd parser
cargo fetch
cd ..
```

### 3. Build the Project

#### Using Make (Recommended)

```bash
make build
```

This will:
1. Build the Rust parser library (`parser/target/release/libparser.a`)
2. Build the Go server executable (`finnestdb`)

#### Manual Build

If you don't have Make, build manually:

```bash
# 1. Build Rust parser
cd parser
cargo build --release
cd ..

# 2. Build Go server
go build -o finnestdb ./cmd/server
```

## Running the Server

### Using Make

```bash
make run
```

### Manual Run

```bash
./finnestdb
```

Or with custom port/database:

```bash
./finnestdb -port 8080 -db mydb.db
```

The server will start on `http://localhost:8080` by default.

## Accessing the Application

1. Open your browser and navigate to: `http://localhost:8080`

2. The app opens to the public landing page.
   - Sign in with an email address and password (8+ chars) to use Dashboard, Parse, Decks, and Review.
   - Auth uses Argon2id password hashes and DB-backed `session_token` sessions.
   - Admin users can open the internal parser workbench.

## Using the Application

### Parsing Text

1. Sign in and open **Parse**.
2. Paste text into the textarea or load a `.txt`, `.md`, or `.epub` file.
3. Keep the input under **1,500,000 Unicode characters**.
4. Click **Parse text**.
5. Review the results page:
   - dictionary coverage
   - sortable word list with lemmas, forms, definitions, grammar labels, and token counts
   - correction buttons for logged-in users
   - save-as-deck flow

Language handling policy:

- high-confidence pasted or file-loaded text warns and requires an explicit language switch
- if the selected language conflicts with detected Finnish or Estonian, parse is blocked until you switch
- unknown-language warnings are advisory and do not block parse

Admins can use **Admin → Workbench** to compare the Basic and Custom parser
modes and inspect parser timing details.

### Theme Toggle

Click the theme toggle button (🌙/☀️) in the header to switch between light and dark modes.

## Project Structure

```
finnestdb/
├── cmd/server/          # Go server main entry point
├── internal/
│   ├── api/             # HTTP handlers
│   ├── parserffi/       # CGO bindings to Rust parser
│   └── store/           # SQLite database layer
├── parser/              # Rust parser library
│   ├── src/lib.rs       # Parser implementation
│   └── Cargo.toml       # Rust dependencies
├── web/                 # Frontend files
│   ├── index.html       # Main HTML page
│   ├── app.ts           # TypeScript application
│   └── styles.css       # Styling
├── docs/                # Documentation
├── Makefile             # Build automation
└── go.mod               # Go dependencies
```

## Troubleshooting

### Build Errors

**Rust build fails:**
- Ensure Rust is installed: `rustc --version`
- Try: `cd parser && cargo clean && cargo build --release`

**Go build fails:**
- Ensure Go is installed: `go version`
- Try: `go mod tidy && go build -o finnestdb ./cmd/server`

**CGO/linking errors:**
- Ensure the Rust library is built first: `cd parser && cargo build --release`
- Check that `parser/target/release/libparser.a` exists (Linux/macOS) or `parser/target/release/parser.lib` (Windows)
- On macOS, you may need Xcode Command Line Tools: `xcode-select --install`
- If linking fails, try setting `CGO_ENABLED=1` explicitly: `CGO_ENABLED=1 go build -o finnestdb ./cmd/server`
- On some systems, you may need to adjust the library path in `internal/parserffi/bindings.go`

### Runtime Errors

**"Database error":**
- Check file permissions for the database file
- On a fresh clone only: deleting `finnestdb.db` and restarting creates a new
  empty database. Never do this to a populated multi-GB database — that is the
  real dictionary. Small ~100 KB `finnestdb.db` files are auto-created stubs.

**"Parse error":**
- Ensure the Rust parser library is built
- Check server logs for detailed error messages

**Port already in use:**
- Use a different port: `./finnestdb -port 8081`
- Or stop the process using port 8080

### Browser Issues

**TypeScript not compiling:**
- Install frontend dependencies once: `cd web && npm install`
- Rebuild the frontend: `cd web && npm run build`
- If the build still fails, check the TypeScript error output directly

**Styles not loading:**
- Ensure `web/styles.css` exists
- Check browser console for 404 errors

## Development Notes

### Alpha Limitations

This is an alpha implementation with the following limitations:

- **Auth/hardening**: password auth, server-side sessions, CSRF/Origin checks,
  rate limits, and retention/deletion controls shipped (2026-07-02 launch
  stack). Remaining launch gates live in `TODO.md` "Public alpha gates" and
  `docs/GO_LIVE_CHECKLIST.md`.
- **Parser core**: Rust tokenization plus dictionary/FST-backed lemmatization
  in `custom` mode; not Omorfi/Vabamorf (those are eval baselines)
- **Admin parser modes**: Basic is direct dictionary lookup; Custom adds the
  full enrichment pipeline
- **Retention**: Inspect parses are ephemeral until saved as a deck or
  submitted as parser feedback; the History page can delete retained parse
  sessions, and account deletion cascades server-side

### Next Steps

Current direction: start from [`../TODO.md` "LLM handoff read
order"](../TODO.md#llm-handoff-read-order) — it points at the public-alpha
gates, launch issue ledger, go/no-go rubric, and implementation specs.

## Support

For issues or questions, please check:
- [`INDEX.md`](INDEX.md) — the documentation map ([`FOR_MICHAEL.md`](FOR_MICHAEL.md) for the question-routing table)
- Server logs for detailed error messages
- Browser console for frontend errors

## License

MIT License - See LICENSE file for details.
