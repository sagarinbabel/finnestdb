# Security Review (FIN-27) - 2026-07-02

Scope: repository-wide review of the go-live security surface, per the FIN-27
scope from the 2026-06-03 verification report: auth/session behavior, role
enforcement, CSRF posture, XSS exposure, request-size caps, HTTP timeouts,
rate limiting, data isolation, admin-route leakage, and correction-submission
abuse. Reviewed at the tip of the launch PR stack (#240–#247).

Reviewer: Claude (Fable 5), working session with the repo and a live local
server against the production-size DB. This satisfies the *content* of FIN-27;
it is not the external "Codex Security" tool run - if that specific workflow
is still wanted, it remains a separate authorization.

## Verified controls (spot-checked in code and/or exercised live)

| Area | Status | Evidence |
|---|---|---|
| Password storage | OK | Argon2id (64 MiB, t=3, p=2), PHC encoding, constant-time verify (`internal/auth`) |
| Sessions | OK | 32-byte random tokens, SHA-256-hashed at rest, 7-day sliding expiry, logout + reset revoke server-side |
| Cookie flags | OK | HttpOnly, SameSite=Lax, Secure under `APP_ENV=production` |
| Login/register CSRF | OK (fixed #240) | `allowStateChangingRequest` now guards both; regression tests |
| Mutation CSRF | OK | Origin/Referer check on every cookie-authenticated state-changing route; live smoke covers foreign-origin rejection |
| Role enforcement | OK | `requireAdmin` wraps admin APIs; self-demotion guard; non-admin 403 covered by tests + smoke |
| Rate limiting | OK (fixed #240) | Per-IP + per-account fixed windows on parse/feedback/login/register; forwarded headers trusted only from private/loopback peers or explicit flag, rightmost IP wins; limiter maps pruned |
| Request caps | OK | `MaxBytesReader` before JSON decode (4 MiB); 16 MiB upload cap; EPUB reads length-bounded (`io.LimitReader`) |
| HTTP timeouts | OK | ReadHeader 5s / Read 15s / Write 30s / Idle 60s; worst measured legitimate request 4s (full novel, cold) |
| XSS | OK | All 30+ dynamic `innerHTML` sinks audited: user-controlled fields (feedback notes, deck titles, source previews, emails, glosses, example sentences) pass through `escapeHtml`/`escapeAttr` before interpolation; `highlightFormsInSentence` escapes before inserting its own markup. Admin feedback queue (attacker-content → admin browser) specifically verified |
| SQL injection | OK | All queries parameterized; the only string-built SQL is migration-time hardcoded identifiers and test-only PRAGMAs |
| Data isolation | OK | Deck/review/known-words/history/comprehension paths scope by authenticated user id; foreign-deck 404 and cross-user feedback 403 covered by tests; account deletion cascade preserves other users' data (fixed #240) |
| Correction abuse | OK | Feedback submission is authenticated + rate-limited; only admin acceptance writes lexical rows; Phase-4 gold guard (#247) blocks corrections contradicting the frozen eval; Phase-3 promotion requires 3 distinct users and human review before gold changes |
| Secrets | OK | Pattern scan over Go/TS/JS/YAML/JSON found no embedded credentials |
| Dependency posture | OK | CI pinned to Go 1.25.11 (govulncheck clean 2026-06-03); npm audit and cargo audit clean at last verification |

## Findings

### S2 - Dev artifacts would be publicly served from the web root (fixed)

`cmd/server` serves everything under `web/` with a plain file server. The
deployment runbook's original `rsync -a web/` would have shipped
`node_modules/` and Playwright `test-results/` (traces, screenshots) to
production, where they'd be world-readable. Fixed in this change: the runbook
rsync now excludes dev artifacts. Residual risk on a careless manual deploy is
noted in the runbook comment. Post-alpha hardening option: an explicit
allowlist file server.

### S3 - No Content-Security-Policy (accepted for alpha)

The app relies on escaping discipline (audited above) plus the proxy-level
`X-Content-Type-Options`/`X-Frame-Options`/HSTS headers from the shipped
Caddyfile. There is no CSP. Because the frontend is a single self-hosted
script with no third-party embeds, a strict CSP would be cheap insurance
against any future escaping slip. Recommended post-alpha:
`default-src 'self'` + `style-src 'self' 'unsafe-inline'`.

### S4 - Registration reveals account existence (accepted for alpha)

`POST /api/auth/register` returns 409 "Email already registered", enabling
enumeration. It is rate-limited per IP and per email, which bounds bulk
enumeration. Acceptable for alpha; revisit if abuse appears in the rejected-
request logs.

### S5 - Admin bootstrap depends on pre-registration discipline (mitigated)

`FINNESTDB_ADMIN_EMAILS` grants admin to whoever first registers a listed
address, and emails are unverified. Mitigation shipped in #241: operator
pre-registration via `cmd/resetpassword -create` is a documented, gating step
in both the checklist and the runbook. The durable fix is email verification
or an explicit admin-invite flow - post-alpha.

### S6 - Operator password reset prints the password to stdout (accepted)

`cmd/resetpassword` prints the generated password; on a shared host that can
land in shell history/scrollback. It is an operator tool run over SSH by the
deploy owner; guidance to deliver over a trusted channel is in the checklist.

## Residual risks accepted for alpha

- No email verification / self-service password reset (operator procedure
  documented instead).
- Anonymous `/api/parse` stays enabled by product decision - ephemeral,
  rate-limited, size-capped; CPU exposure bounded by the measured ~4s
  worst-case parse and the per-IP window.
- Single-host SQLite: availability (not integrity) depends on one machine;
  backups + restore drill are the compensating control.
