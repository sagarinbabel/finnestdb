# FinnEst TODO — Status & Action Items

_Current as of 2026-07-03 — see [docs/CHANGELOG.md](docs/CHANGELOG.md) for revisions._

## Purpose

This is the single repo-level task list. It answers two questions for any reader:

1. **What's in main today?** — what works, what was shipped.
2. **What's next?** — open work, by area.

Other status lives elsewhere:
- **Why** decisions were made → [`docs/DECISIONS.md`](docs/DECISIONS.md)
- **What changed when** → [`docs/CHANGELOG.md`](docs/CHANGELOG.md)
- **Measured parser quality over time** → [`docs/PARSER_EVOLUTION.md`](docs/PARSER_EVOLUTION.md)
- **System architecture** → [`ARCHITECTURE.md`](ARCHITECTURE.md), [`docs/LEXICAL_PLAN.md`](docs/LEXICAL_PLAN.md)
- **Product framing** → [`docs/FEATURES.md`](docs/FEATURES.md)
- **Release quality bar** → [`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md)

## Table of Contents

- [LLM handoff read order](#llm-handoff-read-order)
- [Public alpha gates](#public-alpha-gates)
- [Alpha launch issue ledger](#alpha-launch-issue-ledger)
- [Post-launch roadmap checkpoints](#post-launch-roadmap-checkpoints)
- [What's in main](#whats-in-main)
- [What's not in main yet](#whats-not-in-main-yet)
- [Open PRs](#open-prs)
- [Research Goals](#research-goals)
- [Notes & historical](#notes--historical)
  - [Critical Findings (PRD review, 2026-04-29)](#critical-findings-prd-review-2026-04-29)
  - [Consumer alpha execution plan (2026-04-29)](#consumer-alpha-execution-plan-2026-04-29)
  - [Consumer flow review (2026-05-07)](#consumer-flow-review-2026-05-07)

## LLM handoff read order

For a new agent taking over public-alpha planning or implementation, read:

1. [`AGENTS.md`](AGENTS.md) for repo rules and local tooling constraints.
2. [`CONTEXT.md`](CONTEXT.md) for shared product vocabulary.
3. This file's [Public alpha gates](#public-alpha-gates) and
   [Alpha launch issue ledger](#alpha-launch-issue-ledger).
4. [`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md) for the
   alpha go/no-go rubric and release checks.
5. [`docs/DECISIONS.md`](docs/DECISIONS.md), especially Decisions 23-29.
6. The relevant implementation spec:
   [`docs/USER_FLOWS.md`](docs/USER_FLOWS.md),
   [`docs/srs-deck-spec.md`](docs/srs-deck-spec.md),
   [`docs/PARSER_FEEDBACK_LOOP.md`](docs/PARSER_FEEDBACK_LOOP.md),
   [`docs/CROSS_LANGUAGE_STRATEGY.md`](docs/CROSS_LANGUAGE_STRATEGY.md), or
   [`ARCHITECTURE.md`](ARCHITECTURE.md).

Do not execute directly from `docs/grill-sessions/`; those files are audit
trails. Stable grill decisions should already be promoted into this TODO,
`CONTEXT.md`, `DECISIONS.md`, and the relevant specs.

## Public alpha gates

Consolidated from the 2026-07-03 product-readiness grill. Use this section as
the implementation-start checklist, then follow the detailed tasks below.

