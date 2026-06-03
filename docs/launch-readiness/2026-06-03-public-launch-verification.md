# Public Launch Verification - 2026-06-03

Scope: current workspace at `/Users/sagar/Downloads/projects/finnestdb`, local `main`
aligned with `origin/main` plus the uncommitted public-launch blocker changes.

## Summary

- PASS: local tooling health via `make doctor`.
- PASS: Go, Rust, corpus-pipeline, TypeScript, Playwright browser, parser-comparison, race, `go vet`, live API smoke, and database invariant checks.
- FAIL: `govulncheck` reports reachable Go standard-library vulnerabilities because local Go is `go1.25.4`; public release should build with Go `1.25.11` or newer.
- PASS: `cargo audit` is now installed and the parser crate lockfile audit passed.
- DEFERRED: exhaustive Codex Security repository-wide scan is pending explicit subagent authorization. Non-subagent smoke/security probes completed.

## Test Results

| Check | Result | Notes |
|---|---:|---|
| `make doctor` | PASS | DB, FI/ET dictionaries, FST tables, NLP venvs, Ekilex shards, UD cache, frequency baselines, Rust parser dylib all present. |
| `go test ./...` | PASS | Root module tests passed. |
| `cargo test --release` | PASS | Parser crate: 42 tests passed. |
| `go test ./...` in `corpus_pipeline/` | PASS | Corpus pipeline tests passed. |
| `npm run build` in `web/` | PASS | TypeScript build passed. |
| `npm test` in `web/` | PASS | Playwright: 108 browser tests passed, including parse-history access, cancel/delete, bulk delete, and empty-state coverage. |
| `make compare-parsers` | PASS | FI comparison completed; reports written with timestamp `20260603T183545Z`. |
| `make compare-parsers-et` | PASS | ET comparison completed; reports written with timestamp `20260603T183545Z`. |
| Live API smoke/security probes | PASS | 15/15 passed. |
| `make live-api-smoke` | PASS | Repeatable script passed against `http://127.0.0.1:8092`; 15/15 checks. |
| `go test -race ./internal/api ./internal/auth ./internal/store` | PASS | Race run passed. |
| `make db-invariants` | PASS | Integrity ok; checked forms, definitions, translations, cards, decks, parse feedback, parse sessions, known/ignored overlap, and source breakdown. |
| Secret-pattern scan | PASS | No obvious API keys/private keys/secrets found outside ignored runtime artifacts and dependencies. |
| `npm audit` / `npm audit --omit=dev` | PASS | 0 vulnerabilities. |
| `go vet ./...` | PASS | Fixed `cmd/doctor/main_test.go` to avoid `testing.Chdir` so module Go 1.21 compatibility is preserved. |
| `go test ./cmd/doctor` | PASS | Focused regression for the vet-compatible helper. |
| `go test ./cmd/server` | PASS | Production DB readiness guard rejects missing, empty, stub, and undersized DBs before server startup; explicit degraded override is covered. |
| `govulncheck ./...` | FAIL | Reachable Go stdlib vulnerabilities from Go 1.25.4; fixed by newer Go patch releases. |
| `govulncheck ./...` in `corpus_pipeline/` | FAIL | Same Go stdlib issue; corpus pipeline also reaches `archive/tar` vulnerable code under current Go. |
| `cargo audit` in `parser/` | PASS | Installed `cargo-audit v0.22.1`; scanned `Cargo.lock` with 15 crate dependencies and no vulnerabilities reported. |

## Live Smoke Coverage

The live probe against `http://127.0.0.1:8092` verified:

- registration sets a session cookie
- `/api/me` resolves authenticated user
- non-admin `/api/admin/users` returns 403
- authenticated `/api/parse` succeeds and remains ephemeral
- anonymous `/api/parse` remains allowed as documented
- foreign-origin authenticated mutation is rejected
- same-origin authenticated mutation is allowed
- parser feedback creates retained parse context
- parse history lists retained sessions
- parse history per-row delete works
- parse history is empty after deletion
- oversized parse request is rejected
- login rate limit triggers
- account deletion succeeds
- deleted account session is no longer authenticated

## Security Findings

### S1 - Build/runtime Go version has reachable vulnerabilities

`govulncheck` reports reachable vulnerabilities in the Go standard library for
the current local toolchain `go1.25.4`. Public launch builds should use Go
`1.25.11` or newer.

Examples reported in the root module:

- `GO-2026-5039` in `net/textproto`, fixed in `go1.25.11`
- `GO-2026-5037` in `crypto/x509`, fixed in `go1.25.11`
- `GO-2026-4971` in `net`, fixed in `go1.25.10`
- `GO-2026-4918` in `net/http`, fixed in `go1.25.10`
- `GO-2026-4601` in `net/url`, fixed in `go1.25.8`; reachable through `api.sameOrigin`
- `GO-2026-4341` in `net/url`, fixed in `go1.25.6`; reachable through multipart/query parsing

Corpus pipeline scan reports the same Go patch-level issue plus a reachable
`archive/tar` issue in `cmd/extractcorpus/extract_leipzig.go`.

Recommended action: pin CI/release builders to Go `1.25.11+` and re-run
`govulncheck ./...` in both root and `corpus_pipeline/`.

## Missing / Still Open Launch Features

From `docs/GO_LIVE_CHECKLIST.md`, `docs/FEATURES.md`, and `TODO.md`, the
remaining non-code or not-yet-automated public-launch items are:

- Go release/runtime toolchain update to `1.25.11+`, followed by clean
  `govulncheck ./...` runs in the root module and `corpus_pipeline/`
- deployment-level WAF / edge throttling / monitoring in addition to app rate limits
- final human parser regression review against the frozen baseline reports
  before deploy sign-off
- long-term retention policy for retained deck/feedback parse context
- optional identity-provider decision for public auth

## Codex Security Status

The full Codex Security repository-wide scan was not completed because the
Codex Security workflow requires explicit authorization to use subagents for
exhaustive repository-wide coverage. I requested that authorization in the
thread. Once approved, run the Codex Security phases in order:

1. threat model
2. finding discovery
3. validation
4. attack-path analysis
5. final markdown/HTML report

This report covers deterministic tests, browser smoke, live API/security probes,
dependency audit checks, database invariants, and documentation gap analysis.
