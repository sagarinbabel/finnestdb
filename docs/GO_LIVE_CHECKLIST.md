# Go-Live Checklist

Owner: @chickendude

This list tracks blockers that must be resolved before FinnEst is exposed to
real users outside a trusted dev/internal environment.

How to actually stand up the production host (TLS, proxy, systemd, backups,
purge cron, monitoring) lives in [`DEPLOYMENT.md`](DEPLOYMENT.md).

## Alpha Go/No-Go Rubric

Public alpha can launch when core journeys work end-to-end and every known rough
edge is classified as non-dangerous.

The first experience should be excellent about 95% of the time. Gate launch
with release-candidate testing. After launch, validate the same bar with
privacy-preserving telemetry when available. Telemetry is not a public-alpha
blocker if server logs plus manual feedback/admin review are available as the
week-one fallback. This is stricter than "the app technically works": the first
anonymous or signed-in journey should usually feel credible, fast-enough, and
trustworthy to a real learner.

Core journeys for the launch decision:

- anonymous FI and ET paste -> parse -> word list -> explore;
- signed-in FI and ET Inspect -> save/add deck -> review;
- FI and ET known-word import;
- FI and ET parser feedback;
- admin feedback triage/quarantine;
- account creation, sign-in, sign-out, account deletion, and retention purge;
- overload behavior for anonymous/signed-in parse traffic; and
- production artifact readiness for both languages.

First-experience quality check:

- Build a journey-first FI/ET test pack that covers anonymous demo, embedded
  text, own-text Inspect, save deck, first review, known-word import, and parser
  feedback. This must be a checked-in, repeatable release-candidate artifact,
  not an ad hoc manual checklist.
- Include explicit FI and ET cases for curated embedded texts, realistic pasted
  texts, known-word import examples, ambiguity/homograph handling,
  parser-feedback flows, deck save, and first review.
- Keep one canonical manifest at `testdata/first-experience-rc/manifest.json`.
  Parser checks, `web/tests` Playwright specs, and the manual walkthrough should
  consume that same manifest and fixtures so the launch gate cannot drift across
  three separate definitions.
- Build the manifest and a small skeleton runner first, before waiting for all
  alpha flows to exist. It may fail initially; public alpha still requires the
  final pack to pass or have all findings classified under this rubric.
- Provide one top-level automated command, `make first-experience-rc`, that runs
  the parser fixture checks and Playwright RC specs, then points at the manual
  walkthrough instructions in this section. The walkthrough instructions live
  here (Q60): do not create a separate walkthrough/MANUAL doc, and keep
  `testdata/first-experience-rc/manifest.json` data/fixtures-only, not another
  planning document.
- Run the pack in two parts: automated parser/browser checks for deterministic
  behavior, plus a short manual product walkthrough for judgment calls such as
  trustworthiness, clarity, and first-screen credibility.
- Grade every finding as `blocker`, `serious`, or `minor`. A `blocker` breaks a
  core journey or violates privacy/security/data/review/retention/parity safety.
  A `serious` finding is a trust-breaking first-experience issue even if the app
  can technically recover. Blockers and serious findings are no-go until fixed or
  explicitly reclassified with evidence. A `minor` finding can ship only if it
  satisfies the non-dangerous rough-edge rubric and is tracked in the launch
  issue ledger.
- Count a run as clean only when the flow completes, the UI state is truthful,
  the first screenful has no obvious high-severity parser/card issue, and
  latency/error behavior does not make the product feel unreliable.
- Public alpha should not launch if more than roughly 5% of first-experience
  runs are broken, misleading, embarrassing, or likely to make a serious learner
  lose trust.
- After launch, validate the same standard with week-one telemetry when
  available: journey
  completion/drop-off, parse/deck/review errors, latency, retry/429/503 rates,
  feedback/flag rate, quarantine-triggering reports, and language split. Do not
  retain pasted source text merely to compute these metrics.
- Telemetry is aggregate by default. Per-user event trails are allowed only for
  signed-in users, only for product events needed to debug onboarding failures,
  and never for pasted source text.
- If telemetry is not ready at launch, record it as a post-launch roadmap
  checkpoint and use server logs plus manual feedback/admin review until the
  minimal telemetry lands.

A rough edge is **non-dangerous** only if all conditions hold:

- It creates no privacy, security, account, retention, or abuse risk.
- It does not lose or corrupt learner data, review state, known-word state,
  source-retention state, or feedback/quarantine history.
- It does not mislead the learner about what is known, saved, retained,
  deleted, reviewed, quarantined, or parser-confident.
- It does not break an agreed core journey for either FI or ET.
- It has a clear workaround, retry path, or honest UI explanation.
- It is documented in the launch issue list with owner, severity, affected
  journey/language, evidence, workaround, and revisit condition.

No-go blockers include:

- auth/session/admin isolation bugs;
- account deletion or retention behavior that does not match product copy;
- source text stored when the UI says it is ephemeral;
- parser-feedback or quarantine failure that leaves confirmed-bad study content
  in circulation;
- deck/review/FSRS state loss, duplicate state, or incorrect durable due state;
- known-word import that silently records the wrong durable knowledge claim;
- FI/ET asymmetry that makes one language fail the equal-status journey;
- anonymous parse load that can starve signed-in review/deck usage;
- overload behavior that times out or corrupts state instead of returning clear
  retry behavior; and
- misleading parser-confidence or meaning-check UI.