Note: this launch bar was intentionally expanded on 2026-07-03. The 2026-07-02
launch stack (PRs #240-#249) treated hardening + ops + deferrals as the
remaining launch work; the product-readiness grill then added the gates below
(surface-first card identity, narrow FSRS, embedded catalog, flag-only
feedback + quarantine, RC pack, load test, FI/ET parity audit) as deliberate
scope, not accidental creep. See Decisions 23-29.

- [x] **Anonymous parser demo** (shipped 2026-07-04): unsigned visitors can
      paste text on the landing form, parse it, get a parsed word list, and
      explore the list (POS filters, sorting, row expansion, definitions/forms/
      examples, counts). Stateless, ephemeral, rate-limited, and capped below
      signed-in text size via `FINNESTDB_ANON_MAX_CHARS` (default 300,000, vs the
      1,500,000 signed-in cap), enforced server-side before parser work and
      surfaced to the client through `/api/me`. Save/deck/review, known/ignored
      state, imports, parser feedback, history, and account settings stay
      sign-in-gated (hidden via `data-role-show`). Anonymous results carry a
      dismiss-per-session sign-up ribbon (reappears on next parse) and a privacy
      footer. Remaining: tune the default cap through the 1,000-concurrent load
      test (see the load-test gate below).
- [x] **Signed-in learner alpha loop** (verified in the 2026-07-04 parity audit): signed-in dashboard/Inspect -> parse real
      text -> save deck/add to deck -> review. Open signup should make this
      loop available immediately after account creation.
- [x] **Open signup access posture** (shipped; hardening landed in the 2026-07-02 launch stack): public alpha allows self-serve account
      creation, not invite-only or waitlist-first access. Treat abuse controls,
      rate limits, account deletion, retention, admin visibility, auth
      hardening, and basic monitoring as launch gates because signup is open.
- [x] **Email verification posture** (current behavior matches: no verification gate on first value): do not block first value on verification.
      After signup, allow parse -> save deck -> first review immediately; gate
      high-volume parsing, repeated feedback, exports if enabled, account
      recovery, and trust-weighted signals on verified email.
- [ ] **1,000-concurrent-user launch target**: load-test and configure graceful
      degradation for roughly 1,000 concurrent users. Under pressure, throttle
      anonymous/oversized parses first, preserve core signed-in review/deck
      actions where possible, return clear retry behavior, and document the
      horizontal scale path for adding servers. Tune the anonymous text-size cap
      through this load test; do not launch with anonymous using the same
      1,500,000-character ceiling as signed-in parsing.

      **Progress 2026-07-04**: parser concurrency semaphore
      (`FINNESTDB_PARSER_MAX_CONCURRENCY` / `FINNESTDB_PARSER_QUEUE_TIMEOUT_MS`,
      `internal/api/parser_limiter.go`) and `cmd/loadtest` shipped. Anonymous
      parse draws from a smaller sub-pool so it sheds first under saturation;
      429 (rate limit) and 503 (parser saturation) both carry `Retry-After`.
      Local laptop runs at 50/200/500/1000 concurrent virtual users plus a
      dedicated anonymous-heavy mixed stage confirm: anonymous parse sheds at
      a meaningfully higher rate than signed-in parse (e.g. 50.2% vs. 12.7% in
      the anon-heavy stage), and deck/review reads never errored or exceeded
      ~700ms p95 even at full 1000-VU saturation. The anonymous cap was
      20,000 chars during this run and was re-checked against this load, not
      changed — no evidence justified lowering it (see report for the
      `custom`-parser-mode caveat). Note: the default was raised to 300,000
      later the same day (commit 7bff399); the larger cap has not itself been
      load-tested and should be covered by the production-host re-run. Full
      method, numbers, and hardware caveat:
      [`docs/launch-readiness/2026-07-04-load-test.md`](docs/launch-readiness/2026-07-04-load-test.md).
      **Remaining, gate stays open**: this was a laptop run against a local DB,
      not the production host. Re-run `cmd/loadtest` against the real
      production host at 1,000 concurrent users, confirm the concurrency
      default suits its actual core count, and wire parser
      latency/error/rejection counts into production monitoring before
      checking this off.
- [x] **FI/ET equal-status parity audit** (run 2026-07-04, conditional pass — see docs/launch-readiness/2026-07-04-fi-et-parity-audit.md; sole alpha-blocker PARITY-1 is the deploy-time starter-deck seeding, tracked in the ledger): Finnish and Estonian launch with equal
      product status. Before public alpha, run this journey-first: compare the
      same FI and ET learner/admin paths, then attach data, parser-quality,
      embedded-catalog, known-word import, deck/review, feedback/quarantine, UX
      copy, test, and production-artifact metrics under each step. Fix
      alpha-blocking gaps; document true language-specific differences without
      making either language feel secondary.

      **Audit run 2026-07-04**: journey-first pass against the production-size
      local DB (26.8M FI / 6.3M ET forms) plus a live server on `:8083`.
      Verdict: **conditional pass**. Anonymous parse, signed-in Inspect parse,
      deck save/detail/review, known-word import, and parser-feedback
      submission all showed full FI/ET parity with concrete evidence (100%
      resolution and gloss attachment both languages). One alpha-blocking
      finding: official "Top 1000" starter decks do not exist in the DB for
      either language despite a TODO.md entry claiming this shipped and was
      verified end-to-end (see ledger row PARITY-1 above). Several
      differences (FI/ET `translations`/`definitions` table population, UD
      gold-set breadth) were traced to documented, licensed, non-learner-
      visible causes and classified language-specific. Full detail, evidence,
      and cleanup appendix in
      [`docs/launch-readiness/2026-07-04-fi-et-parity-audit.md`](docs/launch-readiness/2026-07-04-fi-et-parity-audit.md).
- [x] **Alpha go/no-go rubric** (defined in docs/GO_LIVE_CHECKLIST.md; applying it at launch remains the final human step): launch when core journeys work end-to-end and
      every known rough edge is classified as non-dangerous under
      `docs/GO_LIVE_CHECKLIST.md` "Alpha Go/No-Go Rubric". Any issue touching
      privacy/security, retention, account deletion, data integrity, review
      state, parser feedback/quarantine, misleading parser confidence, overload
      behavior, or FI/ET equal status is a no-go until fixed or explicitly
      reclassified.
- [ ] **First-experience quality bar**: the first-run experience should feel
      excellent about 95% of the time before public alpha. Gate launch with a
      journey-first FI/ET release-candidate pack covering anonymous demo,
      embedded text, own-text Inspect, save deck, first review, known-word
      import, and parser feedback. The pack should be a checked-in, repeatable
      artifact with explicit FI/ET cases for curated embedded texts, realistic
      pasted texts, known-word imports, ambiguity/homograph handling,
      parser-feedback flows, deck save, and first review. Use one canonical
      manifest at `testdata/first-experience-rc/manifest.json` so parser checks,
      `web/tests` Playwright specs, and the manual walkthrough consume the same
      cases and fixtures. Build the manifest and a small skeleton runner as the
      first alpha implementation task; it may fail initially, but should become
      the concrete launch bar as missing flows land. Run it in two parts:
      automated parser/browser checks for deterministic behavior, plus a short
      manual product walkthrough for judgment calls about trust, clarity, and
      first-screen credibility. Grade findings as `blocker`, `serious`, or
      `minor`: blocker/serious findings stop launch unless fixed or explicitly
      reclassified with evidence; minor findings can ship only if they meet the
      non-dangerous rough-edge rubric and have a ledger row. A clean pass means
      no broken flow, no misleading state, no obvious high-severity parser/card
      issue in the learner's first screenful, and no latency/error behavior that
      makes the product feel unreliable. Privacy-preserving week-one telemetry
      validates this after launch, but it is not a public-alpha blocker if
      server logs and manual feedback review are available. Expose the automated
      portion through one top-level command, `make first-experience-rc`, which
      runs parser fixture checks and Playwright RC specs, then points at the
      manual walkthrough instructions in `docs/GO_LIVE_CHECKLIST.md` (they live
      there, not in a new doc; the manifest stays data-only — Q60).
      - [x] Skeleton shipped 2026-07-04: `testdata/first-experience-rc/manifest.json`
            (18 cases, FI+ET per journey), `cmd/firstexperiencerc` (Go runner),
            and `web/tests/first-experience-rc.spec.ts` (Playwright spec) exist;
            `make first-experience-rc` runs green end to end.
      - [x] Automated now: `embedded-text`, `own-text-inspect`, and
            `ambiguity-homograph` (FI `kuusi`/`tuli`/`voi` + one ET case) via
            the Go parser runner; `deck-save` + `first-review` (FI+ET) and
            `parser-feedback` (FI only) via Playwright.
      - [x] Pending journeys now covered (done 2026-07-04): `anonymous-demo`
            (FI+ET, driven against the manifest's embedded-text fixtures on the
            landing page), `known-word-import` (FI+ET, RC-fixture-driven on
            `/#/vocab`), and `parser-feedback` for ET (parity Playwright case
            alongside the existing FI one). Ambiguity-homograph journeys also
            gained a Playwright pass asserting the "Multiple possible
            meanings" panel and its flag-only escape, on top of the existing
            Go parser-quality assertions. `make first-experience-rc` now runs
            all 18 manifest cases with zero `automation:"pending"` entries.
      - [ ] Remaining human step: the manual product walkthrough (trust,
            clarity, first-screen credibility judgment calls per
            `docs/GO_LIVE_CHECKLIST.md`) and the go/no-go call itself. All
            automated coverage is in place; this checklist item stays open
            until that walkthrough runs and findings are graded.
- [x] **Documentation consolidation pass** (done 2026-07-03/04: handoff read order, canonical doc roles, FOR_MICHAEL guide): avoid adding new docs for execution
      ledgers. Keep launch issues in this TODO, keep the quality rubric in
      `docs/GO_LIVE_CHECKLIST.md`, and audit overlapping docs for merge,
      archival, or clearer source-of-truth pointers before public alpha.
- [x] **Brand normalization pass** (done 2026-07-03 in PR #250; historical dated entries keep FinEstDB): user-facing product name is **FinnEst**.
      Replace `FinEstDB` / `Finnest` / `FinnestDB` in current product docs and UI copy where
      it means the product. Do not rename the local folder, `finnestdb.db`,
      module paths, historical file names, or GitHub URLs without an explicit
      engineering rename plan.
- [ ] **Curated embedded text catalog** (mechanism + 6 initial license-clean texts shipped 2026-07-04 in PR #258; FI difficulty review done 2026-07-04 by Sagar — labels widened to a five-level scale with human overrides recorded in internal/catalog/reviews.json; the 4 machine-written texts replaced with real published sources 2026-07-04 after a naturalness review — FI/ET Wikipedia CC BY-SA articles + Juhan Liiv PD poem + Juhan Kunder PD folk tale, so no agent-authored text ships; the replaced FI article and all ET texts are back to pending human review; remaining: ET reviewer sign-off, re-review of the new FI article, full 36-text matrix): checked-in metadata and lazy-loaded full
      text fixtures from redistributable FI/ET sources; target matrix is
      stories/articles/poems x Easy/Medium/Hard x two texts per bucket per
      language, with computed difficulty and human sanity-check.
      Progress (2026-07-04): mechanism shipped — `internal/catalog` (`go:embed`
      metadata + fixtures), `cmd/gencatalog` (deterministic difficulty from a
      real custom-mode parse; thresholds in `docs/GO_LIVE_CHECKLIST.md`
      "Embedded catalog difficulty model"), `GET /api/catalog` (+ per-learner
      coverage) / `GET /api/catalog/{id}/text`, and dashboard/Inspect cold-start
      pickers. Initial honest coverage: 3 FI (Gutenberg public-domain poem +
      short story, 1 original CC0 article) and 3 ET (original CC0; ET Gutenberg
      was effectively unavailable) spanning ≥2 genres and ≥2 difficulty buckets
      each. Still open before the gate closes: the full 36-text matrix and the
      human sanity-check (Sagar FI, Estonian reviewer ET) — every entry ships
      `difficulty_review: "pending"`. Gate stays unchecked.
- [ ] **Surface-first learner model** (card identity migrated to surface-form cards 2026-07-04 in PR #259; the surface-first known-vocabulary table remains open): preserve submitted known surface forms,
      migrate alpha review identity to surface-form-in-context cards, and keep
      lemma/POS/dictionary entries as derived support.
- [~] **Ambiguous meaning flow**: context-free imports resolve lazily in real
      sentences; parse-result checks are non-blocking until deck save; low or
      unmeasured parser confidence shows **Multiple possible meanings**. Add the
      Finnish-first ambiguity eval slice before simplifying ambiguity UI.
      _(2026-07-04: **Multiple-possible-meanings shipped as the alpha default.**
      `/api/parse` enriches signed-in results with `ambiguous_surfaces` (FST-merged
      candidate set, quarantine-filtered); results rows carry the chip →
      expand → per-candidate "I know this meaning" / "Study this meaning" /
      "Not sure" plus the "None of these looks right" flag-only escape; explicit
      FST-sense study selections create cards on deck save via a narrow bypass of
      PR #269's dict-only expansion; known-word import reports
      `needs_sense_confirmation`; review card back carries the same flag-only
      escape. The single confident **Meaning Check** remains threshold-gated
      future work — no ambiguity class qualifies on the v1 slice
      (PARSER_EVAL_METHODOLOGY.md §4), so it is deliberately not built.)_
- [x] **Review readiness** — surface-card identity + narrow FSRS, FSRS now the
      default scheduler. Migrated card identity before FSRS; shipped narrow Go
      FSRS (default parameters, current Again/Hard/Good/Easy UI), the opt-out
      feature flag, lazy migration/fallback, and regression tests. _(2026-07-04:
      staging validation green across seeded histories, migration-at-scale,
      rollback, and a real-DB smoke — see
      [`docs/launch-readiness/2026-07-04-fsrs-validation.md`](docs/launch-readiness/2026-07-04-fsrs-validation.md).
      FSRS enabled by default; `FINNESTDB_FSRS_ENABLED=0` is the rollback lever —
      see DEPLOYMENT.md "FSRS scheduler rollout".)_
- [x] **Parser feedback alpha gate**: flag-only feedback, minimal
      `correction_issues` grouping, admin-only global quarantine, and quiet
      learner-facing suppression. `parse_feedback` stays raw intake; no broad
      in-app fix editor or separate Issues page. Flag-only feedback shipped
      2026-07-04 (Phase 1b); `correction_issues` grouping + admin-only
      quarantine + learner suppression shipped 2026-07-04 (Phase 1c).
- [x] **Quarantine behavior**: globally quarantined content disappears from
      learner review/new-card queues, deck word/due/new-card counts, and
      comprehension coverage/unlocks; `review_log` history/audit data remains.
      Restored items keep their existing `card_state` scheduler state.
      Shipped 2026-07-04 (Phase 1c).
- [x] **Known-word import polish** (documented in TODO item 12 + FOR_MICHAEL; .apkg upload remains tracked future work): document existing AnkiConnect and
      CSV/TSV/first-column import behavior; `.apkg` upload remains future work.
- [x] **Production safety** (shipped in the 2026-07-02 launch stack: retention/deletion, account deletion, abuse controls, admin gating, FIN-27 security review): keep parse source retention/deletion, account
      deletion, abuse controls, admin gating, and security review on the launch
      path.

## Alpha launch issue ledger

Use this section for public-alpha release issues. Do not create a separate
`ALPHA_LAUNCH_ISSUES.md` unless this table becomes too large for `TODO.md` to
remain readable.

Classify each issue with the rubric in
[`docs/GO_LIVE_CHECKLIST.md` "Alpha Go/No-Go Rubric"](docs/GO_LIVE_CHECKLIST.md#alpha-gono-go-rubric):

- `blocker`: must be fixed before public alpha.
- `non-dangerous rough edge`: can ship if it has owner/evidence/workaround and
  does not violate the rubric.
- `post-alpha`: tracked but not part of the launch bar.

| ID | Classification | Area | Affected journey/lang | Issue | Evidence | Owner | Exit / revisit condition |
|---|---|---|---|---|---|---|---|
| _TBD_ | _blocker / non-dangerous rough edge / post-alpha_ | _auth / parser / review / docs / ops / UX_ | _FI / ET / both_ | _Concise issue_ | _Test, audit note, screenshot, metric, or user report_ | _TBD_ | _Fix, workaround, or revisit trigger_ |
| PARITY-1 | blocker | ops / decks | both | Official "Top 1000" starter decks (`cmd/seedcolddeck`) do not exist in the DB for either language (`decks` has zero `is_public=1` rows), contradicting the `[x]` "shipped, verified end-to-end" claim at the "Cold-start Top 1000 CTA" entry below. Blocks the equal-status cold-start journey for both languages. | [`docs/launch-readiness/2026-07-04-fi-et-parity-audit.md`](docs/launch-readiness/2026-07-04-fi-et-parity-audit.md) journey 11 and ledger summary | _TBD_ | Run `cmd/seedcolddeck` for FI and ET against the launch DB per `docs/DEPLOYMENT.md`, confirm `decks.is_public=1` rows exist for both languages, then correct or re-confirm the TODO.md "verified end-to-end" claim. |
| DICT-1 | non-dangerous rough edge | parser / dictionary data | FI | Top-1000 FI starter deck resolved surface `ase` ("weapon", a top-frequency noun) to lemma `asea`/VERB with gloss "synonym of asettaa" instead of `ase`/NOUN "weapon". Root cause is NOT `seedcolddeck` ranking or parser candidate filtering: `finnestdb.db`'s `forms` table has zero rows for `(form='ase', lemma='ase', pos='NOUN')` — every other inflected form of noun `ase` (aseen, aseita, aseessa, aseeseen, ...) is present, but the bare nominative-singular self-mapping row is missing, while `(form='ase', lemma='asea', pos='VERB')` (an archaic imperative of `asettaa`) is the only row keyed by surface `ase`. `pickBestFormCandidate`/`filterLowValueAlternatives` never get a chance to prefer the noun because there is nothing to rank against — with one candidate row, that row wins by construction. `dict_metadata` shows the live FI import ran 2026-03-13, while `localdata/kaikki/kaikki.org-dictionary-Finnish.jsonl.gz` on disk has mtime 2026-05-07 (~7 weeks newer); the currently-downloaded kaikki dump's single `word="ase"/pos="noun"` entry does list `ase` with `tags:["nominative","singular"]`, so a re-import against the current file is expected to add the missing row. This is dictionary-import staleness, not a ranking bug — do not change `pickBestFormCandidate`/`filterLowValueAlternatives`/`seedcolddeck` to work around it. | `sqlite3 finnestdb.db "SELECT form,lemma,pos FROM forms WHERE lang='FI' AND form='ase'"` returns only the `asea`/VERB row; `SELECT imported_at,row_count FROM dict_metadata WHERE lang='FI'` shows 2026-03-13; PR fixing test-run findings (this PR) | _TBD_ | Re-run `go run ./cmd/importdict -lang fi -db finnestdb.db -file localdata/kaikki/kaikki.org-dictionary-Finnish.jsonl.gz` (or fetch a fresh dump) against a maintainer DB, confirm `SELECT form,lemma,pos FROM forms WHERE lang='FI' AND form='ase'` now includes `ase\|ase\|NOUN`, re-run `cmd/seedcolddeck` for FI to pick up the corrected resolution, then re-freeze the FI parser baseline per `docs/PARSER_EVAL_METHODOLOGY.md` since a dictionary re-import can shift eval numbers. |

## Post-launch roadmap checkpoints

These are not public-alpha blockers unless promoted into
[Public alpha gates](#public-alpha-gates) or the
[Alpha launch issue ledger](#alpha-launch-issue-ledger).

| Checkpoint | Timing | Why | Minimum acceptable fallback |
|---|---|---|---|
| Privacy-preserving first-experience telemetry | Week one / first post-alpha patch | Verify whether real users match the 95% first-experience bar | Server logs plus manual feedback/admin review until telemetry lands |

## What's in main

Snapshot of capabilities currently shipped on main, organized by area.

### Parser core

- Rust tokenizer (NFC, sentence splitting, punctuation, numeric-hyphen rules R1–R4 per [Decision 6](docs/DECISIONS.md))
- `basic` and `custom` parser modes
- FST as parallel scorer in dict step 1 ([PR #127](https://github.com/sagarinbabel/finnestdb/pull/127))
- FST candidate-merge FEATS enrichment ([PR #129](https://github.com/sagarinbabel/finnestdb/pull/129))
- Per-attribute FEATS eval ([PR #130](https://github.com/sagarinbabel/finnestdb/pull/130))
- Parser behavior stamp `parser-v2` / `2026.05.15a`
- Lexical overlays, bad-lemma blocklists, MA/A-infinitive biasing, and low-value dict-alternative suppression from the 2026-05-12 parser-quality run
- Multi-source dictionary with row-level provenance and source-priority-first ranker ([Decision 21](docs/DECISIONS.md))
- Multi-lemma surface forms (`forms` PK = `(form, lang, lemma, pos)`)
- Possessive suffix stripping (FI), compound splitting (FI/ET), case-suffix matcher (FI/ET)
- Case-suffix grammar-label stopgap (`attachCaseLabelIfStemMatches`) — transitional

### Lexical layer

- Schema: `lemmas`, `forms`, `translations`, `definitions`, `dict_metadata` with source priorities
- FI: kaikki.org Finnish + Kotus sanalista (populates `paradigm_class`)
- ET: kaikki.org Estonian + Ekilex bulk pipeline (~178k lemmas + ~6.2M form rows)
- ET FEATS via Ekilex morph_code → UD FEATS

### Evaluation

- Gold sets: ~9.8k FI committed, ~37.9k ET local-only (CC BY-NC-SA), ~37k FI train (local)
- 2026-05-07k baseline frozen ([PR #135](https://github.com/sagarinbabel/finnestdb/pull/135)) — FI 8 datasets / 61,927 tokens, ET 2 datasets / 190 tokens
- Per-attribute FEATS eval (Case, Number, Tense, Mood, Voice, Person)
- Bootstrap CIs in comparison reports ([PR #114](https://github.com/sagarinbabel/finnestdb/pull/114))
- Held-out discipline: dev splits gitignored under `localdata/`

### App surface

- Real password auth (Argon2id, sliding sessions)
- Role-aware: anonymous / user / admin
- Routes: landing, sign-in, Inspect, Decks, Review, admin workbench, admin parse-feedback queue
- Inspect/workbench `.txt`, `.md`, and `.epub` upload extraction via `POST /api/import/extract`
- Deck CRUD, sentence/occurrence persistence, multi-lemma deck cards
- Known-word import + delete + list (`POST /api/known-words`), with paste,
  `.txt` / `.csv` / `.tsv` / `.md` first-column file import, and AnkiConnect
  local-deck import/sync. Anki `.apkg` upload is not implemented.
- Parse feedback submission + admin triage. Accepted lemma/POS feedback writes
  `custom_overrides`; accepted grammar labels become UD FEATS on the override
  row; acceptance is eval-gated against `gold_surfaces` (HTTP 409 on
  contradiction) and repeat corrections auto-queue as `gold_candidates`
  (Phases 1-4, shipped 2026-07-02). Flag-only feedback and quarantine are not
  built yet.
- FSRS review scheduler (`go-fsrs/v3`, default parameters), the default as of
  2026-07-04; hand-rolled step scheduler retained as the `FINNESTDB_FSRS_ENABLED=0`
  rollback fallback
- Hybrid language detection (warn/block on high-confidence conflict; advisory on unknown)
- Progress dashboard: known count, due count, cards in review, reviews today,
  14-day activity chart, per-deck comprehension (shipped 2026-07-02)
- Per-deck comprehension prediction (`GET /api/decks/{id}/comprehension`,
  `comprehension_pct` on deck list)
- Cold-start "Top 1000" FI/ET official starter decks (`cmd/seedcolddeck`,
  operator-seeded at deploy time); starter cards now carry curated corpus
  example sentences via `cmd/pickexamples` +
  `testdata/starter-examples/{fi,et}-examples-v1.tsv` (attach with
  `seedcolddeck -examples`)

### Data and infrastructure

- Single-folder bootstrap: gitignored runtime data under `localdata/` ([PR #131](https://github.com/sagarinbabel/finnestdb/pull/131))
- `docs/data_enhancement.md` ledger of every external corpus
- ARTIFACT_POLICY: no transducer blobs in git, generated factual tables only via local generators
- Public frequency baselines via `cmd/fetchfrequency` ([PR #134](https://github.com/sagarinbabel/finnestdb/pull/134))
- Setup automation: `scripts/setup-local.sh` (10 best-effort steps)
- Release verification targets: `make live-api-smoke` for live API/security
  probes and `make db-invariants` for production-candidate SQLite integrity,
  orphan, overlap, and source-breakdown checks
- Production deployment runbook ([`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md))
  and operator password-reset / pre-registration CLI (`cmd/resetpassword`)

## What's not in main yet

Open work, organized by area. Each entry is brief; follow cross-links for detail.

### Parser quality

- [x] **ET lemmatizer table generator** — shipped in `cmd/genlemmatizertables -lang et -hfstol ...` plus `make gen-lemmatizer-tables-et`. Remaining production work is a real ET wordlist, provenance notes, row counts, and a fresh eval gate before relying on a full ET table in deployment.
- [x] **Re-freeze baselines once gold sets get a `feats` field** — done 2026-05-07k via PR [#139](https://github.com/sagarinbabel/finnestdb/pull/139). All 6 manual gold sets now carry FEATS (`cmd/enrichgoldfeats`); new baselines committed at `docs/baselines/2026-05-07-feats-rich-*`. The `feats_attributes` table is non-empty for omorfi (FI) and estnltk (ET); for `custom` it stays at 0% until the live SQLite DB is re-imported with the new FEATS-aware `cmd/importdict` (runbook in the methodology doc).
- [x] **Re-import the live DB to populate `forms.feats`** — done and verified 2026-07-02. The live DB carries FEATS on 26.6M/26.8M FI rows (99.3%) and 6.0M/6.3M ET rows (96%), and live `custom`-mode parses emit full UD FEATS end-to-end (verified: FI "talossa" → `Case=Ine|Number=Sing`, verb morphology on "istuu"; ET "majas" → `Case=Ine|Number=Sing`). The remaining ~1–4% are rows whose upstream source carries no morph tags. Future DB rebuilds must preserve these imports before new parser baseline claims.
- [ ] **Remove the `attachCaseLabelIfStemMatches` stopgap** in `internal/store/dict.go` once the FST runtime emits FEATS for direct dict hits. PR [#139](https://github.com/sagarinbabel/finnestdb/pull/139) added `featsFromCaseLabel` so the stopgap's output is at least UD-shaped (`Case=Xxx`); the remove condition still requires production FST tables.
- [ ] **Re-run FI/ET gold baselines** after each fix and keep only justified gains. Use the new eval regressions to prioritize parser fixes. Recursive compounds and consonant gradation are *not* candidates here — they're gated behind the FST migration. See [`docs/DECISIONS.md`](docs/DECISIONS.md) Decision 5.
- [ ] **Finnish-first ambiguity eval slice**. Add focused contextual
      homograph/disambiguation cases before simplifying meaning-check UI based
      on parser confidence. Start with FI pairs like `kuusi` (six/spruce),
      `tuli` (came/fire), and `voi` (can/butter), then add ET parity cases.
      Measure candidate inclusion, selected lemma+POS, FEATS where applicable,
      and compare `custom` against Omorfi for FI / EstNLTK for ET.
      - [x] **Spec + verified gold data** (2026-07-04, `docs/ambiguity-eval-spec`):
            spec written into `docs/PARSER_EVAL_METHODOLOGY.md` §Ambiguity and
            meaning-check calibration (metrics, confidence-proxy definition,
            gold format, threshold→UI rule, ET parity, runner plan). Gold slices
            committed: `testdata/parser-eval/fi-ambiguity/fi-ambiguity-v1.json`
            (48 cases, 21 classes) + `et-ambiguity-v1.json` (13 cases, 6 classes)
            + `README.md`. **Verified baseline (parser 2026.05.15a):** FI
            selection 36/48 = 75.0%, candidate inclusion 35/48 = 72.9%; ET
            selection 7/13 = 53.8%, candidate inclusion 13/13 = 100%. Key finding:
            FI and ET fail differently — FI's blocker is candidate inclusion
            (kaikki `forms` stores only one reading per cross-POS homograph, so
            `kuusi`/`tuli`/`voi` second sense is absent from `BatchLookupAllForms`
            even though the FST knows it), ET's blocker is selection ranking
            (Ekilex populates all candidates but the pick prefers VERB on
            cross-POS collisions). (S — done)
      - [x] **Build `cmd/ambiguityeval`** (M): load a `slice:"ambiguity"` gold
            file, per case run `parsecore.Analyze(...,"custom")` for the pick and
            `store.BatchLookupAllForms` for the candidate set, emit
            candidate-inclusion + selection-accuracy + proxy-stratified accuracy
            keyed by `ambiguity_class`. Separate from `cmd/parsertest` so
            ambiguity metrics don't distort the token-accuracy summary schema.
            (M — done) Shipped as `cmd/ambiguityeval`. Measured on the production
            DB: FI selection 34/48 = 70.8%, candidate inclusion 35/48 = 72.9%; ET
            selection 6/13 = 46.2%, candidate inclusion 13/13 = 100.0%. Candidate
            inclusion matches the spec's hand-verified baseline exactly on both
            languages; selection differs by 2 FI cases and 1 ET case because the
            runner's exact-`Form`-match occurrence lookup (same convention as
            `internal/eval.findOccurrence`) does not fold case, and 3 of the 61
            gold cases have a sentence-initial target surface capitalized in the
            parse output but lowercase in `expected_candidates`/gold `surface`
            (`fi-amb-sain-2`, `et-amb-pea-2` are pure casing artifacts — the
            parser's pick is actually correct; `fi-amb-kayda-2` is a genuine miss,
            confirmed independently: `lääkärikäynti` compound-splits to its own
            lemma and never appears in `BatchLookupAllForms` at all, so the
            expected `käydä`/`käynti` senses are unreachable, an instance of the
            same candidate-set-gap failure mode as `kuusi`/`tuli`/`voi`, not a
            runner bug). Not fixed here per instructions not to tune the runner
            to match the spec's ad-hoc numbers.
      - [x] **Wire `make compare-ambiguity`** (S): parallel to
            `make compare-parsers`; discover `testdata/parser-eval/*-ambiguity/*.json`,
            run FI + ET, write a report; extend `scripts/freeze-baseline.sh` usage
            so a `YYYY-MM-DD<rev>-fi-ambiguity` report is append-only (see
            `docs/baselines/README.md`).
            (S — done) `scripts/compare-ambiguity.sh` discovers gold files under
            `testdata/parser-eval/*-ambiguity/*.json`, runs `cmd/ambiguityeval`
            against `finnestdb.db`, and writes a dated JSON report under
            `reports/parser-eval/`. The formal `freeze-baseline.sh` naming
            extension (`YYYY-MM-DD<rev>-fi-ambiguity`) is documented in
            `docs/baselines/README.md` but the actual freeze stays a maintainer
            action, as instructed — no frozen baseline is committed by this PR.
      - [x] **Close the FI candidate-inclusion gap** (M): merge FST-known
            readings into `store.BatchLookupAllForms` (or a candidate-set path the
            product's Multi-Lemma expansion consumes) so `kuusi`/`tuli`/`voi`
            second sense becomes offerable. Gated behind the eval slice: re-run
            `make compare-ambiguity` and keep only justified gains. Do NOT extend
            the frozen suffix tables (DECISIONS.md Decision 5); this is FST-merge
            + candidate-API work.
            (M — done) Shipped as `store.BatchLookupAllFormsWithOptions(...,
            AllFormsOptions{MergeFSTReadings: true})`: FST-known homograph readings
            the `forms` table omits are appended to the candidate set, deduped by
            `(lemma, POS)` against dict rows and ranked below authoritative
            dict/override candidates (source-priority model), analyzer emission
            order preserved. `cmd/ambiguityeval` uses it. **FI ambiguity candidate
            inclusion 35/48 → 46/48 (72.9% → 95.8%); selection accuracy unchanged
            at 34/48 = 70.8%.** Classes `kuusi`/`tuli`/`voi`/`palaa`/`alusta` reach
            100% inclusion (`tie` → 2/2). The two remaining misses (`sanoin`,
            `lääkärikäynti` compound) are genuinely outside the FST-merge
            mechanism and not special-cased. FI + ET headline baselines byte-stable
            (accuracy columns; only timing noise differs). The merge is gated OFF
            the deck / import expansion path (still dict-only
            `BatchLookupAllForms`), so learner-facing deck word counts are
            unchanged. Parser stamp `2026.05.15a` → `2026.05.15b`.
      - [ ] **Expand + freeze** (M): raise per-class case counts to N ≥ 4
            (≥ 2/sense) so control classes become threshold-eligible; expand ET to
            ~20-30 cases; freeze the first formal ambiguity baseline and record it
            in `PARSER_EVOLUTION.md`.
- [ ] **Disambiguation model**: select UD treebanks (FI, ET); train initial POS tagging model; establish evaluation metrics and baseline; version model artifacts.
- [ ] **Custom dictionary knowledge graph spike**: separate custom lexicon for FI/ET that accumulates data from multiple upstream dictionaries plus manual edits; provenance tables; compiled read model for hot-path lookups; live-merge admin view; manual injection flows for curated edits, CSV/JSONL imports, precedence rules. Michael owns the full Ekilex `word/details` enrichment scrape (~87+ GB raw JSON, 174k headwords) — resumable batch job with checkpointing by `word_id`, conservative rate limiting, retry/backoff, raw responses in ignored `localdata/`, compact reduced JSONL artifact for review.

### Learner experience

- [ ] **Migrate alpha review identity and scheduler before public alpha**.
  `internal/store/db.go::nextAlphaStepScheduleForRating` is a hand-rolled step scheduler
  with hardcoded day arrays — **not** FSRS. [`docs/srs-deck-spec.md`](docs/srs-deck-spec.md)
  §13–24 already recommends [`go-fsrs`](https://github.com/open-spaced-repetition/go-fsrs).
  Alpha scope means runtime scheduling only: default FSRS parameters, current
  Again/Hard/Good/Easy UI, due-date calculation, basic migration/fallback, and
  regression tests. Do **not** bundle parameter optimization, `fsrs-rs`,
  rescheduling tools, mature-card analytics, or a broad review redesign into this
  alpha migration.
  - [x] Alpha card identity decision: public alpha review cards are
        surface-form-in-context cards, not long-lived lemma/POS cards.
  - [x] Migrate card/deck/known/ignored review identity from lemma-backed cards
        toward surface-form cards before attaching real FSRS state. Keep
        lemma/POS/dictionary entries as derived support. _(2026-07-04: cards keyed
        `(user, lang, surface_norm, lemma, pos)`; `ensureSurfaceScopedCardsTable`
        migration preserves scheduler state; known/ignored/quarantine still key on
        `(lemma, pos)` sense scope, surface-only quarantine also matches
        `surface_norm`.)_
  - [x] For identical-looking surfaces with multiple supported meanings, create
        sense-aware surface cards and include a context sentence plus an explicit
        homograph note on the card, for example noun vs verb form. _(2026-07-04:
        homographs produce separate `(surface_norm, lemma, pos)` cards;
        `GetNextReviewCard` emits a homograph note; API/web render it.)_
  - [x] Resolve ambiguous context-free known-word imports lazily with contextual
        meaning checks. The "Study this meaning" action must indicate that it
        creates/keeps a review card now or creates one when the deck is saved,
        depending on context. _(2026-07-04: imports report
        `needs_sense_confirmation`; no upfront disambiguation. In parse results
        the study action says "Creates a review card when you save".)_
  - [x] In ephemeral parse results, make meaning checks non-blocking and pending:
        "Study this meaning" should mean "creates a review card when you save",
        not immediate card creation. _(2026-07-04: the chip is non-blocking;
        "Study this meaning" only marks `selected_senses` in the pending deck-save
        payload — no card until save.)_
  - [x] Add parse-result ambiguity metadata for meaning checks: candidate
        meanings, selected candidate when available, and parser confidence
        calibrated from eval slices. When confidence is low or unmeasured, show
        "Multiple possible meanings" with per-candidate known/study actions
        instead of asking "Do you know this meaning?" for a guessed sense.
        The single-Meaning-Check vs Multiple-possible-meanings branch must use
        the per-class threshold rule specced in `docs/PARSER_EVAL_METHODOLOGY.md`
        §Ambiguity and meaning-check calibration (selection ≥ 90% AND candidate
        inclusion = 100% AND N ≥ 4 per class → single check; else multi). On the
        v1 slice **no class qualifies yet**, so the honest alpha default is
        Multiple possible meanings for every ambiguous surface until the eval
        slice is expanded and the FI candidate-inclusion gap is closed.
        _(2026-07-04: shipped Multiple possible meanings only; `ambiguous_surfaces`
        metadata carries candidate meanings + source. No confidence field is
        emitted and no single-check UI is built — the single confident Meaning
        Check stays threshold-gated future work.)_
  - [x] Wire "None of these looks right" to parser feedback, not study state.
        This needs the planned flag-only feedback path: nullable proposed
        lemma/POS plus `flag_only=true`. _(2026-07-04: the escape opens the
        correction modal forced to the flag-only path with surface/context
        prefilled, from both the parse-results panel and the review card back.)_
  - [x] Add the Go FSRS dependency and a small scheduling adapter around the
        library. Keep all routing, validation, and deterministic transforms in Go.
        _(2026-07-04: `go-fsrs/v3`; adapter in `internal/store/fsrs.go`.)_
  - [x] Store enough FSRS state in `card_state` (either explicit columns or a
        versioned `fsrs_json` payload) without losing `next_due`,
        `last_answer_at`, and `introduced_at`. _(2026-07-04: versioned
        `{"v":2,"fsrs":{…}}` payload coexists with legacy step payload; timestamps
        preserved.)_
  - [x] Keep explicit known-word evidence separate from FSRS maturity. A mature
        card may influence derived comprehension estimates, but must not
        silently write known-surface state. _(2026-07-04: FSRS state is confined to
        `card_state`; known/ignored evidence is untouched by scheduling.)_
  - [x] Implement `FSRSScheduleForRating(card, rating, now) (next time.Time, newState CardState)`
        behind a feature flag. Keep `nextAlphaStepScheduleForRating` as fallback while the
        migration is in flight. _(2026-07-04: `FSRSScheduleForRating` behind
        `FINNESTDB_FSRS_ENABLED`, default off; step scheduler untouched.)_
  - [x] Migration plan for existing `card_state` rows: `NULL` state becomes a new
        FSRS card; legacy `Step`/`Streak` JSON is converted heuristically from
        `last_answer_at`, `next_due`, and review count if available. Do not
        pretend the old step state can reconstruct true FSRS memory. _(2026-07-04:
        lazy migration-on-first-rating; interval-derived Review seed, no bulk
        rewrite.)_
  - [x] Tests: deterministic schedule tests with fixed `now`, due-queue ordering,
        daily-new-card limit, Again/Hard/Good/Easy API responses, legacy-state
        migration, and rollback/fallback behavior. _(2026-07-04: `fsrs_test.go` +
        `surface_cards_test.go`; existing due-queue/new-limit tests still green.)_
  - [x] Cutover: flip flag on staging DB, validate with seeded review histories,
        then production before public alpha. _(2026-07-04: staging validation
        green — `TestFSRSValidation*` in `internal/store/fsrs_validation_test.go`,
        report at
        [`docs/launch-readiness/2026-07-04-fsrs-validation.md`](docs/launch-readiness/2026-07-04-fsrs-validation.md).
        FSRS is now the default (opt-out flag); `FINNESTDB_FSRS_ENABLED=0` is the
        rollback lever. See DEPLOYMENT.md "FSRS scheduler rollout".)_
  - [x] Honest naming shipped 2026-07-02: the runtime step scheduler is now
        `nextAlphaStepScheduleForRating`, no longer presented as FSRS-shaped.

- [x] **Comprehension prediction per deck** — shipped 2026-07-02. `store.DeckComprehension` computes token-position coverage in SQL (multi-lemma positions covered when ANY candidate is known; ignored lemmas count as covered — decisions recorded in [`docs/srs-deck-spec.md` §Coverage metrics](docs/srs-deck-spec.md)). `GET /api/decks/:id/comprehension` returns coverage + top-10 unlocks with marginal gain; `comprehension_pct` rides the deck list and `/api/me` dashboard summaries. Frontend: deck-list headline, deck-detail projection panel with before→after expansion. Covered by store, handler, and Playwright tests. Cross-deck study ordering remains open below.

- [ ] **Highest-leverage study ordering across decks**. Extend new-card ranking to consider comprehension gain across all study-list decks, not just `token_count` within a single source; user weighting (high/medium/low) for deck priority. Cross-deck variant of marginal gain.

- [ ] **Source-agnostic learning-target correction overlays**. Implement the
  DB-backed model described in [`docs/CORRECTION_TAXONOMY.md`](docs/CORRECTION_TAXONOMY.md):
  learning targets can be lemma, surface, phrase, or proper-name entries, and
  accepted feedback writes to parser-identity, meaning-cue, contextual-sense,
  phrase-boundary, example-quality, or card-presentation overlay rows. This
  must work for pasted text, EPUBs, articles, subtitles, Anki imports, and
  future catalog decks, with Finnish and Estonian correction content kept
  separate. The overlay model must preserve learner/review history while letting
  admins remove faulty current content from circulation: suppress bad
  occurrences/cards from queues, render reviewed replacements, or mark content
  for reparse without pretending old reviews showed different content.

- [x] **EPUB and file upload support**. Server-side extraction lives in `internal/epub` (zip walk + XHTML strip, ported from `corpus_pipeline/cmd/extractcorpus/extract_epub.go`). The inspect and workbench forms now accept `.txt`, `.md`, and `.epub`; `.epub` uploads are POSTed to `POST /api/import/extract` which returns plain text that lands in the textarea so the existing parse → save-deck flow handles books. Plain text continues to be read client-side. Auth-gated, 16 MiB upload cap, 1.5M-char return cap matching the textarea limit. The TODO originally named `POST /api/import/decks`; an extract-only primitive was chosen because the user flow goes through the existing `/api/decks` save path — a one-shot deck-from-file endpoint can be layered on later if needed.

- [ ] **Known-word import polish / external sources**. Current code has
  authenticated `/api/known-words` paste import, `.txt` / `.csv` / `.tsv` /
  `.md` first-column file import, and AnkiConnect local-deck import/sync. It
  does **not** support Anki `.apkg` upload. Remaining work: user-facing CSV/TSV
  guidance, optional robust CSV parsing, `.apkg` front-field extraction, and the
  data-model migration from lemma-only known state toward surface-first known
  vocabulary evidence.

- [x] **Progress dashboard** — shipped 2026-07-02. Dashboard now shows total known lemmas, due count, cards in review (`store.CardsInReview`), reviews today, a 14-day review-activity bar chart, and per-deck comprehension on the recent-decks cards. Backed by a new `review_log` table appended in `RecordReviewAnswer`'s transaction (accumulates from ship date — pre-existing history was never recorded and cannot be backfilled). Remaining follow-up: a *cumulative comprehension over time* chart needs periodic coverage snapshots that don't exist yet; revisit once `review_log` has a few weeks of data to make the panel worth the storage.

- [ ] **Native iOS app for FinnEst (post-go-live)**. After the responsive
  web alpha is shipped and stable, create a native iOS app for FinnEst.
  Treat draft PR [#212](https://github.com/sagarinbabel/finnestdb/pull/212)
  as parked planning input; do not pull this into current go-live scope unless
  explicitly reprioritized.

- [x] **Parse history / deletion UI** so logged-in users can review and delete source context retained by saved decks and parser feedback.

- [x] **Ephemeral Inspect parse behavior** on `/api/parse` so logged-in users get non-persisted parses by default.

### Self-improving feedback loop

Phases 1–4 are live as of 2026-07-02: accepted corrections write authoritative
`custom_overrides` rows (with FEATS), contradicting the frozen gold sets blocks
acceptance, and repeat corrections auto-queue as gold candidates. See
[`docs/FEATURES.md` "User correction loop"](docs/FEATURES.md).

- [x] **Phase 1 — apply accepted lemma/POS corrections** as a `custom_overrides` lexical row. On admin acceptance, write `forms`/`lemmas` rows with `source='custom_overrides'`, `source_priority=1000`, proposed `lemma`/`pos`, and a back-pointer to `parse_feedback.id`.
- [x] **Phase 2 — apply accepted grammar-label corrections to FEATS** — shipped 2026-07-02. The proposed grammar label maps through `udfeats` (`featsFromCaseLabel`) onto the override row's `forms.feats`. Deliberate deviation from the original sketch: corrected FEATS live only on the `custom_overrides` row, never edited into upstream imported rows, so a dictionary re-import can't silently revert or duplicate a correction.
- [x] **Phase 3 — auto-promote to gold candidates** — shipped 2026-07-02. When `store.GoldPromotionThreshold` (3) distinct users have the same correction accepted, it upserts into `gold_candidates`; `make export-gold-candidates` prints pending rows as gold-token JSON for **manual** review into `testdata/parser-eval/*/gold` (auto-committing eval cases would let the system write its own exam).
- [x] **Phase 4 — eval-backed safety check before applying** — shipped 2026-07-02. `make import-gold-surfaces` loads the frozen gold analyses into `gold_surfaces`; acceptance is refused (HTTP 409, full rollback) when ≥2 gold occurrences of the surface unanimously disagree with the proposal. Runs in-transaction in <1ms, so no background job needed. An empty `gold_surfaces` table degrades to no-op — run the importer after clone and after gold changes.
- [x] **Phase 1b — flag-only parser feedback** (public-alpha gate). Nullable proposed lemma/POS plus `flag_only=true`; no lexical writeback until an admin supplies/accepts a concrete correction. Detailed tasks in "Close the self-improving feedback loop" below. — shipped 2026-07-04: `parse_feedback.flag_only` column (idempotent ALTER; proposed columns kept `NOT NULL` and stored empty for flag-only rows), two-path correction modal, admin `flag_only` filter + convert-then-accept path.
- [x] **Phase 1c — correction issues + admin-only quarantine** (public-alpha gate) — shipped 2026-07-04. `correction_issues` table + `parse_feedback.correction_issue_id`; feedback submission groups into an issue by the `(lang, parser, norm_surface, lemma, pos)` fingerprint, recomputes report/distinct-reporter counts, and reopens fixed issues on new reports. Admin **Quarantine now** (required class + reason) suppresses matching content globally from review/new-card queues, deck word/due/new-card counts, and `DeckComprehension` coverage/unlocks; restore is a status flip that preserves `card_state`. `threshold_candidate` badge at ≥3 distinct reporters never auto-quarantines. Combined admin queue gains the issue ledger; no separate Issues page.
- [ ] **Phase 5 (research, not engineering)** — automatic re-ranking of source priorities when a single source consistently produces accepted corrections in one direction. **Stays parked deliberately**: it needs months of accepted-correction volume to have any signal, and per the original scoping it is out of scope for alpha; revisit after Phase 4 has real production data.

Phase 1 is gated on FEATS threading (already shipped via [PR #130](https://github.com/sagarinbabel/finnestdb/pull/130)) so corrections can update FEATS, not just GrammarLabel.

### Observability

- [ ] Add analyzer cache-hit and unknown-lemma counters to complement existing stage-timing stats.
- [ ] Track analyzer cache hit rates in production.
- [ ] Monitor unknown lemma frequency.
- [ ] Create dashboards/alerts for parser health.

### Backend hardening (mostly @chickendude / go-live)

- [x] **Legacy mock-auth/raw-cookie replacement** — current auth uses Argon2id password hashes and DB-backed `session_token` sessions. Remaining go-live auth work is bootstrap retirement, CSRF/Origin posture, and operational controls in [`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md).
- [x] **@chickendude go-live app-level controls**: add rate limiting and CSRF/strict-Origin posture to `POST /api/parse`, `POST /api/parse/feedback`, login, register, and cookie-authenticated state-changing routes before broad public rollout. Deployment-level WAF/monitoring remains in [`docs/GO_LIVE_CHECKLIST.md`](docs/GO_LIVE_CHECKLIST.md).
- [x] Define and implement a retention policy for `parse_sessions.source_text`; current alpha behavior is ephemeral parse by default, with raw source text retained only for saved decks and parser feedback, then purged after 30 days by `make purge-parse-context`.
- [ ] Preserve existing `card_state` scheduling data when rebuilding `cards` during schema migrations instead of dropping and recreating.
- [ ] Batch known/ignored checks during deck creation so card seeding does not do one lookup per unique `(lang, lemma, pos)` pair.
- [ ] Replace `COUNT(*)` existence checks in known-word and parse-feedback paths with `EXISTS`/short-circuit queries.
- [ ] Document parse-session storage behavior directly in the parse UI, not only in docs.
- [x] Add a production startup guard that refuses to serve if `finnestdb.db` is missing, empty, or lacks expected FI/ET dictionary rows unless an explicit dev-only degraded mode is set. Enabled by `APP_ENV=production`; `FINNESTDB_ALLOW_DEGRADED_DB=1` is an explicit development/emergency override.
- [x] **Security review and hardening pass** — completed 2026-07-02, report at [`docs/launch-readiness/2026-07-02-security-review.md`](docs/launch-readiness/2026-07-02-security-review.md). Covers the full FIN-27 scope (auth/sessions, roles, CSRF, XSS sink audit, caps, timeouts, rate limits, data isolation, admin leakage, correction abuse); one fix shipped (web-root deploy excludes dev artifacts), three accepted-for-alpha risks recorded (no CSP, register enumeration, stdout password print). If the external "Codex Security" tool run is still wanted on top, that remains a separate authorization.
- [ ] Either implement real auth/deck/review behavior or narrow the exposed stub endpoints to match current product focus. Remove or isolate non-parser scaffolding that no longer reflects the active roadmap.
- [x] **Operational constraints for parsing — measured 2026-07-02, background queue deferred.** Live measurement against the production-size DB (26.8M FI forms) with real full-novel EPUB texts: `POST /api/decks` with a 550k-char book (70,234 tokens, 10,360 unique words) completed in 1.6s; the largest local book (809k chars) in 2.0s; `POST /api/parse` on the same inputs 1.3–4.0s (cold cache worst case). That is ~0.2–0.6s per 10k tokens — the 30s `WriteTimeout` has ≥7x headroom even cold at the shipped input caps (4 MiB JSON body, 1.5M-char textarea). **Decision: deck creation stays synchronous.** A job queue + "processing" state + polling would add restart-recovery and UX complexity to shave nothing a user can feel. Revisit only if input caps rise materially or production p95 approaches ~10s (watch `parse_duration_ms` in parse stats). Latency expectations are documented in [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).

### Sentence-level features

- [ ] **Sentence generation**: design sentence-level synthesis API; implement agreement rules; validate via re-parsing; test with various feature changes. Either expand the FFI to whole-sentence synthesis (lemma + desired feature change → grammatical sentences) or move generation to Go and call Rust only for token-level inflections. Document agreement, pronoun insertion, enclitic handling.
- [ ] **MWE lexicon schema**: draft schema for `pattern_json`, acceptance thresholds for PMI/LLR, review loop for user-submitted candidates. Consider "seed only" for alpha to bound risk. Draft schema so frontend (highlighting, counts) can be exercised with dummy data.
- [ ] **PR 8 — Track B live quality metrics** (from consumer alpha plan): production metrics from parse usage + accepted corrections. Capture: parse id, user id, language, parser mode, token count, unique lemma count, correction submissions, accepted corrections. Derived: accepted correction rate per 1,000 tokens / per 1,000 unique lemmas, by language and parser mode. Deliver as weekly admin report first; document in `docs/EVAL_AND_CI.md` and `docs/PARSER_FEEDBACK_LOOP.md`. AI may draft triage summaries/classifications for admins, but deterministic code and human approval must own correction routing, quarantine, overlay writes, and parser-identity writeback.

### Documentation

- [ ] Document the expected browser-QA setup more clearly in the repo so Playwright use is obvious on a fresh checkout.
- [ ] Review additional Finnish and Estonian draft cases for promotion after more corpus mining.

### Performance

- [ ] **Bloom filter for compound pre-filtering**. Profile compound splitting on large texts (10k+ tokens) before implementing. Each unresolved form currently triggers up to N×2 SQLite queries for split-point attempts. A Bloom filter over `forms` could eliminate most impossible splits without DB queries. Only implement if profiling shows compound splitting >10% of parse time.

### Suspended / superseded

- ~~**Three-part / recursive compound splitting via `tryCompoundSplit()`**~~ — SUPERSEDED by FST migration. libvoikko VFST handles recursive compounds natively via concatenated `[Xp]...[X]` segments. Do NOT extend `tryCompoundSplit()`. See [`docs/DECISIONS.md`](docs/DECISIONS.md) Decision 5.
- ~~**Finnish consonant gradation rules in `internal/parserules/` or `tryCaseSuffixStrip`**~~ — REJECTED. Gradation belongs in the FST's lexicon-aware paradigm tables (`pkg/lemmatizer-fi-et/`), not in string-rewrite rules over the surface. See [`docs/DECISIONS.md`](docs/DECISIONS.md) Decision 5.

## Open PRs

Snapshot. Refresh by running:

```sh
gh pr list --state open --json number,title,headRefName --jq '.[] | "- #\(.number) `\(.headRefName)` — \(.title)"'
```

Currently open as of 2026-06-01:

- [#212](https://github.com/sagarinbabel/finnestdb/pull/212) `codex/swift-offline-migration-plan` — draft/parked Swift offline migration plan.
- [#211](https://github.com/sagarinbabel/finnestdb/pull/211) `codex/correction-overlay-schema` — correction overlay schema; uniqueness-index feedback has been addressed on the PR branch and is awaiting review/CI.

## Research Goals

_Added 2026-05-07._

Larger investigations recorded here so they don't get lost between PRs.
These are not "must-do this quarter"; they are explicit research bets we
want to pursue alongside execution work.

### Discover the most-frequent inflected forms in user-pasted text

**What.** As users paste text, aggregate per-form counts into a running
tally per language. Periodically recompute and publish a ranked top-N
list of **inflected surface forms** (not lemmas) for Finnish and Estonian.

**Why this is novel (working hypothesis).** Public Finnish/Estonian
frequency lists either rank lemmas (wrong unit for a learner reading
running text) or rank forms on a fixed corpus (subtitle, news, Wikipedia)
that may not reflect what real learners want to read. The aggregated
user-pasted corpus reflects real reader interest. The "novel" claim is a
hypothesis until checked against public baselines under
[`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md).

**Tasks.**

- [ ] Schema for per-language inflected-form counts; document
      retention/anonymization policy
- [ ] Aggregation job — online (per-parse increment) or offline batch
- [ ] Ranked top-N publication: UI surface + downloadable artifact
- [ ] Comparison against the public baselines in [`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md)
      to validate the "differs from existing lists" claim
- [ ] Surface comprehension-coverage curves to learners
      ("learn N forms → understand X% of running text"), linking to
      `Comprehension prediction per deck` below

**Cross-references.**

- Rationale, ML angle, and constraints: [`docs/ML_IDEAS.md` §2b](docs/ML_IDEAS.md)
- Public baseline lists used for comparison: [`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md)
- Comprehension prediction (already tracked as §13 below) is the
  natural consumer of this ranking; both should ship together.

### Re-test FI vs ET top-N coverage with corpus-size-comparable data

**Added 2026-05-07.**

**What.** Confirm or refute the ~3–7pp Estonian-vs-Finnish coverage
advantage observed in our 2026-05-07 baseline measurements (see
[`docs/CROSS_LANGUAGE_STRATEGY.md` "Measurable Divergences"](docs/CROSS_LANGUAGE_STRATEGY.md))
using corpora that are size-matched and register-matched between the two
languages.

**Why.** The current measurement is mixed evidence:

- In OpenSubtitles 2018, the ET corpus is roughly half the size of the
  FI corpus, so smaller-corpus effect (shorter long tail) could
  partially inflate ET's top-N coverage.
- In UD treebanks, the ET treebank is roughly 2× the FI treebank — the
  opposite of the inflation direction — and ET still has a slight
  coverage advantage. So the gap is at least partly real.

A size-matched, register-matched re-measurement would let us state with
high confidence whether the gap reflects a genuine morphological
property (ET being marginally less inflectionally rich than FI) or
sampling artifacts.

**Tasks.**

- [ ] Identify or sub-sample one comparable corpus per language at the
      same register (e.g. matched-size Yle FI news + ERR ET news,
      matched-size Project Gutenberg FI + DEA ET, etc.)
- [ ] Compute and compare top-N coverage curves with bootstrap CIs
- [ ] Report the result in `docs/CROSS_LANGUAGE_STRATEGY.md` and
      `docs/FREQUENCY_BASELINES.md`

**Cross-references.**

- Initial measurement: `docs/FREQUENCY_BASELINES.md`
- Theoretical context (register variation in Zipfian distributions —
  well-studied; this work is empirical confirmation on FI/ET): see
  citations in CROSS_LANGUAGE_STRATEGY.md once added.

### Close the self-improving feedback loop (accepted corrections → lexical updates)

**Added 2026-05-07.**

**What.** A logged-in user can submit a parse correction
(`POST /api/parse/feedback`, [internal/api/handlers.go](internal/api/handlers.go))
and an admin can change its status to `accepted`
([internal/store/db.go::ReviewParseFeedback](internal/store/db.go)).
Accepted lemma/POS corrections now write `custom_overrides` lexical rows
that can change future parser output. FEATS corrections, gold-case
promotion, and eval-gated safety checks shipped 2026-07-02 — see the
"Self-improving feedback loop" summary above for the as-built behavior.
Phases 1b and 1c below are the remaining public-alpha work in this section.

**Why.** The correction-feedback moat is one of the project's core
differentiators (see
[`docs/FEATURES.md` "User correction loop"](docs/FEATURES.md)). Every
accepted correction that doesn't update the lexicon is a learner who fixed
something for the next learner — and didn't.

**Tasks (sequenced).**

- [x] **Phase 1 — apply accepted lemma/POS corrections as a `custom_overrides`
      lexical row.** Schema: a new source `custom_overrides` with the
      highest priority (1000), as described in
      [`docs/LEXICAL_PLAN.md`](docs/LEXICAL_PLAN.md) "Resolution Layer".
      On admin acceptance, write rows to `forms` and `lemmas` with
      `source='custom_overrides'`, `source_priority=1000`, the proposed
      `lemma`/`pos`, and a back-pointer to `parse_feedback.id`.
- [x] **Phase 1b — add flag-only parser feedback before alpha ambiguity UI.**
      Allow signed-in learners to submit "this analysis looks wrong" without
      proposing lemma/POS. Add nullable proposed fields when `flag_only=true`,
      expose the flag through API/store/admin UI, add an admin filter, and keep
      lexical writeback limited to accepted concrete parser-identity
      corrections. — shipped 2026-07-04. `parse_feedback.flag_only INTEGER NOT
      NULL DEFAULT 0` via idempotent ALTER; `proposed_lemma`/`proposed_pos` kept
      `NOT NULL` (a table rebuild to relax them was disproportionate) and stored
      empty for flag-only rows, with validation enforcing non-empty only when
      `flag_only=0`. Two-path correction modal (flag-only default vs propose
      fix), admin `flag_only` list filter + badge, and a convert-then-accept
      path where an admin-supplied lemma/POS turns a flag-only row into a normal
      correction that flows through the existing eval-gated writeback.
      Flag-only acceptance alone writes nothing to `custom_overrides`,
      `gold_candidates`, or the gold guard.
- [x] **Phase 1c — quarantine or replace existing faulty study content
      (public-alpha gate).** — shipped 2026-07-04. Built as admin-only global
      quarantine of scoped `correction_issues` (no overlay/replace layer yet;
      that stays future overlay work). Grouping keys on
      `(lang, parser, norm_surface, lemma, pos)`; quarantine matches by
      `(lang, lemma, pos)` when present else `(lang, normalized surface)` and
      suppresses review/new-card queues, deck word/due/new-card counts, and
      `DeckComprehension`. Restore preserves `card_state`. `threshold_candidate`
      badge only, never auto-quarantine. Original spec below.
      Accepted feedback should be able to stop a known-bad card/occurrence from
      appearing in review/new-card queues for all matching learners, or render
      an accepted overlay for the cue/sentence/explanation/sense. Do not delete
      or rewrite historical review events or scheduler provenance. Keep the
      alpha schema small: `parse_feedback` stays raw intake; add a
      `correction_issues` global state table plus `parse_feedback.correction_issue_id`.
      The issue row tracks scope, status, duplicate/distinct-reporter counts,
      quarantine/fix fields, and reopened/regression markers. Default behavior:
      reports create/update an issue but do not globally hide content until
      admin confirmation. Add an emergency admin "quarantine now" action with
      required reason, explicit scope, event logging, and a rollback/fix path.
      Defer separate quarantine-target and rich event tables unless needed for
      traceability. Collect threshold-candidate badges for future automation, but
      do not auto-quarantine from thresholds in alpha.
      Current learner-facing stats must exclude quarantined content: deck word
      counts, due counts, new-card counts, comprehension estimates, and
      next-unlock projections. Historical/admin audit views can include it.
      Restoring a fixed item should preserve review/FSRS scheduler state when
      learning target identity is unchanged; reset scheduling only for a new
      target identity.
      Admin triage requires a simple alpha class (`parser issue`, `bad card
      content`, `source/extraction issue`, `not sure`); full taxonomy labels are
      optional and should not block quarantine/fix.
      Alpha admin UI supports classification, notes, report grouping, duplicate
      counts, and "quarantine now"; it should not include a broad in-app fix
      editor. Parser-identity fixes can keep using accepted lemma/POS
      `custom_overrides`; richer fixes go through manual code/data changes or
      future overlay work.
      Keep one combined admin feedback/issues queue for alpha with filters such
      as `submitted`, `needs review`, `quarantined`, `fixed`, and `reopened`;
      do not build a separate Issues page unless real volume demands it.
- [x] **Phase 2 — apply accepted grammar-label corrections to `forms.feats`**
      — shipped 2026-07-02, with a deliberate deviation from this sketch:
      corrected FEATS live only on the `custom_overrides` row, never edited
      into upstream imported rows. Original sketch: for the specific surface form. Smaller blast radius than full lemma
      rewrites; useful for the 0%-grammar-on-some-datasets gap. As of
      `2026.05.07k` the `forms.feats` column is populated by the import
      pipelines themselves (`cmd/importdict/feats.go::kaikkiTagsToFeats`,
      `cmd/importekilexdetails/feats.go::ekilexMorphToFeats`), so a
      correction PR can update an existing row's FEATS instead of
      writing a parallel `custom_overrides` row in many cases.
- [x] **Phase 3 — auto-promote a corrected `(surface, lemma, pos)` tuple
      to a gold-eval case** — shipped 2026-07-02 with N=3 distinct users
      (`store.GoldPromotionThreshold`) and manual promotion via
      `make export-gold-candidates`. Original sketch: threshold and review
      workflow TBD.
- [x] **Phase 4 — eval-backed safety check before applying.** — shipped
      2026-07-02 as the `gold_surfaces` contradiction check: acceptance is
      refused (HTTP 409, full rollback) when >=2 gold occurrences unanimously
      contradict the proposal. `cmd/autoresearch` remains parked.
- [ ] **Phase 5 (long-tail) — automatic re-ranking of source priorities**
      when a single source consistently produces accepted corrections in
      one direction. Out of scope for alpha; revisit after Phase 4 is
      stable.

**Cross-references.**

- Schema groundwork: source priority columns + `dict_metadata` already
  exist; the `custom_overrides` source is named in
  [`docs/LEXICAL_PLAN.md` "Resolution Layer"](docs/LEXICAL_PLAN.md).
- `cmd/autoresearch` is parked as a post-live idea. It may be useful later as
  inspiration, but it is not part of the alpha implementation scope.
- This work is gated on the FEATS threading PR (already shipped via
  [PR #130](https://github.com/sagarinbabel/finnestdb/pull/130)) so
  corrections can update FEATS, not just GrammarLabel.

**Confidence.** Phase 1 is a 1-week task with high confidence. Phase 2
is a few-day task. Phase 3 needs design before scoping. Phase 4 needs
a measured eval-time budget; if it adds >100ms to admin-accept latency,
push it to a background job. Phase 5 is research, not engineering.

## Notes & historical

### Critical Findings (PRD review, 2026-04-29)

These four findings came out of the 2026-04-29 PRD review. Most are
addressed or actively tracked in "What's not in main yet" above; this
section is preserved for traceability.

#### 1. Synchronous Deck Creation Blocking Issue

**Problem.** Synchronous deck creation assumed the entire 2 MB upload
is parsed in-request (§3.2) while running the full Rust pipeline
(steps 1–7) and even MWE discovery (§5.1). No latency/error budget,
timeout story, or fallbacks if Omorfi/Vabamorf hiccups.

**References.** `finnestdb-prd-alpha.md:73-137`.

**Status.** Tracked under "Backend hardening" → operational constraints
for parsing + background job system. Parser observability shipped
(stage timings); analyzer cache hits / unknown-lemma counters tracked
under "Observability".

#### 2. Disambiguation Model Specification Missing

**Problem.** Spec named disambiguation techniques (Viterbi over UD
tags, lemma frequency priors) but never stated where the training
data/model lives, how it's versioned, or how to evaluate "good enough."

**References.** `finnestdb-prd-alpha.md:125-137,295-303`.

**Status.** Tracked under "Parser quality" → disambiguation model.

#### 3. MWE Handling Underspecified

**Problem.** MWE described as "seed lexicon + PMI/LLR + DP segmentation"
but lexicon format, scoring thresholds, governance not defined.

**References.** `finnestdb-prd-alpha.md:133-166,300-314`.

**Status.** Tracked under "Sentence-level features" → MWE lexicon schema.

#### 4. Example Generation FFI Contract Incomplete

**Problem.** Example generation relies on "FST synthesizer + reparse
to validate features" (§4.3) yet the FFI only exposes `inflect` per
token. No sentence-level agreement (subject pronouns, enclitic
placement) or grammatical filler word assembly.

**References.** `finnestdb-prd-alpha.md:114-159`.

**Status.** Tracked under "Sentence-level features" → sentence generation.

### Consumer alpha execution plan (2026-04-29)

**This was the locked execution plan when the alpha was scoped. Most of
PR 1–6 has shipped (auth roles, frontend surface split, known
words/global cards, parse feedback subsystem, deck CRUD + review). PR 7
(ET evaluation parity) is partially shipped via Plan C / PRs
[#113](https://github.com/sagarinbabel/finnestdb/pull/113)/[#114](https://github.com/sagarinbabel/finnestdb/pull/114)/[#115](https://github.com/sagarinbabel/finnestdb/pull/115)
+ FST migration. PR 8 (Track B live quality metrics) and PR 9 (security
review) remain open. The full plan is preserved here for traceability
and is not actively re-litigated. See "What's not in main yet" above
for up-to-date open work.**

5. **Migrate alpha review identity and scheduler before public alpha** _(added 2026-05-07; narrowed 2026-07-03; implemented behind a default-off flag 2026-07-04)_

   > **Pointer (2026-07-04):** Surface-form card identity and narrow FSRS are
   > implemented — see the live "Migrate alpha review identity and scheduler
   > before public alpha" item under "What's not in main yet" for the ticked
   > subtasks. FSRS ships behind `FINNESTDB_FSRS_ENABLED` (default off); the only
   > remaining step is the staged flag cutover.

   `internal/store/db.go::nextAlphaStepScheduleForRating` is a hand-rolled step scheduler with hardcoded day arrays `{1,3,7,14,30,60}` (good) / `{3,7,14,30,60,90}` (easy). `again` is 10 minutes; `hard` is 8 hours. This is **not** FSRS; `docs/srs-deck-spec.md §13–24` already recommends [`go-fsrs`](https://github.com/open-spaced-repetition/go-fsrs); Decision 23 (2026-07-03) moved narrow FSRS onto the public-alpha launch path.

   - [ ] Ship the narrow alpha version: Go FSRS runtime scheduling, default parameters, current four-button UI, feature flag, migration/fallback, and regression tests.
   - [ ] Explicitly defer full FSRS ecosystem work: personal parameter optimization, `fsrs-rs`, rescheduling tools, simulation dashboards, mature-card analytics, and broad review UX redesign.
   - [x] Alpha card identity is settled: public alpha review cards are surface-form-in-context cards, not long-lived lemma/POS cards.
   - [ ] Migrate review card identity to stable surface-form cards before attaching real FSRS state. Keep lemma/POS/dictionary entries as derived support.
   - [ ] Split identical-looking surfaces into sense-aware cards when parser/dictionary evidence supports distinct meanings, and show context plus homograph guidance on the card.
   - [ ] Resolve ambiguous imported known surfaces lazily in context. Meaning-check UI must make clear whether "Study this meaning" creates/keeps a review card now or creates one when the deck is saved.
   - [ ] In parse results, meaning-check UI must be pending until deck save: "Study this meaning" means the card is created when the learner saves/adds the deck.
   - [ ] Add parse-result candidate/confidence metadata and branch UI calibrated by eval slices: high confidence uses a single meaning check; low/unmeasured confidence shows "Multiple possible meanings" with per-candidate known/study actions.
   - [ ] Wire "None of these looks right" to flag-only parser feedback (`flag_only=true`, nullable proposed lemma/POS), not to known/study state. Flag-only reports are triage signal and must not write `custom_overrides` until an admin supplies/accepts a concrete parser-identity correction.
   - [ ] Add the Go FSRS dependency. Plan the schema delta on `card_state` (FSRS needs stability, difficulty, last review, last rating, retrievability — multiple fields the current schema doesn't carry unless encoded in a versioned `fsrs_json`).
   - [ ] Keep explicit known-word evidence separate from FSRS maturity. Mature cards can feed derived retained/learning coverage views, but they must not silently mark surfaces known.
   - [ ] Implement `FSRSScheduleForRating(card, rating, now) (next time.Time, newState CardState)` behind a feature flag. Keep `nextAlphaStepScheduleForRating` as fallback while the migration is in flight.
   - [ ] Migration plan for existing `card_state` rows: convert `NULL` state to a new FSRS card and derive a conservative starter FSRS state from legacy `Step`/`Streak`, `last_answer_at`, and `next_due` where possible. Do not overclaim precision from the old step scheduler.
   - [ ] Cutover: flip the feature flag on a staging DB, validate seeded histories and due queues, then cut over production before public alpha.
   - [x] Honest naming shipped 2026-07-02: runtime scheduler renamed to `nextAlphaStepScheduleForRating`.

6. **Disambiguation model**
   - [ ] Select UD treebanks (Finnish, Estonian)
   - [ ] Train initial POS tagging model
   - [ ] Establish evaluation metrics and baseline
   - [ ] Version model artifacts

7. **Server surface cleanup**
   - [ ] Either implement real auth/deck/review behavior or narrow the exposed stub endpoints so the server surface matches the parser-workbench product focus
   - [ ] Remove or isolate non-parser product scaffolding that no longer reflects the active roadmap

8. **Custom dictionary knowledge graph spike**
   - [ ] Spike a separate custom lexicon for Finnish and Estonian that can accumulate data from multiple upstream dictionaries plus manual edits
   - [ ] Michael: run the full Ekilex `word/details` enrichment scrape when you have time and enough disk space. Sagar does not currently have space for this locally. The lightweight Ekilex import in `localdata/ekilex/eki-public-words-2026-et.jsonl` only uses `/api/public_word/eki`; the richer endpoint has POS, definitions, usage examples, and paradigms, but sample payloads were about 492 KB for `koer` and 770 KB for `maja`. Fetching details for all 174,229 public Estonian headwords would be roughly 87+ GB of raw JSON and 174k HTTP requests, likely many hours before retries or rate limits. Please run it as a resumable batch job with checkpointing by `word_id`, conservative rate limiting, retry/backoff, raw responses in ignored `localdata/`, and a compact reduced JSONL artifact for review/commit after validation.
   - [ ] Design provenance tables so accepted fields (definition, examples, morphology, register) retain source attribution and fetch/import metadata
   - [ ] Design a compiled read model so hot-path lookups remain indexed and near direct-lookup cost, with no live provenance merge in request handling
   - [ ] Define a slower live-merge/admin view for curation, debugging, and experimenting with merge rules outside the request path
   - [ ] Define how fallback lookups append new source facts and trigger per-entry recompilation rather than full-database rebuilds
   - [ ] Define manual injection flows for curated edits, CSV/JSONL imports, and precedence rules between manual facts and auto-imported facts

9. **Background job system**
   - [ ] Design async processing architecture for deck creation
   - [ ] Implement job queue (in-memory or external)
   - [ ] Add "processing" state to deck model
   - [ ] Create webhook/polling mechanism for status updates

10. **Sentence generation**
   - [ ] Design sentence-level synthesis API
   - [ ] Implement agreement rules
   - [ ] Add validation via re-parsing
   - [ ] Test with various feature changes

11. **EPUB and file upload support**
   - [x] Add EPUB text extraction to the import pipeline (parse XHTML content documents, strip markup, concatenate chapter text) — `internal/epub/extract.go`
   - [x] Accept file upload alongside raw text — `POST /api/import/extract` returns extracted text; inspect/workbench forms route `.epub` through it
   - [x] Support plain-text (.txt) and EPUB (.epub) as initial formats — `.txt`/`.md` read client-side, `.epub` via server endpoint
   - Surasura already does EPUB extraction for Japanese/Chinese; same approach applies to Finnish/Estonian content
   - Lowers friction for book-based learners who currently have to paste text manually

12. **External vocabulary import (Anki, CSV)**
   - [x] Support paste import through `/api/known-words`.
   - [x] Support `.txt`, `.csv`, `.tsv`, and `.md` import by reading one word per line or the first column.
   - [x] Support AnkiConnect import/sync from a local running Anki desktop collection, including deck selection, field selection, source tagging, and preserve-manual sync scope.
   - [ ] Add clearer CSV/TSV guidance for learners exporting custom vocabulary lists.
   - [ ] Decide whether first-column parsing is enough or whether quoted CSV needs a real parser.
   - [ ] Support Anki `.apkg` upload as a separate offline path.
   - [ ] Move known-word modeling toward surface-first storage. Today imports submit surface strings but persist resolved `(lemma, pos)` rows in `user_known_lemmas`; the product target is to preserve the exact surface forms the learner says they know, with lemma/POS resolution as derived evidence.
   - Surasura imports known vocabulary from Anki, Migaku, and Jiten.moe to bootstrap the user's known-word state; same idea applies here so coverage metrics and new-card selection are useful from day one.

13. **Comprehension prediction per deck**
   - [ ] Add a "predicted comprehension %" display to deck detail views using token-weighted coverage
   - [ ] Show before/after projection: "if you learn the top N words from this deck, your comprehension goes from X% to Y%"
   - [ ] Compute marginal comprehension gain per word to drive study ordering
   - Token-weighted coverage (`srs-deck-spec.md §Coverage metrics`) already defines the formula; this item is about surfacing it as a prominent UI feature
   - Surasura's core UX centers on showing comprehension percentages before and after consuming media

   **Sequencing and parallelization _(added 2026-05-07)_.**

   This work is **parallel-safe with the FEATS-threading PR and the
   CRF-disambiguator track** (see `docs/ML_IDEAS.md §1a`). It touches
   `web/app.ts`, a small handful of new API endpoints, and a couple of
   read-side store helpers. None of those overlap with the parser hot
   path. The math is small; the open questions are product design.

   **Formulas (verified against [`docs/srs-deck-spec.md` §Coverage
   metrics](docs/srs-deck-spec.md)).**

   ```
   personal_coverage(text, user_known_set) =
       Σ token_count[t] for t in tokens(text) where t.lemma ∈ user_known_set
       ÷ Σ token_count[t] for t in tokens(text)

   marginal_gain(text, user_known_set, candidate_lemma) =
       Σ token_count[t] for t in tokens(text) where t.lemma == candidate_lemma
       ÷ Σ token_count[t] for t in tokens(text)
   ```

   Both are O(N) over the deck's `occurrence` rows once a known-lemma
   set lookup is indexed. No new tables required. The deck's
   per-`(lemma, pos)` token counts are already materialized at deck
   creation time (`internal/store/db.go::CreateDeck` expands tokens
   into `occurrence` rows).

   **Backend tasks.**

   - [ ] Read-side helper: `store.DeckLemmaStats(deckID) []LemmaCount`
         returning `(lemma, pos, token_count)` rows from `occurrence`,
         sorted by `token_count` desc. Cache invalidation: invalidate
         on deck content change (rare).
   - [ ] Read-side helper: `store.UserKnownLemmaSet(userID, lang) map[LemmaKey]bool`
         (this likely already exists in some form for known-words
         filtering during deck creation; verify and reuse if so).
   - [ ] New endpoint: `GET /api/decks/:id/comprehension`
         - returns `{ coverage_pct: float, total_tokens: int, known_tokens: int, top_unlocks: [{lemma, pos, gain_pct, token_count}, ...] }`
         - `top_unlocks` is the top N (default 10) candidate lemmas
           ranked by `marginal_gain` for the current user.
   - [ ] Extend `GET /api/decks/:id` response to include the headline
         `comprehension_pct` so the deck list / dashboard can render
         it without an extra round trip per deck.

   **Frontend tasks.**

   - [ ] Deck-detail page: add a "Predicted comprehension" badge near
         the deck title showing `X% / 100%`. Click → expand to the
         marginal-gain projection table.
   - [ ] Deck list / dashboard: add the comprehension column.
   - [ ] Marginal-gain projection: "Learn these N words to reach Y%
         comprehension." Show the next 10 unlocks; let the user "mark
         as known" inline.

   **Open product-design questions (decide before frontend work).**

   - [ ] Where does this surface live: deck detail page, parse results
         page, or both? Recommendation: deck detail first (highest
         signal-to-noise), then optionally on parse results once the
         shape is settled.
   - [ ] How does coverage interact with the user's *ignored* lemma
         set? Treat ignored as known (so "I don't care about proper
         names" raises my coverage), or as separate? `srs-deck-spec.md`
         doesn't take a position; needs a call.
   - [ ] Form-level vs lemma-level coverage display. Most users will
         want lemma-level ("I know the word"); advanced learners may
         want form-level ("I know this exact inflected form"). Pick
         lemma-level for v1; expose form-level as a toggle later.

   **Suggested form to start in parallel.** Sketch the UI layout
   (Figma or a hand drawing is fine), write the backend endpoint
   stubs against an in-memory test deck, ship the deck-detail badge
   first as the smallest meaningful slice. The marginal-gain
   projection can be a separate PR. Confidence: high that this
   shipping order minimizes blast radius.

   **Eval / sanity check.** Before shipping, sanity-check the
   coverage numbers against the public baselines under
   [`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md) — if a user with the top-1000
   FI inflected forms as their known set sees `personal_coverage`
   far from 65% on subtitle-style decks or far from 40% on written
   decks, something is wrong. The expected band is the calibration
   data we already collected.

   **Cross-references.**

   - Formulas: [`docs/srs-deck-spec.md` §Coverage metrics](docs/srs-deck-spec.md)
   - Calibration baselines:
     [`docs/FREQUENCY_BASELINES.md`](docs/FREQUENCY_BASELINES.md) and
     [`docs/CROSS_LANGUAGE_STRATEGY.md` "Measurable Divergences"](docs/CROSS_LANGUAGE_STRATEGY.md)
   - User-text-aggregated frequency feeds the same machinery —
     see Research Goals above.
   - Cross-deck variant of marginal gain — see §6 below.

14. **Highest-leverage study ordering across decks**
    - [ ] Extend new-card ranking to consider comprehension gain across all study-list decks, not just token_count within a single source
    - [ ] Rank candidate words by: "how many tokens across all active decks does learning this lemma unlock?"
    - [ ] Allow the user to weight decks by priority (high/medium/low) so words in high-priority content are preferred
    - Current ranking (`srs-deck-spec.md §New card selection`) sorts by token_count within the selected source; cross-deck optimization would be a meaningful upgrade
    - Surasura generates "highest-leverage order" study sequences by analyzing frequency across a user's entire content library

15. **Progress dashboard**
    - [ ] Implement the dashboard tab with learning progress visualization over time
    - [ ] Show: total known vocabulary, cards in review, comprehension trend per deck, daily review count
    - [ ] Add a cumulative comprehension chart: how does total coverage change as the user learns more words?
    - The frontend already has a dashboard tab placeholder; this is about filling it with meaningful data
    - Surasura has an interactive HTML dashboard with progress tracking that users find motivating

16. **Observability**
    - [x] Add timing instrumentation to parser steps
    - [ ] Track analyzer cache hit rates
    - [ ] Monitor unknown lemma frequency
    - [ ] Create dashboards/alerts for parser health

17. **Three-part compound splitting** — ~~SUPERSEDED by FST migration~~
    - Recursive compounds are handled natively by libvoikko VFST via concatenated `[Xp]...[X]` segments; see [PR #107](https://github.com/sagarinbabel/finnestdb/pull/107) (FI) and the planned ET equivalent.
    - Do NOT extend `tryCompoundSplit()` — see `docs/DECISIONS.md` Decision 5.

18. **Consonant gradation rules** — ~~REJECTED~~
    - Gradation does not belong in `internal/parserules/` or
      `internal/store/dict.go::tryCaseSuffixStrip`. It belongs in the FST's
      lexicon-aware paradigm tables (`pkg/lemmatizer-fi-et/`).
    - Adding strong↔weak grade pairs to a string-rewrite path produces
      false positives at lemma boundaries and double-counts cases the FST
      already handles. See `docs/DECISIONS.md` Decision 5.

19. **Bloom filter for compound pre-filtering**
    - [ ] Profile compound splitting performance on large texts (10k+ tokens) before implementing
    - Currently each unresolved form triggers up to N×2 SQLite queries for split-point attempts
    - A Bloom filter over the forms table could eliminate most impossible splits without DB queries
    - Only implement if profiling shows compound splitting as a bottleneck (>10% of parse time)

### Consumer flow review (2026-05-07)

Companion docs:

- [`docs/USER_FLOWS.md`](docs/USER_FLOWS.md) — screen-by-screen consumer alpha spec with wireframes and the recommended correction-flow design
- [`docs/DESIGN_AI_PROMPTS.md`](docs/DESIGN_AI_PROMPTS.md) — prompt templates for v0 / Lovable / Bolt / Cursor that respect the existing token system
- [`experiments/2026-05-07-top-1000-inflected-forms.md`](experiments/2026-05-07-top-1000-inflected-forms.md) — research plan for the cold-start seed deck

New work surfaced by the review (not yet broken into sequenced PRs):

- [x] **Public alpha posture: anonymous parser demo plus signed-in learning.**
  Anonymous visitors can paste, parse, get a word list, and explore it. Durable
  and personalized features require sign-in: save/deck/review, known/ignored
  state, imports, parser feedback, history, and account settings. See
  `docs/USER_FLOWS.md` §1.
- [x] **Anonymous parse has a stricter text-size limit.** Signed-in parsing
  keeps the 1,500,000-character cap; anonymous demo parsing uses a lower
  configurable cap. Shipped 2026-07-04 as `FINNESTDB_ANON_MAX_CHARS` (default
  20,000 at ship, raised to 300,000 the same day), enforced server-side before
  expensive parser work, returning a 4xx
  that names the limit and prompts sign-up for longer texts.
- [x] **Public alpha language status: Finnish and Estonian are equal.** Do not
  label either language experimental or secondary. If parity gaps exist, track
  them as concrete data/parser/catalog/UX/test gaps and classify them as
  alpha-blocking, acceptable language-specific differences, or post-alpha
  improvements.
- [x] **Public alpha go/no-go standard.** Launch with working core journeys and
  known non-dangerous rough edges only. The exact rough-edge rubric lives in
  `docs/GO_LIVE_CHECKLIST.md`; no-go categories include privacy/security,
  retention/account deletion, data integrity, review state, parser
  feedback/quarantine, misleading parser-confidence UI, overload behavior, and
  FI/ET equal-status failures.
- [x] **Public alpha access: open signup.** Hosted alpha should allow
  self-serve account creation, not invite-only or waitlist-first access. Because
  signup is open, abuse controls, rate limits, account deletion, retention,
  admin visibility, auth hardening, and basic monitoring are launch gates rather
  than post-alpha polish.
- [x] **Email verification should not block first value.** New users can parse,
  save a deck, and start review immediately after signup. Verification can gate
  high-volume parsing, repeated feedback, exports if enabled, account recovery,
  and trust-weighted correction signals.
- [ ] **Plan hosted alpha for 1,000 concurrent users.** Add load-test targets,
  parser concurrency/backpressure controls, overload behavior, and monitoring.
  Graceful degradation should throttle anonymous/oversized parses first and
  keep the signed-in review/deck loop alive as long as possible.

  **Progress 2026-07-04**: see the matching entry under "1,000-concurrent-user
  launch target" above and
  [`docs/launch-readiness/2026-07-04-load-test.md`](docs/launch-readiness/2026-07-04-load-test.md)
  — semaphore, anonymous-sheds-first, `cmd/loadtest`, and local laptop
  evidence shipped; production-host re-run and monitoring wiring remain.
- [ ] **Curated embedded catalog for signed-in cold start.**
      Current state (2026-07-04): the mechanism is built and shipped — embedded
      metadata + lazy full text (`internal/catalog`, `go:embed`), a
      deterministic generator (`cmd/gencatalog`), signed-in `GET /api/catalog`
      (with per-learner known-token coverage) and `GET /api/catalog/{id}/text`,
      and dashboard + Inspect cold-start pickers that load a chosen text into
      the Inspect textarea for the normal parse→deck flow. Initial texts are
      honest, not the full matrix: 3 FI + 3 ET across ≥2 genres and ≥2 difficulty
      buckets each, all license-clean (FI: Project Gutenberg public domain +
      original CC0; ET: original CC0 because ET Gutenberg was effectively
      unavailable). Remaining: the full 36-text matrix and human difficulty
      sanity-check (`difficulty_review` is `"pending"` on every entry). The rest
      of this item is the still-open target.
      Dashboard and
      Inspect empty states should offer both "paste/upload your own text" and
      FI/ET texts from the redistributable subset of the corpus. Start with a
      hand-curated catalog generated from local corpus tooling: FI and ET;
      stories, articles, poems; Easy, Medium, Hard; two texts per bucket
      (36 texts when complete). Prefer full coherent texts when license and
      size allow it; use preview excerpts only for UI display. Generate fixed
      global difficulty from text-level metrics, then sanity-check Finnish with
      Sagar and Estonian with an Estonian reviewer. Ship checked-in metadata
      plus checked-in text fixtures, and lazy-load full text only when selected.
      If the learner has known-word data, show personalized known-token coverage
      or "fit for you" signals in addition to global difficulty. Current code
      computes from lemma-backed known state; the target model should preserve
      known surface forms as first-class evidence. If no known-word data exists,
      prompt import. Track source URL, corpus source, title, author when known,
      language, license/reuse basis, text length, import date, and attribution.
      Do not use local corpus material whose manifest or source ledger says it
      is non-redistributable.
- [ ] **Live stats strip under the textarea**. Detected language, char count, token count, unique-form count, number count — debounced. Drives the language-mismatch banner. See `docs/USER_FLOWS.md` §1.
- [ ] **Anki .apkg upload**. Front-field extraction client-side, routed through
  known-word import. This is separate from the existing AnkiConnect import/sync
  path and from Inspect `.txt` / `.md` / `.epub` uploads.
- [ ] **Carry-forward of anonymous parses on sign-up**. Last-N parses held in **`sessionStorage`** (tab-scoped — `localStorage` would survive browser restarts and break the anonymous-is-ephemeral promise), POSTed and persisted after account creation so the user doesn't lose what they just did. Cross-restart survival, if we ever ship it, must be an explicit opt-in checkbox.
- [ ] **Google OAuth**. Adds `auth_provider`, `auth_provider_uid` columns; `password_hash` becomes nullable for OAuth accounts. Verify the Google ID token and copy/require its `email_verified` claim rather than assuming every returned email is verified. Email+password path stays the default. See `docs/USER_FLOWS.md` §3.
- [ ] **`first_name` on the user profile**. Required at signup; used for greeting copy on the dashboard.
- [ ] **"Add to existing deck" save path**. Results-page save panel gains a radio for new-deck vs. add-to-existing; merge by `(lemma, pos)` with `deck_lemma_stats` accumulation. New verb on the deck-import API. See `docs/USER_FLOWS.md` §6.
- [x] **Ephemeral Inspect parses by default**. `/api/parse` does not write `parse_sessions`; source context is retained only when the user saves a deck or submits parser feedback.
- [x] **Parse-history UI**. Logged-in users can list retained parse sessions and delete one or all retained sessions server-side.
- [ ] **Correction flow lighter entry point**. Replace the per-row correction button with a hover/focus-revealed `✎ Wrong?` link. Add a "flag-only" radio path so users who notice a wrong parse but don't know the right answer can still submit signal. Backend: `parse_feedback.proposed_lemma`/`proposed_pos` become nullable; add `flag_only` boolean. See `docs/USER_FLOWS.md` §10.
- [ ] **Sentence translation endpoint**. `POST /api/translate-sentence` backed by Sonnet 4.6 with prompt caching. Persist results in a new `sentence_translations` table only for retained parse/deck content, keyed on source/target language + prompt version + `hash(text)`; ephemeral Inspect parses use no shared persistent cache. Wires into the review-card back and the deck-detail rows. Companion to `docs/ideas.md` "Making it AI native" Phase 1.
- [x] **Cold-start "Top 1000" CTA** — shipped 2026-07-02 as an *official deck* rather than a per-user route: `cmd/seedcolddeck` builds a "Top 1000 words" official deck per language from the public OpenSubtitles baseline (forms resolved to lemmas via the dictionary, ranked by summed token mass across inflections, proper names filtered; verified end-to-end against the full local DB). Users add it through the existing official-decks surface; the dashboard/decks empty states link there. Operator step documented in [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md). The original "seed from the user-pasted-text research ranking" idea remains open under Research Goals — when that ranking ships, reseed from it.
- [ ] **Cold-start milestones + individual test-out (grill Q15/Q16 follow-up).**
  The shipped Top-1000 official deck is the alpha cold-start mechanism, but the
  2026-07-03 grill decided the product direction on top of it: present one
  ranked catalog as Top 250 (default empty-state CTA) / 500 / 1000 milestones;
  never bulk-mark a tier as known — learners skip, test out with fast
  individual "I know this" confirmations, or start the milestone; known state
  records individually confirmed forms. The current deck is lemma-ranked
  (summed token mass across inflections); re-key it during the surface-first
  known-word migration. Not a launch blocker by itself, but the test-out flow
  feeds the surface-first known-evidence model, which is a launch gate.
- [ ] **First-run register picker**. Once on first sign-in, ask "What kinds of texts do you want to read most? Conversation / News & books / Mixed." Persists to `user_language_settings`. Drives which top-1000 register the cold-start uses, and may later weight new-card ranking.
- [ ] **Account deletion**. Cascade through parses, decks, known-word lists, sessions. Profile page is otherwise out of scope for the first version, but deletion is privacy-table-stakes.
- [x] **Privacy chip on the parse form**. Persistent visible signifier under the signed-in parse textarea replaces the doc-only privacy commitment in `FEATURES.md`.

Already on this list and just confirmed by the review:

- Anonymous parser demo is paste-first for alpha. File upload extraction
  (`.txt` / `.md` / `.epub`) remains a signed-in Inspect/workbench capability
  unless a later decision deliberately expands anonymous scope.
- FSRS migration — landed and enabled by default 2026-07-04 (Decision 23). The public review surface now runs narrow FSRS (`go-fsrs/v3`); the hand-rolled step scheduler is the opt-out rollback fallback. Marketing the review scheduler as FSRS is now accurate.
- Comprehension prediction per deck — wireframe is in `docs/USER_FLOWS.md` §8
- Rate limiting and parser backpressure on `/api/parse` — launch-gating now that
  anonymous parsing is a public product surface.
- Highest-leverage study ordering across decks — recommended UX gate is "user has 2+ decks", not always-on

## Notes

### Post-Alpha Follow-Ups from Alpha PR Review

- [x] Replace the legacy mock-auth/raw-cookie path; current auth uses Argon2id password hashes and DB-backed `session_token` sessions
- [x] @chickendude go-live: add rate limiting, abuse controls, and CSRF/strict-Origin posture to parse, feedback, login, register, and other cookie-authenticated state-changing routes; see `docs/GO_LIVE_CHECKLIST.md`
- [ ] Define and implement a retention policy for `parse_sessions.source_text`; current alpha behavior is ephemeral parse by default, with source context retained only for saved decks and parser feedback
- [ ] Preserve existing `card_state` scheduling data when rebuilding `cards` during schema migrations instead of dropping and recreating the table
- [ ] Batch known/ignored checks during deck creation so card seeding does not do one lookup per unique `(lang, lemma, pos)` pair
- [ ] Replace `COUNT(*)` existence checks in known-word and parse-feedback paths with `EXISTS`/short-circuit queries once alpha correctness work is merged
- [x] Parse history / deletion UI so logged-in users can review and delete source context retained by saved decks and parser feedback
- [x] Make signed-in `/api/parse` ephemeral by default; no per-parse opt-out flag needed
- [x] Document parse-session storage behavior directly in the parse UI, not only in docs

- These findings were identified during PRD review and stub implementation
- Items are organized by severity and implementation priority
- Check off items as they are completed
- Update this document as new findings emerge or priorities change

---

## 2026-04-29 — Consumer alpha execution plan

**This was the locked execution plan when the alpha was scoped. Most of
PR 1–6 has shipped (auth roles, frontend surface split, known
words/global cards, parse feedback subsystem, deck CRUD + review). PR 7
(ET evaluation parity) is partially shipped via Plan C / PRs
[#113](https://github.com/sagarinbabel/finnestdb/pull/113)/[#114](https://github.com/sagarinbabel/finnestdb/pull/114)/[#115](https://github.com/sagarinbabel/finnestdb/pull/115)
+ FST migration. PR 8 (Track B live quality metrics) and PR 9 (security
review) remain open. The full plan is preserved here for traceability
and is not actively re-litigated. See "What's not in main yet" above
for up-to-date open work.**

This is the locked execution plan for the FinnEst consumer alpha. Where
this plan disagrees with older sections of `TODO.md`,
`finnestdb-prd-alpha.md`, `ARCHITECTURE.md`, or `docs/IMPLEMENTATION.md`,
this plan wins. Older sections remain for historical context but are not
re-litigated here.

Companion docs introduced alongside this plan:

- [docs/FEATURES.md](docs/FEATURES.md)
- [docs/CROSS_LANGUAGE_STRATEGY.md](docs/CROSS_LANGUAGE_STRATEGY.md)
- [docs/CHANGELOG.md](docs/CHANGELOG.md)

#### Summary

- Build the alpha as a consumer language-learning product with a clear split
  between:
  - public/anonymous product surfaces
  - authenticated user study surfaces
  - admin-only parser and feedback operations
- Ship only when Finnish and Estonian are both first-class across the same
  core user flow: `paste -> inspect -> correct -> deck -> review`.
- Use global cards so vocabulary knowledge belongs to the user, not to decks.
- Keep the full parser workbench admin-only.
- Allow logged-in users to access a lightweight parse-inspection view and
  submit parser corrections.
- Use two evaluation tracks:
  - Track A: offline gold + external benchmark
  - Track B: live accepted-correction metrics from real usage
- External benchmark parity:
  - Finnish: Omorfi
  - Estonian: EstNLTK / Vabamorf
- Reuse [`docs/baselines/`](docs/baselines/) as the canonical baseline store.
- Implement in reviewable slices and open PRs per slice; resolve conflicts
  as they arise, then stop for review before merge.

#### Product model

- **Anonymous visitor**
  - can see landing and product explanation surfaces
  - can sign in
  - cannot create decks
  - cannot review
  - cannot submit full parser corrections in alpha
- **User**
  - can sign in
  - can paste/import text
  - can view lightweight parse inspection
  - can import known words
  - can create decks
  - can review cards
  - can submit parser corrections
  - cannot access workbench internals, admin queue, or benchmark/eval tools
- **Admin**
  - everything a user can do
  - full parser workbench access
  - feedback triage queue
  - parser comparison/eval surfaces if exposed
  - weekly parser quality reporting
  - annotation/testing surfaces
- **Michael's scope**
  - production auth and session model
  - user/admin role separation
  - payment/paywall/free-tier limits
  - deployment and live hardening
  - testing and ET annotation support
  - not parser-quality strategy owner

#### Implementation sequence

##### PR 1 — Planning and product docs

- Append this plan to `TODO.md` under a dated execution-plan section.
- Create `docs/FEATURES.md`, `docs/CHANGELOG.md`, and
  `docs/CROSS_LANGUAGE_STRATEGY.md`.
- Update live planning/architecture docs with dated headers and changelog
  references.
- `docs/FEATURES.md` is user-perspective and decision-complete about: what
  the product is, how users learn before reading, leverage and comprehension
  concepts, progress tracking concept, mobile web direction, and technology
  differentiators (fast parser, benchmarked quality, user-correction loop,
  inflected-form-aware frequency). Autoresearch belongs in the post-live idea
  parking lot, not the active alpha plan.

##### PR 2 — Auth roles and surface separation

- Extend the current mock-cookie auth into a role-aware alpha auth model.
- Add an admin flag to the user model and role-aware response behavior in
  `GET /api/me`.
- Split surfaces into anonymous, authenticated user, and admin-only.
- Restrict the current workbench in `web/index.html` and `web/app.ts` to
  admin access.
- Add a lightweight parse-inspection surface for logged-in users.
- Correction submission requires login. No anonymous full correction flow.

##### PR 3 — Frontend surface split

- Separate anonymous, user, and admin surfaces in the UI.
- Keep the existing frontend architecture in `web/app.ts`.
- Add: landing/product explanation, sign-in, deck list, rename/delete,
  review, known-word import/manage, lightweight parse inspection,
  admin-only workbench gating, admin feedback queue surface.
- Preserve one responsive app and existing breakpoints in `web/styles.css`.
- Validate mobile usability at 375 px.

##### PR 4 — Known words and global cards

- Keep global cards as the alpha model.
- Implement known-word import with canonical resolution at import time using
  the existing resolver chain in `internal/store/dict.go`.
- `POST /api/known-words` returns resolved imports and unresolved inputs.
- `GET /api/known-words?lang=`
- delete-one support for known words
- Deck creation:
  - persists `sentences` and `occurrence`
  - derives unique `(lemma, pos)` pairs
  - skips `user_known_lemmas` and `user_ignored_lemmas`
  - ensures one global `cards` row and one `card_state` row per remaining pair

##### PR 5 — Parse feedback subsystem

- Add or verify `parse_feedback` schema in `internal/store/db.go`.
- Add parse/session identifiers to parse results so feedback ties to a
  specific run.
- Implement `POST /api/parse/feedback`.
- Logged-in users can submit corrections from the lightweight
  parse-inspection view.
- Add admin queue surface (`/admin/feedback.html` or equivalent) with
  accept / reject / needs follow-up actions.
- Accepted corrections become the official signal for live error metrics.
- Document the full flow in `docs/PARSER_FEEDBACK_LOOP.md`.

##### PR 6 — Deck CRUD and review flow

- Implement: `GET /api/decks`, `PATCH /api/decks/{id}`, `DELETE /api/decks/{id}`.
- Replace mocked dashboard counts in `GET /api/me`.
- `DELETE /api/decks/{id}` deletes only the deck content graph
  (`occurrence`, `sentences`, `decks`). Do not delete `cards` or
  `card_state`.
- Historical note: PR 6 shipped the alpha step scheduler. This has been
  superseded — surface-card review identity plus narrowly-scoped real FSRS landed
  and became the default scheduler on 2026-07-04; the step scheduler is now the
  opt-out rollback fallback. See "Migrate alpha review identity and scheduler
  before public alpha" above.
- `GET /api/review/next?deck_id=` means due global cards, optionally
  filtered to cards appearing in the selected deck's occurrences.
- `POST /api/review/answer`, `POST /api/card/known`, `POST /api/card/ignore`.
- Alpha backside content is intentionally thin: lemma, gloss, one example
  sentence, optional grammar label.

##### PR 7 — Track A evaluation parity (Estonian)

- Use two offline subtracks for each language: gold dataset evaluation and
  external benchmark comparison.
- Finnish: compare `basic`, `custom`, `omorfi`.
- Estonian: compare `basic`, `custom`, external EstNLTK/Vabamorf adapter mode.
- Expand ET manual gold to at least Finnish manual scale and comparable
  annotation density.
- Audit `internal/parserules/estonian.go` against
  `internal/parserules/finnish.go`: implement ET equivalents where
  appropriate; document N/A where not applicable; add ET-specific handling
  for already-identified morphology categories.
- Add `make eval`, `make compare-parsers`, `make eval-check`.
- Freeze FI and ET reports under `docs/baselines/`.
- `docs/EVAL_AND_CI.md` describes gold evaluation, external benchmark
  evaluation, and baseline regression policy.

##### PR 8 — Track B live quality metrics

- Define production metrics sourced from parse usage plus accepted
  corrections.
- Minimum capture: parse id, user id, language, parser mode, token count,
  unique lemma count, correction submissions, accepted corrections.
- Minimum derived metrics: accepted correction rate per 1,000 tokens and
  per 1,000 unique lemmas, by language and by parser mode.
- Deliver first as a weekly admin report, not a polished analytics dashboard.
- If AI assistance is used, it drafts evidence summaries and candidate
  classifications only; admins approve all correction writes.
- Document Track B in `docs/EVAL_AND_CI.md` and
  `docs/PARSER_FEEDBACK_LOOP.md`.

##### PR 9 — Security review and hardening pass

- Scope: auth/session behavior, role enforcement on admin-only routes, CSRF
  posture for cookie-based auth, XSS exposure in feedback and parse views,
  rate limiting on login and feedback endpoints, data isolation between
  users, admin-route leakage to non-admins, correction submission abuse
  surface.
- Record findings and dispositions in `docs/SECURITY_REVIEW_ALPHA.md`.
- Fix any high-severity issues before stopping for merge review.

#### Parallel ownership split

- **Main backend owner**: PR-2 (Auth/Roles), PR-4 (Known Words + Global
  Cards), PR-5 (Parse Feedback), PR-6 (Deck CRUD + Review), PR-8 (Track B
  Reporting), PR-9 (Security Review).
- **Second model (parallel safe)**: PR-1 (Planning + Product Docs), PR-3
  (Frontend Surface Split, after PR-2 contract is fixed), PR-7 (ET
  Evaluation + Benchmark + Baselines).

High-conflict files where parallel edits must be avoided:

- `internal/api/handlers.go`
- `internal/store/db.go`

#### Public APIs

- `GET /api/me` — role-aware, real dashboard counts
- `POST /api/auth/login` — role-aware session behavior (alpha entry)
- `GET /api/decks`
- `PATCH /api/decks/{id}`
- `DELETE /api/decks/{id}`
- `POST /api/known-words`
- `GET /api/known-words?lang=`
- `GET /api/review/next?deck_id=`
- `POST /api/review/answer`
- `POST /api/card/known`
- `POST /api/card/ignore`
- `POST /api/parse/feedback`
- Admin-only feedback review interface

#### Documentation deliverables

- `docs/FEATURES.md` (this PR)
- `docs/CHANGELOG.md` (this PR)
- `docs/CROSS_LANGUAGE_STRATEGY.md` (this PR)
- `docs/SYSTEM_OVERVIEW.md` (later PR)
- `docs/PARSER.md` (later PR)
- `docs/PARSER_FEEDBACK_LOOP.md` (PR 5)
- `docs/EVAL_AND_CI.md` (PR 7 / PR 8)
- `docs/KNOWN_WORDS.md` (PR 4)
- `docs/MICHAEL_TODO.md` (later PR)
- `docs/SECURITY_REVIEW_ALPHA.md` (PR 9)

#### Acceptance criteria

1. This plan is appended to the repo's live planning doc and linked from
   `docs/CHANGELOG.md`.
2. Anonymous users can paste text, parse it, get a word list, and explore the
   list, but cannot persist state, submit corrections, create decks, review,
   import known words, or manage history without sign-in.
3. Logged-in users can complete `paste -> inspect -> correct -> deck -> review`
   in both FI and ET.
4. Admins can access workbench and feedback queue; normal users cannot.
5. Full correction submission requires login.
6. Known-word import resolves to canonical `(lemma, pos)` and reports
   unresolved inputs.
7. Deck deletion removes deck content but not global learning state.
8. Finnish has gold evaluation plus Omorfi comparison.
9. Estonian has gold evaluation plus EstNLTK/Vabamorf comparison.
10. ET manual gold reaches at least FI manual scale and comparable density.
11. `make eval`, `make compare-parsers`, and `make eval-check` cover both FI
    and ET.
12. Frozen FI and ET baseline reports live under `docs/baselines/`.
13. Weekly admin reporting shows accepted correction rates by language and
    parser mode.
14. `docs/CROSS_LANGUAGE_STRATEGY.md` explicitly captures how the parsers
    improve together at the strategy level.
15. A lightweight security review is completed and documented before signoff.
16. PRs are opened in reviewable slices and work stops for review before
    merge.

#### Assumptions

- Global cards are the alpha learning model.
- Parser workbench is admin-only.
- Lightweight parse inspection is user-visible only after login.
- Anonymous full correction submission is out of scope for alpha.
- Omorfi remains the Finnish external benchmark.
- EstNLTK / Vabamorf is the Estonian external benchmark.
- `docs/baselines/` is the single canonical baseline store.
- Cross-language improvement is shared at the
  infrastructure/evaluation/error-taxonomy layer, not by copying morphology
  blindly between languages.
