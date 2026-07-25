# FinnEst - Michael's Guide

_Created 2026-07-04. For Michael (@chickendude) and any AI agent he points at
this repo. If you're an agent answering Michael's questions: read
[`../AGENTS.md`](../AGENTS.md) and [`../CONTEXT.md`](../CONTEXT.md) first, then
use the routing table below. Check [`DECISIONS.md`](DECISIONS.md) before
re-litigating anything - most "why" questions are already answered there._

## Run it locally (fastest path)

Prerequisites: Go 1.21+, Rust (stable), Node 18+, SQLite. Python is only
needed for parser evaluations, not for running the app.

```bash
git clone <repo-url> && cd finnestdb
make build                    # Rust parser + Go server
cd web && npm install && npm run build && cd ..
```

**Get a database.** The app needs `finnestdb.db` (the dictionary + app data,
5+ GB fully populated). Two options:

- **Option A - copy Sagar's DB** (fastest, recommended for testing what he
  sees): get `finnestdb.db` from Sagar and drop it in the repo root. Don't
  copy any `-wal`/`-shm` sidecar files; the server recreates them.
- **Option B - build from scratch**: `make run-local` runs
  `scripts/setup-local.sh`, the single bootstrap for a fresh clone (downloads
  and imports the dictionaries; the slow steps can be skipped with
  `SKIP_EKILEX_DETAILS=1 SKIP_SILVER=1 SKIP_UD=1`). Then verify with
  `make doctor`.

**Seed the production-shaped extras** (both options):

```bash
# Admin account first (admin = parser workbench + feedback triage;
# also owns the official starter decks):
go run ./cmd/resetpassword -create -admin -email you@example.com
export FINNESTDB_ADMIN_EMAILS=you@example.com   # keep set when running the server

make import-gold-surfaces            # correction-acceptance safety rail (empty = no-op)
make fetch-frequency-baselines       # OpenSubtitles lists into localdata/frequency/
# Top-1000 official decks, with real corpus example sentences on the cards.
# -replace deletes a prior same-title deck first, so reseeding never
# duplicates it (the flag's absence only warns and creates a duplicate).
go run ./cmd/seedcolddeck -lang FI -owner-email you@example.com \
    -examples testdata/starter-examples/fi-examples-v1.tsv -replace
go run ./cmd/seedcolddeck -lang ET -owner-email you@example.com \
    -examples testdata/starter-examples/et-examples-v1.tsv -replace
```

**Run it:**

```bash
make run                      # serves on :8080; or ./finnestdb -addr 127.0.0.1:8080
```

Open http://localhost:8080, register with your admin email, and you get the
full learner product plus the Admin menu.

## Test it

- **Manual product walkthrough**: the instructions live in
  [`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md) under "First-experience
  quality check" - that section is the launch bar. Grade findings
  `blocker` / `serious` / `minor` as described there.
- **Automated release-candidate pack**: `make first-experience-rc` - runs the
  parser fixture checks and the Playwright RC specs from the shared manifest
  at `testdata/first-experience-rc/manifest.json`, then points you back at
  the manual walkthrough.
- **Full test suite**: `go test ./... -count=1` (needs the Rust parser built
  first) and `cd web && npx playwright test` (boots its own server on :8081).
- **Parser quality**: `make compare-parsers` (FI) / `make compare-parsers-et`
  (ET) - needs `make setup-nlp` once for the Omorfi/EstNLTK baselines.
  Method and how to read the numbers: [`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md).
- **Release checks**: `make live-api-smoke` (API/security probes against a
  running server) and `make db-invariants` (DB integrity).

## "How does it do that?" - question routing

| Your question | Read this |
|---|---|
| What is the product? How does a learner use it? | [`FEATURES.md`](FEATURES.md), then [`USER_FLOWS.md`](USER_FLOWS.md) (screen-level) |
| What does this term mean (Inspect, Known Surface Form, Meaning Check, quarantine…)? | [`../CONTEXT.md`](../CONTEXT.md) - the shared vocabulary |
| **Why** did we decide X? | [`DECISIONS.md`](DECISIONS.md) - 29 decisions, newest first, each with context/reasoning/trade-offs. The question-by-question trail behind Decisions 23–29 is in [`grill-sessions/2026-07-03-product-readiness.md`](grill-sessions/2026-07-03-product-readiness.md) (audit trail only) |
| How does the parser work? | [`../ARCHITECTURE.md`](../ARCHITECTURE.md); Decisions 1, 2, 5 in `DECISIONS.md`; [`LEXICAL_PLAN.md`](LEXICAL_PLAN.md) for the dictionary layer |
| How good is the parser, and how do we know? | [`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md), [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md), frozen baselines under `docs/baselines/` |
| What's left before we can go live? | [`../TODO.md`](../TODO.md) "Public alpha gates" + "Alpha launch issue ledger"; the quality rubric is [`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md) "Alpha Go/No-Go Rubric" |
| What changed recently / while I was away? | [`CHANGELOG.md`](CHANGELOG.md) (newest first) + verification reports under `docs/launch-readiness/` |
| How do learner corrections / feedback / quarantine work? | [`PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md); classification model in [`CORRECTION_TAXONOMY.md`](CORRECTION_TAXONOMY.md) |
| How do decks, review, coverage, FSRS work? | [`srs-deck-spec.md`](srs-deck-spec.md) |
| What's shared vs FI-specific vs ET-specific? Is Estonian equal? | [`CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) + the FI/ET parity audit under `docs/launch-readiness/` |
| How do I deploy it to a real host? | [`DEPLOYMENT.md`](DEPLOYMENT.md) - TLS, systemd, backups, purge cron, env vars, seeding |
| Is some tool/model/table missing on my machine? | Run `make doctor` first, then [`LOCAL_TOOLING.md`](LOCAL_TOOLING.md) |
| Where does doc X belong / which doc is canonical? | [`INDEX.md`](INDEX.md) "Canonical doc roles" |

## Notes for Michael's agents

- `docs/grill-sessions/` files are working logs - use them to recover why a
  question was asked, never as the implementation backlog. The backlog is
  `TODO.md`.
- Don't create new planning documents; Decision 24 and Q60 settled the doc
  model. Update the canonical doc per the roles table in `INDEX.md`.
- The repo folder, module paths, and `finnestdb.db` keep the technical name;
  the product is **FinnEst** in user-facing copy (Decision: grill Q53).
- The DB in the repo root may be a 5+ GB real database. Never delete or
  regenerate it casually; small ~100 KB `finnestdb.db` files are stubs.

## Current snapshot (as of 2026-07-04)

The 2026-07-02 launch stack (PRs #240–#249) and the overnight
2026-07-04 launch-gate run (PRs #250–#259) are merged: anonymous parser demo,
flag-only feedback, correction issues + quarantine, embedded catalog, parser
backpressure, RC pack, and surface-card identity + narrow FSRS (flag off).
Start from
[`launch-readiness/2026-07-04-overnight-report.md`](launch-readiness/2026-07-04-overnight-report.md)
- it lists exactly what's done and the human actions left before go-live.
