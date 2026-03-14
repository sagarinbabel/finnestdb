# Getting Started with FinEstDB

This guide will help you set up and run the current parser-focused FinEstDB app locally.

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
  - [Stub Limitations](#stub-limitations)
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

2. The app opens directly to the **Parse Text** page.
   - No login is required on the current `main` branch.
   - Choose **Finnish (FI)** or **Estonian (ET)**.
   - Paste text or load a `.txt` / `.md` file.

## Using the Application

### Parsing Text

1. Paste text into the textarea or load a `.txt` / `.md` file.
2. Keep the input under **300,000 Unicode characters**.
3. Choose one parser mode:
   - **Basic Parser**: direct dictionary lookup only after stub parsing
   - **Custom Parser**: adds possessive, compound, and case-suffix enrichment rules
4. Click the parser button you want to run.
5. Review the results page:
   - parser mode used
   - coverage score
   - parse duration
   - sortable word list with lemmas, forms, definitions, grammar labels, and token counts

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
- Try deleting `finnestdb.db` and restarting (will create a new database)

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

### Stub Limitations

This is a parser-focused implementation with the following limitations:

- **Current UI**: parse page and results page only
- **Parser core**: Rust tokenization and heuristic POS guessing, not real morphology
- **Basic parser mode**: direct dictionary lookup only
- **Custom parser mode**: rule-based enrichment, still not Omorfi/Vabamorf
- **Review system**: backend stubs exist, but the current frontend does not expose dashboard/review flows
- **Accounts**: not part of the current main flow

### Next Steps

Current direction:

1. Improve parser quality and observability
2. Define parser evaluation metrics and annotation workflow
3. Build a repeatable parser test harness
4. Design a richer lexical knowledge layer on top of dictionary data
5. Return to accounts, known-word tracking, and review once parser quality is strong enough

## Support

For issues or questions, please check:
- The PRD document: `finnestdb-prd-alpha.md`
- Server logs for detailed error messages
- Browser console for frontend errors

## License

MIT License - See LICENSE file for details.
