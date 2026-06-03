# Go-Live Checklist

Owner: @chickendude

This list tracks blockers that must be resolved before FinEstDB is exposed to
real users outside a trusted dev/internal environment.

## Authentication and Sessions

Current status: real auth is implemented; remaining go-live work is hardening.

- `POST /api/auth/register` creates users with Argon2id password hashes.
- `POST /api/auth/login` verifies passwords and issues a random
  `session_token` cookie.
- Session tokens are hashed before storage in `user_sessions`, and logout
  revokes the server-side session.
- `getCurrentUser` resolves the authenticated user from the hashed session
  token, not from a client-provided user id.
- Legacy alpha users with an empty `password_hash` cannot be claimed through
  public login. They need an explicit reset/migration path if any remain.

Required before go-live:

- Keep the empty-password bootstrap path disabled for legacy alpha accounts.
- Keep regression tests proving forged/unknown session cookies cannot
  authenticate.
- Keep regression tests proving non-admin users cannot access admin APIs or
  admin routes.
- Keep strict Origin/Referer checks on cookie-authenticated state-changing
  routes.
- Decide whether production should use first-party password auth long term or
  add an identity-provider option.

## Abuse Controls

Current status: baseline app-level controls are implemented; deployment-level
limits and monitoring still need production configuration.

`POST /api/parse` can run parser work anonymously. This is useful for product
discovery, and the product decision is to keep that API path available as an
ephemeral, rate-limited endpoint. Public deployment still needs controls around
CPU, memory, storage, and queue pressure.

Required before go-live:

- Keep app-level rate limits for anonymous and authenticated `POST /api/parse`.
- Keep app-level rate limits for `POST /api/parse/feedback`,
  `POST /api/auth/login`, and `POST /api/auth/register`.
- Keep request-size enforcement for pasted text and verify it is applied before
  JSON decoding and expensive parser work.
- Configure HTTP server read/write/header timeouts.
- Add IP/account-level throttling or deployment-level WAF limits.
- Log rejected requests at a level useful for abuse monitoring without storing
  pasted text unnecessarily.
- Keep anonymous parse enabled only as an ephemeral, rate-limited endpoint.

## Runtime Reproducibility and Data Readiness

Current status: healthy locally after the 2026-05-18 audit; production startup
now fails fast when the dictionary DB is missing or undersized.

`make doctor` verifies the restored DB, FST tables, NLP venv, Ekilex shards,
UD cache, and parser dylib. A clean clone without gitignored artifacts fails
doctor clearly, and the documented symlink/bootstrap path restores the expected
FI dict + FST / ET dict + Ekilex + FST quality mode. When `APP_ENV=production`,
the server checks `finnestdb.db` before opening it for migrations or starting
the HTTP listener. Missing, empty, stub, or undersized dictionary DBs fail
startup. The default production minimums are 20,000,000 FI forms and 6,000,000
ET forms; override only with `FINNESTDB_PRODUCTION_MIN_FORMS_FI` /
`FINNESTDB_PRODUCTION_MIN_FORMS_ET` when the production artifact policy changes.
`FINNESTDB_ALLOW_DEGRADED_DB=1` disables the guard and should be treated as a
development or emergency-only override.

Required before go-live:

- Keep the production startup DB guard enabled for public deployments
  (`APP_ENV=production`, no `FINNESTDB_ALLOW_DEGRADED_DB=1`).
- Keep the fast-bootstrap/symlink runbook verified with `make doctor` after
  artifact restore.
- If startup fails on DB readiness, restore/symlink the production DB artifact,
  run `make doctor`, run `make db-invariants`, then restart the server.
- Verify public frequency baselines are present in the production artifact, or
  intentionally disable calibration features so they cannot silently run without
  comparison anchors.

## Release Verification

Current status: the core checks are repeatable; run them before every public
release candidate.

The 2026-06-03 verification run covered live API probes, parser baseline
comparisons, full-DB invariants, Playwright browser tests, race tests, and
dependency/security scans. The live API and DB invariant probes now live as
project targets instead of one-off audit commands.

Required before go-live:

- Run `make live-api-smoke` against a live release-candidate server. It
  exercises register/login, auth-required routes, admin-required routes,
  `/api/parse`, oversized JSON rejection, cross-origin state-changing requests,
  parse feedback/history deletion, rate limiting, and account deletion.
- Run the browser regression suite from `web/` with `npm test` for anonymous,
  registration, Inspect/deck, review, and admin-route coverage.
- Run `make compare-parsers` and `make compare-parsers-et` before release; fix
  or explicitly justify every parser regression against the frozen baseline.
- Run `make db-invariants` against the production candidate DB before launch:
  SQLite integrity, source breakdown, orphan checks, and known/ignored overlap
  checks.
- Run `go vet ./...`, `go test ./...`, `go test -race ./internal/api
  ./internal/auth ./internal/store`, `govulncheck ./...`, `npm audit`, and
  Rust dependency audit in CI/release tooling.

## Privacy and Retention Transparency

Current status: acceptable for alpha if clearly disclosed.

Inspect parses are ephemeral by default, including for signed-in users:
`POST /api/parse` returns results without creating a stored parse ID. Source
text is stored only when the user makes the parse durable by saving a deck or
submitting parser feedback. The app now exposes a History page where users can
list retained parse sessions and delete one or all retained sessions. Deleting
a parse session removes the retained source context and feedback tied to that
session; saved decks remain.

Required before go-live:

- Keep this behavior documented in `docs/FEATURES.md`.
- Keep the storage behavior surfaced in the parse UI before broad public
  release.
- Define the long-term retention policy for retained deck/feedback parse
  context before handling sensitive production user data.
