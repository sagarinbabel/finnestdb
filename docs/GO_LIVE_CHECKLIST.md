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
  cookie-authenticated state-changing routes.
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

## Privacy and Retention Transparency

Current status: acceptable for alpha if clearly disclosed.

Signed-in Inspect parses store `parse_sessions.source_text` with the user
account. Anonymous parses are not persisted unless a future anonymous feedback
flow is introduced. The app does not yet provide parse-history deletion or an
ephemeral signed-in parse option.

Required before go-live:

- Keep this behavior documented in `docs/FEATURES.md`.
- Surface the storage behavior in the UI before broad public release.
- Add a parse-history deletion or retention policy before handling sensitive
  production user data.
