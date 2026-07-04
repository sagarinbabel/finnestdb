# Deployment Runbook

_Current as of 2026-07-02 — see [CHANGELOG.md](CHANGELOG.md) for revisions._

How to run FinEstDB in production. This is the operational companion to
[`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md): the checklist says what must
be true before launch; this document says how to stand it up and keep it up.

## Topology

One Linux host runs everything for the public alpha:

```
internet ── Caddy (TLS, edge body cap, security headers)
                │ 127.0.0.1:8080
            FinEstDB Go server (cmd/server, APP_ENV=production)
                │
            finnestdb.db (SQLite, WAL mode — dictionary + ALL user data)
```

- The app binds loopback only (`FINNESTDB_LISTEN_ADDR=127.0.0.1:8080`), so
  the proxy is the sole public entry point.
- The SQLite file contains both the rebuildable dictionary and the
  **unrebuildable user data** (accounts, decks, review state, feedback).
  Losing it loses every user. Backups are not optional.

## Host requirements

- Linux x86_64 with systemd; 2+ cores, 4+ GB RAM.
- Disk: 5+ GB for `finnestdb.db`, plus backup space — with default rotation
  (7 daily + 4 weekly) budget roughly 12× the compressed backup size. Check
  with `du -sh finnestdb.db` and one trial run of `scripts/backup-db.sh`.
- `sqlite3` CLI (used by the backup script's online `.backup`).
- Build toolchain (Go ≥ 1.25.11 and Rust stable) on the host, or build the
  artifacts elsewhere on a matching platform and copy them.
- Go **1.25.11 or newer is a security requirement**, not a preference — older
  toolchains have reachable stdlib vulnerabilities (see the 2026-06-03
  verification report).

## Build and install layout

```bash
make parser                          # Rust tokenizer (parser/target/release/libparser.so)
go build -o bin/server ./cmd/server
go build -o bin/purgeparsecontext ./cmd/purgeparsecontext
go build -o bin/resetpassword ./cmd/resetpassword
cd web && npm ci && npm run build    # app.js is checked in; this verifies it
```

Install under `/opt/finnestdb` (the systemd units assume this layout):

```
/opt/finnestdb/
  bin/server  bin/purgeparsecontext  bin/resetpassword
  parser/target/release/libparser.so     # cgo runtime dependency of bin/server
  web/                                   # static assets, served by the app
  scripts/backup-db.sh
  finnestdb.db                           # restored production artifact
  backups/
```

Create a dedicated non-login user: `useradd --system --home /opt/finnestdb finnestdb`.

## Database artifact

Restore the production dictionary DB before first start, then verify:

```bash
make doctor          # tooling + artifact presence
make db-invariants   # SQLite integrity, orphans, overlap, source breakdown
```

With `APP_ENV=production` the server refuses to start on a missing, stub, or
undersized DB (defaults: 20M FI / 6M ET forms; see
[`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md) "Runtime Reproducibility").
`FINNESTDB_ALLOW_DEGRADED_DB=1` disables that guard — emergency use only,
never in a normal deployment.

Also load the correction-loop safety data (and re-run whenever the committed
gold sets change — add it to the update procedure):

```bash
make import-gold-surfaces   # gold_surfaces table backs the Phase-4 accept guard
```

## Environment variables

