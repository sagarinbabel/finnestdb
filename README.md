# FinEstDB

A language learning application for Finnish and Estonian using spaced repetition (FSRS).

## Stub Project

This is a stub implementation demonstrating the core UI and basic parsing functionality.

### Quick Start

1. **Prerequisites**: Go 1.21+, Rust (latest stable), SQLite
2. **Build**: `make build` (or `make` for short)
3. **Run**: `make run` (or `./finnestdb`)
4. **Access**: Open `http://localhost:8080` in your browser
5. **Login**: Use any email/password (mock authentication)

See [docs/GETTING_STARTED.md](docs/GETTING_STARTED.md) for detailed setup instructions.

### Features (Stub)

- ✅ Mock login screen
- ✅ Dashboard with KPI tiles
- ✅ Text upload and deck creation
- ✅ Basic Rust parser (tokenization & sentence splitting)
- ✅ Review interface with all buttons (Again, Hard, Good, Easy, Ignore, Mark as Known)
- ✅ Light/dark theme toggle
- ✅ Responsive UI

### Project Structure

- `/cmd/server` - Go HTTP server
- `/internal/api` - API handlers
- `/internal/parserffi` - CGO bindings to Rust parser
- `/internal/store` - SQLite database layer
- `/parser` - Rust parser library (basic tokenization)
- `/web` - Frontend (HTML, CSS, JavaScript)

### Documentation

- [Getting Started Guide](docs/GETTING_STARTED.md)
- [PRD (Alpha)](finnestdb-prd-alpha.md)

## License

MIT