Acceptable rough edges include cosmetic polish gaps, clear-but-plain wording,
minor extra clicks, limited non-core admin conveniences, incomplete post-alpha
roadmap features, and isolated parser imperfections that are reportable and do
not undermine feedback/quarantine safety.

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
  routes, including `POST /api/auth/login` and `POST /api/auth/register`.
- Pre-register every `FINNESTDB_ADMIN_EMAILS` address before opening public
  registration (see "Account recovery and admin provisioning" below).
- Decide whether production should use first-party password auth long term or
  add an identity-provider option.
- Do not block first learner value on email verification. New accounts should
  be able to parse, save a deck, and start review immediately; verification can
  gate high-volume parsing, repeated feedback, exports if enabled, account
  recovery, and trust-weighted correction signals.

### Account recovery and admin provisioning (alpha)

There is no self-service password reset or email verification in the alpha.
Two operational consequences:

1. **Password resets are operator-run.** When a user asks for a reset (over a
   channel where you can plausibly confirm they own the account), run:

   ```bash
   go run ./cmd/resetpassword -db finnestdb.db -email user@example.com
   ```

   The tool generates and prints a fresh password, updates the Argon2id hash,
   and revokes the account's active sessions. Deliver the password over a
   trusted channel and ask the user to change it after signing in. Use
   `-password '...'` to set a specific value and `-keep-sessions` to skip the
   session revocation.

2. **Admin emails must be claimed before launch.** `FINNESTDB_ADMIN_EMAILS`
   grants admin to whoever registers a listed email *first*, and emails are
   not verified — an attacker who registers an unclaimed admin address gets
   admin. Before opening registration to the public, pre-register every
   listed address:

   ```bash
   FINNESTDB_ADMIN_EMAILS="admin@example.com" \
     go run ./cmd/resetpassword -db finnestdb.db -email admin@example.com -create
   ```

   `-create` inserts the account (admin when the email is listed in
   `FINNESTDB_ADMIN_EMAILS` at creation time; add `-admin` to grant it
   explicitly) and prints the generated password.

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
- Enforce a lower configurable text-size cap for anonymous parsing than for
  signed-in parsing. The current signed-in cap is 1,500,000 characters; do not
  expose that full ceiling to unsigned demo traffic.
- Configure HTTP server read/write/header timeouts.
- Add IP/account-level throttling or deployment-level WAF limits.
- Log rejected requests at a level useful for abuse monitoring without storing
  pasted text unnecessarily.
- Keep anonymous parse enabled only for the stateless parser demo:
  paste/parse/list/explore, with no durable learner state.

## Capacity and Graceful Degradation

Current status: the product target is now explicit; production load testing and
degradation behavior still need to be proven.

The initial hosted alpha should be planned for roughly 1,000 concurrent users.
The app is unlikely to exceed that without paid acquisition, but it should
degrade predictably if usage spikes or parser work saturates the server.

Required before go-live:

- Add a release-candidate load test that models anonymous paste/parse/list
  exploration plus signed-in parse/deck/review traffic at 1,000 concurrent
  users.
- Use that load test to tune the anonymous text-size cap and prove oversized
  anonymous requests are rejected before expensive parser work.
- Configure parser concurrency/backpressure so parser CPU or memory saturation
  cannot starve the whole app.
- Under pressure, throttle anonymous and oversized parses first; preserve core
  signed-in deck/review actions as long as possible.
- Return clear 429/503 retry behavior instead of timeouts or partial HTML/API
  failures.
- Monitor parser latency, parser error rate, request rejection counts, DB write
  latency, memory, CPU, and feedback volume.
- Document the horizontal scale path: repeatable runtime artifacts per server,
  no server-local user session assumptions, explicit parser worker limits, and
  deployment notes for adding more app/parser capacity when funding allows.

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
- Verify Finnish and Estonian both meet the equal-status alpha journey:
  anonymous parse/list/explore, signed-in Inspect, embedded catalog, deck save,
  review, known-word import, parser feedback, admin triage/quarantine, eval
  observability, and production artifact readiness. Any asymmetry must be fixed
  or explicitly classified as language-specific/post-alpha before launch.

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
  ./internal/auth ./internal/store`, `govulncheck ./...` with Go `1.25.11`
  or newer, `npm audit`, and Rust dependency audit in CI/release tooling.

## Privacy and Retention Transparency

Current status: acceptable for alpha if clearly disclosed.

Inspect parses are ephemeral by default, including for signed-in users:
`POST /api/parse` returns results without creating a stored parse ID. Source
text is stored only when the user makes the parse durable by saving a deck or
submitting parser feedback. The app now exposes a History page where users can
list retained parse sessions and delete one or all retained sessions. Deleting
a parse session removes the retained source context and feedback tied to that
session; saved decks remain.

Retention policy: raw source text retained through saved decks or feedback is
kept for 30 days, then purged with `make purge-parse-context`. The purge clears
`parse_sessions.source_text` while preserving decks, cards, feedback rows, and
admin review state. Use `make purge-parse-context PURGE_PARSE_CONTEXT_FLAGS=-dry-run`
before applying the purge to a production candidate DB.

Required before go-live:

- Keep this behavior documented in `docs/FEATURES.md`.
- Keep the storage behavior surfaced in the parse UI before broad public
  release.
- Run the dry-run and purge against the production candidate DB before launch,
  then schedule the same command in deployment operations.