| Variable | Production value | Why |
|---|---|---|
| `APP_ENV` | `production` | Secure cookies + DB readiness guard |
| `FINNESTDB_LISTEN_ADDR` | `127.0.0.1:8080` | Loopback bind behind the proxy |
| `FINNESTDB_TRUST_FORWARD_HEADERS` | `1` | See "Rate limiting behind a proxy" |
| `FINNESTDB_ADMIN_EMAILS` | comma-separated | Admin bootstrap — **pre-register every address** |
| `FINNESTDB_ANON_MAX_CHARS` | `300000` (default) | Text-size cap for unauthenticated `/api/parse`; enforced before parser work. Signed-in cap stays 1,500,000. Local load test (2026-07-04) found no reason to lower this — see [`launch-readiness/2026-07-04-load-test.md`](launch-readiness/2026-07-04-load-test.md). |
| `FINNESTDB_PARSER_MAX_CONCURRENCY` | `max(2, NumCPU-1)` (default) | Caps concurrent calls into the parser (`/api/parse` and deck-save). Leave unset unless the production host's core count and co-located services justify a different number — see the load-test report. |
| `FINNESTDB_PARSER_QUEUE_TIMEOUT_MS` | `2000` (default) | How long a parse request waits for a free parser slot before returning 503 + `Retry-After`. |
| `FINNESTDB_PRODUCTION_MIN_FORMS_FI/ET` | unset | Only when the artifact policy changes |
| `FINNESTDB_ALLOW_DEGRADED_DB` | unset | Emergency-only guard override |
| `FINNESTDB_FSRS_ENABLED` | unset (**ON**) | Review scheduler selection (**opt-OUT** since 2026-07-04). Unset/empty runs the **FSRS scheduler** — the shipped default — for `POST /api/study/answer`. Set `0`/`false`/`no`/`off` to fall back to the deterministic step scheduler; that is the **rollback lever**. See "FSRS scheduler rollout". |

## FSRS scheduler rollout

`FINNESTDB_FSRS_ENABLED` selects the review scheduler at request time. **FSRS is
the default** (opt-OUT flag): with the variable unset, ratings route through
`go-fsrs/v3` with default parameters. This was enabled after the 2026-07-04
staging validation ([`launch-readiness/2026-07-04-fsrs-validation.md`](launch-readiness/2026-07-04-fsrs-validation.md))
came back green on seeded histories, migration-at-scale, rollback, and a
real-DB smoke.

