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

## Environment variables

| Variable | Production value | Why |
|---|---|---|
| `APP_ENV` | `production` | Secure cookies + DB readiness guard |
| `FINNESTDB_LISTEN_ADDR` | `127.0.0.1:8080` | Loopback bind behind the proxy |
| `FINNESTDB_TRUST_FORWARD_HEADERS` | `1` | See "Rate limiting behind a proxy" |
| `FINNESTDB_ADMIN_EMAILS` | comma-separated | Admin bootstrap — **pre-register every address** |
| `FINNESTDB_PRODUCTION_MIN_FORMS_FI/ET` | unset | Only when the artifact policy changes |
| `FINNESTDB_ALLOW_DEGRADED_DB` | unset | Emergency-only guard override |

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

Parser-health dashboards (cache hit rates, unknown-lemma counters) are
tracked separately in TODO.md "Observability" and are not launch-gating.

## Deploying an update

```bash
cd /opt/finnestdb/src && git pull
make parser && go build -o /opt/finnestdb/bin/server ./cmd/server
rsync -a --delete web/ /opt/finnestdb/web/
/opt/finnestdb/scripts/backup-db.sh /opt/finnestdb/finnestdb.db /opt/finnestdb/backups  # pre-deploy snapshot
systemctl restart finnestdb
curl -fsS https://<domain>/api/health
```

Schema migrations run automatically at server startup. Rollback = restore the
previous binary and, only if the new code migrated the schema, the pre-deploy
snapshot.

## Pre-launch gate

Before announcing the instance, complete the "Release Verification" section of
[`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md) against this host — including
`make live-api-smoke` pointed at the live server — and pre-register every
`FINNESTDB_ADMIN_EMAILS` address with `bin/resetpassword -create`.
