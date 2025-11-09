# Getting Started with FinEstDB

This guide will help you set up and run the FinEstDB stub project locally.

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
  - [Creating a Deck](#creating-a-deck)
  - [Reviewing Cards](#reviewing-cards)
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

2. **Login**: Use the mock login form
   - **Email**: Any email address (e.g., `test@example.com`)
   - **Password**: Any password (e.g., `password`)
   - Note: This is a stub - any credentials will work!

3. **Dashboard**: After logging in, you'll see:
   - KPI tiles showing Words Known, To Review, and To Learn
   - "Learn" button to start reviewing cards
   - "Upload & Create Deck" button to create a new deck
   - List of your decks

## Using the Application

### Creating a Deck

1. Click **"Upload & Create Deck"** button
2. Fill in the form:
   - **Title**: Give your deck a name (required)
   - **Language**: Select Finnish (FI) or Estonian (ET)
   - **Text**: Paste your text or upload a `.txt` or `.md` file (max 2 MB)
3. Click **"Create Deck"**
4. The text will be parsed using the Rust parser (basic tokenization and sentence splitting)

### Reviewing Cards

1. Click **"Learn"** button from the dashboard
2. You'll see a card with:
   - **Front**: Sentence with highlighted word (or word only)
   - Click **"Show Answer"** to flip the card
   - **Back**: Shows lemma, meaning, grammar label, and example sentences
3. Use the review buttons:
   - **Again** (0): Failed recall
   - **Hard** (1): Hesitant recall
   - **Good** (2): Normal recall (default)
   - **Easy** (3): Instant recall
   - **Ignore**: Hide this word forever
   - **Mark as Known**: Treat as already known

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
- The project uses vanilla TypeScript - ensure your browser supports ES modules
- Modern browsers (Chrome, Firefox, Safari, Edge) should work
- Check browser console for errors

**Styles not loading:**
- Ensure `web/styles.css` exists
- Check browser console for 404 errors

## Development Notes

### Stub Limitations

This is a stub implementation with the following limitations:

- **Authentication**: Mock login only - any credentials work
- **Parser**: Basic tokenization and sentence splitting only (no morphological analysis, POS tagging, or MWE detection)
- **FSRS**: Review buttons exist but don't update scheduling (no actual spaced repetition algorithm)
- **Card Data**: Uses mock data for review interface
- **Database**: Simplified schema (core tables only)

### Next Steps

To extend this stub:

1. Implement real morphological analysis in Rust parser
2. Add FSRS-6 algorithm for spaced repetition
3. Implement proper authentication with password hashing
4. Add email verification and password reset
5. Implement MWE (multi-word expression) detection
6. Add sentence generation with inflection

## Support

For issues or questions, please check:
- The PRD document: `finnestdb-prd-alpha.md`
- Server logs for detailed error messages
- Browser console for frontend errors

## License

MIT License - See LICENSE file for details.

