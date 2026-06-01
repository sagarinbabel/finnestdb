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
- Legacy alpha users with an empty `password_hash` are bootstrapped on first
  login; this migration path should be retired before public launch.

Required before go-live:

- Remove or explicitly expire the empty-password bootstrap path for legacy
  alpha accounts.
- Keep regression tests proving forged/unknown session cookies cannot
  authenticate.
- Keep regression tests proving non-admin users cannot access admin APIs or
  admin routes.
- Add CSRF protection or strict Origin/Content-Type checks for
  cookie-authenticated state-changing routes. The 2026-05-18 audit probe
  confirmed an authenticated `POST /api/lemma-state` with
  `Origin: https://evil.example` is accepted when a valid cookie is supplied.
- Decide whether production should use first-party password auth long term or
  add an identity-provider option.

## Abuse Controls

Current status: blocker.

`POST /api/parse` can run parser work anonymously. This is useful for product
discovery, but public deployment needs controls around CPU, memory, storage, and
queue pressure.

Required before go-live:

- Add rate limits for anonymous and authenticated `POST /api/parse`.
- Add rate limits for `POST /api/parse/feedback`, `POST /api/auth/login`, and
  `POST /api/auth/register`.
- Keep request-size enforcement for pasted text and verify it is applied before
  JSON decoding and expensive parser work.
- Configure HTTP server read/write/header timeouts.
- Add IP/account-level throttling or deployment-level WAF limits.
- Log rejected requests at a level useful for abuse monitoring without storing
  pasted text unnecessarily.
- Decide whether anonymous parse remains enabled in production or becomes
  sign-in gated.

## Runtime Reproducibility and Data Readiness

Current status: healthy locally after the 2026-05-18 audit; production
guardrails still needed.

`make doctor` verifies the restored DB, FST tables, NLP venv, Ekilex shards,
UD cache, and parser dylib. A clean clone without gitignored artifacts fails
doctor clearly, and the documented symlink/bootstrap path restores the expected
FI dict + FST / ET dict + Ekilex + FST quality mode. The server still starts
against an empty SQLite DB, though, which is acceptable for local smoke tests
but unsafe for public production.

Required before go-live:

- Fail production startup when `finnestdb.db` is missing, empty, or lacks the
  expected FI/ET dictionary row counts, unless an explicit development-only
  degraded mode is set.
- Keep the fast-bootstrap/symlink runbook verified with `make doctor` after
  artifact restore.
- Verify public frequency baselines are present in the production artifact, or
  intentionally disable calibration features so they cannot silently run without
  comparison anchors.

## Release Verification

Current status: ad hoc checks exist; they need to become repeatable.

The 2026-05-18 audit ran targeted live API probes, parser baseline
comparisons, full-DB invariants, and a live Playwright smoke flow against a temp
SQLite DB. These should be preserved as release checks instead of one-off audit
commands.

Required before go-live:

- Add a repeatable live API smoke script that exercises register/login,
  auth-required routes, admin-required routes, `/api/parse`, oversized JSON
  rejection, and cross-origin state-changing requests.
- Add a repeatable browser smoke for anonymous landing, registration, Inspect
  parse, save-as-deck, and non-admin admin-route redirect.
- Run `make compare-parsers` and `make compare-parsers-et` before release; fix
  or explicitly justify every parser regression against the frozen baseline.
- Run documented SQL invariants against the production candidate DB before
  launch: SQLite integrity, source breakdown, orphan checks, and known/ignored
  overlap checks.

## Privacy and Retention Transparency

Current status: acceptable for alpha if clearly disclosed.

Inspect parses are ephemeral by default, including for signed-in users:
`POST /api/parse` returns results without creating a stored parse ID. Source
text is stored only when the user makes the parse durable by saving a deck or
submitting parser feedback. The app does not yet provide deletion controls for
stored deck/feedback parse context.

Required before go-live:

- Keep this behavior documented in `docs/FEATURES.md`.
- Surface the storage behavior in the UI before broad public release.
- Add deletion controls or a retention policy for saved deck/feedback parse
  context before handling sensitive production user data.
