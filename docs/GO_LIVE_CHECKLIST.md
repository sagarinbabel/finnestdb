# Go-Live Checklist

Owner: @chickendude

This list tracks blockers that must be resolved before FinEstDB is exposed to
real users outside a trusted dev/internal environment.

## Authentication and Sessions

Current status: blocker.

- `POST /api/auth/login` accepts any email/password and creates the user.
- `getCurrentUser` trusts a plain `user_id` cookie directly.
- Anyone who can set `user_id=1` can impersonate that user. If that row is an
  admin, they become an admin.
- `HttpOnly` and `SameSite=Strict` do not protect against a forged cookie value.

Required before go-live:

- Replace the mock login flow with real credential or identity-provider auth.
- Replace the raw `user_id` cookie with a signed, server-verifiable session.
- Ensure session cookies are `HttpOnly`, `Secure` in production, scoped to the
  app path/domain, and have a deliberate expiration policy.
- Add logout/session invalidation semantics that actually revoke the server-side
  session.
- Add regression tests proving a forged `user_id` cookie cannot authenticate.
- Add regression tests proving non-admin users cannot access admin APIs or admin
  routes.

## Abuse Controls

Current status: blocker.

`POST /api/parse` can run parser work anonymously. This is useful for product
discovery, but public deployment needs controls around CPU, memory, storage, and
queue pressure.

Required before go-live:

- Add rate limits for anonymous and authenticated `POST /api/parse`.
- Add rate limits for `POST /api/parse/feedback` and `POST /api/auth/login`.
- Keep request-size enforcement for pasted text and verify it is applied before
  expensive parser work.
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