State from both schedulers coexists in `card_state.fsrs_json` via a version
discriminator (legacy `{"step","streak"}` vs. `{"v":2,"fsrs":{…}}`), and
`next_due` / `last_answer_at` / `introduced_at` keep working regardless, so the
scheduler choice is reversible per card. Migration is lazy on first rating — no
bulk `card_state` rewrite — deriving a conservative FSRS seed from the card's
existing interval (see [srs-deck-spec.md](srs-deck-spec.md) "Implemented FSRS
state model").

### Rollback

Rollback is a single flag flip, no data migration:

1. Set `FINNESTDB_FSRS_ENABLED=0` (or `false`/`no`/`off`) and restart the
   service.
2. Ratings now route through the deterministic step scheduler. FSRS-touched
   cards keep answering without error: the step path approximates a step from
   the card's current interval, so a card does **not** snap back to step 0 and
   lose earned progress. This byte-identical-to-the-step-scheduler guarantee is
   pinned by `TestRecordReviewAnswerFlagOffByteIdenticalToStepScheduler`.
3. Flipping the flag back to unset resumes FSRS from the current interval — the
   round trip is validated end to end by `TestFSRSValidationRollbackDrill`.

### Re-validating before a scheduler change

The staging gate is a suite, not a manual checklist. To re-run it (e.g. after a
`go-fsrs` bump), on a host with the shared `finnestdb.db` present:

```
go test ./internal/store/ -run TestFSRSValidation -count=1 -v
```

It exercises seeded histories across new/learning/mature/legacy/NULL shapes, a
1k-card lazy-migration scale check, the rollback round trip, and a read-only
real-DB smoke — all on temp DBs; the shared DB is never written.

## Rate limiting behind a proxy

The app trusts `X-Forwarded-For` only when the TCP peer is loopback/private
**or** `FINNESTDB_TRUST_FORWARD_HEADERS=1`. Two failure modes to avoid:

- **Proxy on a public IP, flag unset** → forwarded headers are ignored, every
  client keys to the proxy's IP, and real users mass-429 the moment anyone
  hits a limit.
- **App exposed directly with the flag set** → clients can spoof
  `X-Forwarded-For` and rotate rate-limit keys.

The shipped configuration (loopback bind + same-host Caddy + flag set) avoids
both. If the proxy moves to another host, keep the flag set and firewall the
app port to the proxy host only.

## TLS and reverse proxy

Use [`deploy/Caddyfile`](../deploy/Caddyfile): Caddy provisions TLS
automatically, caps request bodies at the edge, and adds HSTS,
`X-Content-Type-Options`, `X-Frame-Options`, and a referrer policy. Point the
domain's A/AAAA records at the host and set the real domain in the file.

## Asset versioning (cache-busting)

The app versions its own JS/CSS so a deploy never leaves a browser running stale
`app.js` (this bit an operator during manual testing after a `git pull`). It is
handled entirely in-process — no build step, no CDN config:

- On startup the server hashes `web/app.js` and `web/styles.css` (short sha256)
  and logs the stamps: `asset app.js versioned as ?v=<hash>`.
- `index.html` is served with `Cache-Control: no-cache` (always revalidated) and
  its `app.js` / `styles.css` references are re-stamped with the current hashes
  on every request. So a browser always fetches the current `index.html`, which
  points at the current asset URLs.
- `app.js` / `styles.css` are served with `Cache-Control: no-cache,
  must-revalidate` and a content-hash `ETag`, so even an intermediary that
  strips the `?v=` query revalidates instead of serving stale bytes. Matching
  `If-None-Match` returns `304`.
- All other web files keep `Cache-Control: no-store`.

Implication for deploys: **the hash refreshes only when the server process
restarts** (it is computed at startup). The `systemctl restart finnestdb` step
in "Deploying an update" is what publishes new asset hashes. If you sync new
`web/` files but don't restart, `index.html` keeps stamping the old hashes.

At the reverse proxy (`deploy/Caddyfile`), do **not** add caching directives for
`index.html`; the app's own headers are authoritative. Verify after a deploy:

```bash
curl -sI https://<domain>/ | grep -i cache-control          # no-cache
curl -s  https://<domain>/ | grep -o 'app.js?v=[a-f0-9]*'    # current hash
curl -sI "https://<domain>/app.js?v=<hash>" | grep -iE 'cache-control|etag'
```

The stamped hash must change across a rebuild+restart when the asset content
changes.

## Services and timers

Install from [`deploy/`](../deploy/):

```bash
cp deploy/finnestdb.service deploy/finnestdb-backup.* deploy/finnestdb-purge.* /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now finnestdb finnestdb-backup.timer finnestdb-purge.timer
```

- `finnestdb.service` — the app, `Restart=always`, loopback bind.
- `finnestdb-backup.timer` — nightly 03:15 UTC online backup with rotation.
- `finnestdb-purge.timer` — daily 04:00 UTC retention purge
  (`parse_sessions.source_text` older than 30 days), per the privacy policy in
  [`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md).

## Backups and restore

`scripts/backup-db.sh` uses SQLite's online `.backup` (safe against a live
WAL database — never `cp` the raw file while the server runs), integrity-checks
the copy before rotating anything out, gzips, and keeps 7 daily + 4 weekly.

**Restore drill** (run it once before launch, not during your first incident):

```bash
systemctl stop finnestdb
gunzip -c backups/daily/finnestdb-<stamp>.db.gz > /opt/finnestdb/finnestdb.db
rm -f /opt/finnestdb/finnestdb.db-wal /opt/finnestdb/finnestdb.db-shm
make db-invariants
systemctl start finnestdb
curl -fsS https://<domain>/api/health
```

Keep at least one recent backup off the host (object storage or another
machine) — a dead disk must not take the only copies with it.

## Latency expectations

Measured 2026-07-02 on a dev laptop against the production-size DB (26.8M FI
forms), with real full-novel texts:

| Request | Input | Time |
|---|---|---|
| `POST /api/decks` | 550k chars / 70,234 tokens | 1.6 s |
| `POST /api/decks` | 809k chars (largest local book) | 2.0 s |
| `POST /api/parse` | same inputs | 1.3 s warm – 4.0 s cold |

Rule of thumb: ~0.2–0.6 s per 10k tokens. At the shipped input caps (4 MiB
JSON body, 1.5M-char textarea) the worst case stays far below the server's
30 s `WriteTimeout`. If production p95 for full-book requests approaches
~10 s, revisit the deferred background-job design in `TODO.md` before raising
any input caps.

### Concurrency load test

`cmd/loadtest` models concurrent anonymous-parse / signed-in-parse /
review-deck-read traffic and reports per-endpoint latency percentiles plus
429/503 counts. Local (laptop) results and the recommended
`FINNESTDB_PARSER_MAX_CONCURRENCY` / `FINNESTDB_PARSER_QUEUE_TIMEOUT_MS`
values are in
[`launch-readiness/2026-07-04-load-test.md`](launch-readiness/2026-07-04-load-test.md).
**That report must be re-run against the production host** before the
1,000-concurrent-user go-live gate can close — laptop numbers only prove the
shedding mechanism works, not the production host's actual capacity. Example:

```bash
go build -o bin/loadtest ./cmd/loadtest
./bin/loadtest -url https://<domain> -concurrency 200 -duration 30s -out /tmp/loadtest-200.json
```

## Monitoring and alerting (alpha baseline)

- **Uptime**: point an external monitor (UptimeRobot, Gatus, healthchecks.io)
  at `GET /api/health`. It returns 200 `{"status":"ok"}` only when the process
  is up and the DB answers a query; alert on non-200 for >2 minutes.
- **Error logs**: app logs go to journald. Minimum viable alert:
  a cron'd `journalctl -u finnestdb --since -10min -p err` that mails/pings on
  output. Rejected-request rate-limit logs are in the same stream for abuse
  review.
- **Timer health**: `systemctl list-timers finnestdb-*` in the same cron;
  alert if a timer's last run failed (`systemctl is-failed finnestdb-backup`).
- **Disk**: alert at 80% — the DB, WAL, and backups all grow.
- **Parser saturation**: 503 responses from `/api/parse` (or deck-save) mean
  the parser concurrency semaphore is shedding load; both 429 (rate limit)
  and 503 (parser saturation) responses carry `Retry-After` and are logged.
  Grep app logs for these and alert on a sustained rate — an occasional 503
  under a burst is the semaphore working as designed, but a sustained rate
  means `FINNESTDB_PARSER_MAX_CONCURRENCY` may need raising (if the host has
  spare CPU) or the host needs more capacity.

Parser-health dashboards (cache hit rates, unknown-lemma counters) are
tracked separately in TODO.md "Observability" and are not launch-gating.

## Deploying an update

```bash
cd /opt/finnestdb/src && git pull
make parser && go build -o /opt/finnestdb/bin/server ./cmd/server
# The app serves EVERYTHING under its web root. Exclude dev artifacts —
# node_modules is bloat, and Playwright test-results/ contains traces and
# screenshots that must never be publicly served.
rsync -a --delete \
  --exclude node_modules --exclude test-results --exclude tests \
  --exclude playwright.config.ts --exclude '*.ts' --exclude tsconfig.json \
  web/ /opt/finnestdb/web/
/opt/finnestdb/scripts/backup-db.sh /opt/finnestdb/finnestdb.db /opt/finnestdb/backups  # pre-deploy snapshot
systemctl restart finnestdb
curl -fsS https://<domain>/api/health
```

Schema migrations run automatically at server startup. Rollback = restore the
previous binary and, only if the new code migrated the schema, the pre-deploy
snapshot.

## Seeding the cold-start starter decks

New accounts land on an empty dashboard whose CTA links to the official-decks
tab. Seed the "Top 1000 words" official decks once per language after the
admin account exists (requires the frequency baselines from
`make fetch-frequency-baselines` on the machine running the seed):

```bash
go run ./cmd/seedcolddeck -db finnestdb.db -lang FI -owner-email <admin>
go run ./cmd/seedcolddeck -db finnestdb.db -lang ET -owner-email <admin>
```

Forms are resolved to lemmas through the dictionary and ranked by summed
token mass across inflections; proper names are filtered. Re-running creates
a duplicate deck — delete the old one from the admin UI first if reseeding.

## Pre-launch gate

Before announcing the instance, complete the "Release Verification" section of
[`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md) against this host — including
`make live-api-smoke` pointed at the live server — and pre-register every
`FINNESTDB_ADMIN_EMAILS` address with `bin/resetpassword -create`. Then seed
the starter decks (above).
