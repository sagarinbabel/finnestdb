# Overnight Launch-Gate Run - 2026-07-04

Audience: Sagar and Michael. This is the "what happened while you slept"
report and the go-live handoff. Everything below is merged to `main` and CI-green
(Go unit tests, lint, full Playwright suite) as of the last merge,
[PR #259](https://github.com/sagarinbabel/finnestdb/pull/259).

## What shipped tonight (PRs #250–#259)

| PR | What | Gate |
|---|---|---|
| [#250](https://github.com/sagarinbabel/finnestdb/pull/250) | 2026-07-03 product-readiness grill promoted into durable docs (CONTEXT.md, Decisions 23–29, alpha gates, go/no-go rubric, FinnEst naming), reconciled with the merged launch stack | Documentation consolidation, brand pass |
| [#251](https://github.com/sagarinbabel/finnestdb/pull/251) | `docs/FOR_MICHAEL.md` - local run/test path + question→doc routing table; GETTING_STARTED refresh | Handoff |
| [#252](https://github.com/sagarinbabel/finnestdb/pull/252) | FI/ET journey-first parity audit - **conditional pass**, full parity in every exercised learner journey | FI/ET equal status |
| [#253](https://github.com/sagarinbabel/finnestdb/pull/253) | Flag-only parser feedback (Phase 1b): report "this looks wrong" without proposing a fix; no lexical writeback until an admin supplies a concrete correction | Parser feedback |
| [#254](https://github.com/sagarinbabel/finnestdb/pull/254) | First-experience RC pack skeleton: shared manifest, Go runner, Playwright spec, `make first-experience-rc` | First-experience bar |
| [#255](https://github.com/sagarinbabel/finnestdb/pull/255) | Anonymous parser demo: landing paste→parse→explore, `FINNESTDB_ANON_MAX_CHARS` (20k default) enforced pre-parse, signup ribbon | Anonymous demo |
| [#256](https://github.com/sagarinbabel/finnestdb/pull/256) | Correction issues + admin-only quarantine (Phase 1c): global issue grouping, quiet suppression from queues and stats, restore preserves scheduler state | Quarantine |
| [#257](https://github.com/sagarinbabel/finnestdb/pull/257) | Parser backpressure (semaphore, anonymous-sheds-first, 503+Retry-After) + `cmd/loadtest` + local 1,000-VU evidence | Capacity |
| [#258](https://github.com/sagarinbabel/finnestdb/pull/258) | Embedded text catalog: mechanism (generator, API, cold-start UI) + 6 license-clean FI/ET texts | Embedded catalog |
| [#259](https://github.com/sagarinbabel/finnestdb/pull/259) | Surface-form card identity migration + narrow FSRS behind `FINNESTDB_FSRS_ENABLED` (default **off**) | Review readiness |

Load-test headline (laptop, production-size DB): signed-in traffic protected
~4× over anonymous under saturation; deck/review reads never errored at 1,000
concurrent virtual users (p95 675 ms). Full table:
[`2026-07-04-load-test.md`](2026-07-04-load-test.md).

## Gate status (see `TODO.md` "Public alpha gates" for the ledger of record)

Done: anonymous demo · signed-in loop · open signup · email-verification
posture · parity audit (PARITY-1 in ledger) · go/no-go rubric defined · docs
consolidation · brand pass · parser-feedback gate (1b+1c) · quarantine
behavior · known-word import docs · production safety.

Open (all with concrete remaining steps, none blocked on code that doesn't
exist):

1. **First-experience quality bar** - RC skeleton runs; pending cases:
   anonymous-demo FI/ET (surface now exists - unskip is a small follow-up),
   known-word-import FI/ET, parser-feedback ET. Final RC pass + manual
   walkthrough is the launch decision itself.
2. **1,000-concurrent target** - re-run `cmd/loadtest` on the production host;
   wire parser latency/rejection into monitoring.
3. **Embedded catalog** - full 36-text matrix + human difficulty sanity-check.
4. **Review readiness** - staging validation with seeded histories, then flip
   `FINNESTDB_FSRS_ENABLED`.
5. **Surface-first learner model** - known-vocabulary table (cards are done).
6. **Ambiguous meaning flow** - needs the Finnish-first ambiguity eval slice
   first; deliberately not attempted overnight.

## Human actions needed (the go-live checklist for you two)

1. **Michael - run it locally**: follow [`docs/FOR_MICHAEL.md`](../FOR_MICHAEL.md)
   top to bottom. It was written tonight for exactly this.
2. **Michael - deploy**: execute [`docs/DEPLOYMENT.md`](../DEPLOYMENT.md) on the
   host. Note the new env vars (`FINNESTDB_ANON_MAX_CHARS`,
   `FINNESTDB_PARSER_MAX_CONCURRENCY`, `FINNESTDB_PARSER_QUEUE_TIMEOUT_MS`,
   `FINNESTDB_FSRS_ENABLED` - leave FSRS off at first deploy) and the seeding
   steps: `make import-gold-surfaces` + `cmd/seedcolddeck` for both languages
   (**this closes ledger item PARITY-1**).
3. **Michael - production load re-run**: `cmd/loadtest` against the real host
   per [`2026-07-04-load-test.md`](2026-07-04-load-test.md).
4. **Sagar - catalog sanity-check (FI)** and find an **Estonian reviewer (ET)**:
   six texts await `difficulty_review` sign-off; ET Gutenberg is effectively
   empty, so sourcing more real published ET texts is an open question.
5. **Sagar - parity-audit QA account**: user 28
   (`parity-audit-2026-07-04@example.test`, admin) plus probe rows are in the
   local DB - keep as a standing QA account or delete per the audit's cleanup
   appendix.
6. **Sagar - parser baseline re-freeze** (maintainer-local FST tables needed).
7. **Both - the launch call**: run `make first-experience-rc`, do the manual
   walkthrough in `GO_LIVE_CHECKLIST.md`, grade findings, and apply the
   go/no-go rubric.

## Provenance

Work was executed by model-matched subagents in isolated worktrees, one PR per
gate, sequentially rebased and merged only on green CI plus orchestrator diff
review. Two review interventions worth knowing about: the RC pack's
parser-feedback spec was adapted to the flag-only two-path modal (caught by CI),
and the surface-card migration's collision guard was upgraded from a log line to
a transaction-rollback abort before merge. `@codex review` was not requested on
tonight's PRs (codex is out of credits); #211 and #212 remain open and untouched.
