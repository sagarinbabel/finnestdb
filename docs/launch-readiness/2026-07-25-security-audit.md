# Security audit (FIN-27) - 2026-07-25

Scope: repository review plus non-invasive checks against `https://finne.st`.
The review covered authentication and authorization, request limits, parser
backpressure, static-file exposure, production headers, dependency posture,
and tracked-secret patterns. This is the canonical tracked report for the
2026-07-25 run; no generated HTML copy is maintained.

## Validated controls

| Area | Result | Evidence |
|---|---|---|
| Passwords and sessions | Pass | Argon2id password hashes; random 32-byte tokens, hashed at rest; production Secure/HttpOnly cookies; server-side revocation. |
| CSRF and authorization | Pass | Origin/Referer guard protects auth and state-changing requests; admin routes require authenticated admin users. |
| Parse abuse controls | Pass | Request caps precede parsing, per-IP/account limits apply, and the parser limiter sheds anonymous work first. |
| Data isolation | Pass | User-scoped deck, review, vocabulary, history, and feedback queries have regression coverage. |
| Dependencies and secrets | Pass | `npm audit` and `cargo audit` reported no vulnerabilities; tracked-secret scan reported no credentials. |

## Findings

### S2 - deployed Caddy configuration has drifted

Live `/` and `/api/health` responses omit the HSTS, `X-Content-Type-Options`,
`X-Frame-Options`, Referrer-Policy, and CSP headers the repository intends to
serve. The tracked Caddy configuration previously targeted a placeholder domain
and therefore was not an installation-ready source of truth.

This change makes `deploy/Caddyfile` target `finne.st`, adds the headers and
JSON access logging, and documents the host apply and validation steps. The
production host must still install, validate, and reload that file before this
finding can close.

### S2 - the static handler exposes frontend development artifacts

The live service returned 200 for `/app.ts`, `/playwright.config.ts`,
`/tests/landing-prototype.spec.ts`, and `/package-lock.json`. A catch-all
`http.FileServer` exposed every file below `web/`.

The server now serves only `index.html`, `app.js`, and `styles.css`; all other
paths return 404. `TestStaticHandlerServesOnlyShippedAssets` prevents a future
test, source, or package artifact from becoming public accidentally.

### S3 - production builds need Go 1.25.12 or newer

The former 1.25.11 CI/deployment baseline is behind the fixed Go release. The
audit found reachable standard-library findings under the local 1.25.4 toolchain
and a clean `govulncheck` result with Go 1.25.12. CI and deployment guidance
now require 1.25.12. The operator must record `go version` from the rebuilt
production binary during deployment.

## Deployment verification required

After applying the security PR to the host, verify:

```bash
curl -sSI https://finne.st/
curl -sS -D - https://finne.st/api/health -o /dev/null
for path in /app.ts /playwright.config.ts /tests/landing-prototype.spec.ts /package-lock.json; do
  curl -s -o /dev/null -w "%{http_code} %{url_effective}\\n" "https://finne.st${path}"
done
```

Both first responses must contain the configured security headers. The four
development paths must return 404, while `/`, `/app.js`, and `/styles.css`
remain available. Verify Anki import with desktop Anki open because the CSP
intentionally permits the opt-in local AnkiConnect endpoint.

## Remaining accepted alpha risks

- Registration reveals whether an email already exists; rate limits bound bulk
  enumeration.
- Admin-email bootstrap relies on pre-registering configured admin addresses.
- Password reset is an operator workflow that prints the generated password.
- FIN-25 remains open until a real edge WAF/rate-limit provider, origin
  firewall, monitoring, and alerting are configured on the production host.
