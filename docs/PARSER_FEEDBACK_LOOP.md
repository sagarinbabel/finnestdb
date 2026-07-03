# Parser Feedback Loop

_Created 2026-05-07 (consolidated from `docs/IMPLEMENTATION.md` "Suggest fix"
section, which has been replaced with a redirect stub.)_

This doc describes the parser-feedback flow ("Suggest fix") for the
admin queue and the recommended UX/semantics. The schema and live wiring
live in `internal/api/handlers.go` and `internal/store/db.go`; this is the
human-facing contract.

## Intent

"Suggest fix" is **parser feedback**, not an immediate "fix my deck"
action. Submissions are queued for admin review and inform future
dictionary/enrichment/parser improvements. Accepted lemma/POS *and
grammar-label* fixes are applied only after admin approval, as
`custom_overrides` lexical rows (grammar labels become UD FEATS on the
override). Grammar/FEATS writeback, eval-gated acceptance, and gold-candidate
promotion shipped 2026-07-02; see [`TODO.md`](../TODO.md) "Close the
self-improving feedback loop" (Phases 1–4 live; Phase 5 source re-ranking
parked).

Accepted feedback should be classified with
[`CORRECTION_TAXONOMY.md`](CORRECTION_TAXONOMY.md) before writeback. Not every
learner-visible bad card is a parser identity bug: some are meaning-cue,
contextual-sense, phrase-boundary, example-quality, or card-presentation fixes.
Writing each fix to the smallest durable layer keeps pasted text, EPUBs,
articles, subtitles, Anki imports, and future catalog decks on the same path.

Current implementation covers the queue, status triage, accepted lemma/POS
`custom_overrides` writeback with grammar-label -> UD FEATS on the override
row, eval-gated acceptance, gold-candidate promotion (Phases 1-4, shipped
2026-07-02), flag-only feedback (Phase 1b, shipped 2026-07-04), and global
correction issues with admin-only faulty-content quarantine (Phase 1c, shipped
2026-07-04). Weekly admin reports, AI-assisted triage, and source-agnostic
correction-overlay tables are not built yet.

## UX recommendations

### Always attach a parse session

- Inspect results already carry `parse_id` from `POST /api/parse`.
- Reopened deck detail should also carry a real `parse_id` — either the
  deck's originating `parse_session`, or a new "replay" session created
  on open.
- Avoid any UX that exposes the button but cannot submit.

### Make copy explicit

- Consider renaming the CTA to "Report parser issue" / "Suggest correction".
- In the modal, state clearly: feedback is queued for review and **does
  not** immediately change deck contents.
- From low-confidence meaning checks, label the escape hatch **"None of these
  looks right"** and route it here. That action means "the app's analysis looks
  wrong"; it is not a known-word confirmation and not a request to create a
  study card.

### Include enough context for triage

- Show the source sentence (with the surface form highlighted).
- Include parser mode and whatever stable token reference is available
  (occurrence, token index, etc.) so reviewers can reproduce.

### Close the loop

- On success: confirm submission and that it's queued for review.
- On failure: show a specific, actionable reason (missing parse session,
  auth required, etc.), not a generic error.

### Guardrails

- Hide/disable for rows that are not meaningful to correct (e.g. PUNCT).
- Add basic spam-prevention later (rate limiting / dedupe) as needed.

## Server-side schema

Parse-feedback rows live in `parse_feedback` per
`internal/store/db.go::CreateParseFeedback`. Each row stores:

- `parse_session_id` — back-pointer to the parse run that produced this token
- `user_id` — feedback submitter (login required per
  [`docs/DECISIONS.md` Decision 4](DECISIONS.md))
- `lang`, `parser`, `surface` — what was being parsed
- `occurrence` — stable token reference (sentence index, token index)
- `original_lemma`, `original_pos`, `original_grammar_label` — what the
  parser said
- `proposed_lemma`, `proposed_pos`, `proposed_grammar_label` — what the
  user thinks it should say (empty for flag-only rows)
- `flag_only` — `1` when the learner reported "this looks wrong" without
  proposing a fix; `0` for a concrete correction
- `note` — free-form context
- `status` — `submitted` / `accepted` / `rejected` / `needs_follow_up`
  (admin-managed)

A concrete correction requires `proposed_lemma` and `proposed_pos`. Flag-only
reports (`flag_only=1`, shipped 2026-07-04) let learners report "none of these
meanings looks right" without inventing a correction; their proposed columns stay
empty. The columns remain `NOT NULL` — a SQLite table rebuild just to relax two
constraints was disproportionate, so validation enforces non-empty proposed
fields only when `flag_only=0`. An admin can attach a concrete lemma/POS to a
flag-only row at accept time, converting it into a normal parser-identity
correction; only then does it write a lexical override.

## Current implementation vs alpha target

| Area | Current implementation | Alpha target |
|------|------------------------|--------------|
| Learner entry point | Per-row correction button opens a modal. | Hover/focus `Wrong?` entry point from results rows, review cards, and low-confidence meaning UI. |
| Feedback type | Two paths (shipped 2026-07-04): flag-only issue report, or proposed correction with lemma/POS and optional grammar/note. | Two paths: flag-only issue report, or proposed correction with lemma/POS and optional grammar/note. |
| Schema/API | `parse_feedback.flag_only` (`NOT NULL DEFAULT 0`, shipped 2026-07-04). `proposed_lemma`/`proposed_pos` stay `NOT NULL` and empty for flag-only rows; `ParseFeedbackRequest` requires them only when `flag_only` is false. Store model and API response expose the flag. | Proposed fields nullable/empty when `flag_only=true`; store model and API response expose the flag. |
| Admin triage | Admin can list by status, filter by `flag_only`, and accept/reject/follow up. A flag-only badge marks reports with no proposed fix. Correction-type classification (parser identity, meaning cue, etc.) is still future work. | Admin can filter flag-only feedback and classify whether an issue is parser identity, meaning cue, context sense, phrase boundary, example quality, or card presentation. |
| Acceptance behavior | Accepted concrete corrections write `custom_overrides` from proposed lemma/POS and change future lookups; acceptance is eval-gated against `gold_surfaces` (HTTP 409 on contradiction) and repeat corrections auto-queue as `gold_candidates`. Accepting a flag-only report writes nothing to the lexicon unless the admin supplies a concrete lemma/POS, which converts it into a normal correction that then flows through the same eval-gated writeback. | Only accepted parser-identity corrections with concrete lemma/POS write lexical overrides. Flag-only reports do not write the lexicon until an admin supplies/accepts a concrete correction. |
| Grammar/FEATS | Accepted grammar labels map through `udfeats` onto the override row's `forms.feats` (shipped 2026-07-02); corrected FEATS live only on the `custom_overrides` row. | Done for parser-identity overrides; richer meaning/card layers go through future overlay work. |
| Existing learner decks/cards | Feedback does not immediately mutate the current deck/card. It changes future parser output after admin acceptance. Admin-confirmed quarantine (Phase 1c, shipped 2026-07-04) suppresses matching cards/occurrences globally from review, new-card, deck-stats, and comprehension surfaces without touching review history; restore returns them with scheduler state intact. | Preserve learning history, but remove known-faulty content from circulation after admin acceptance: skip/suppress bad occurrences or cards (shipped), or render accepted overlays for cue/sentence/explanation/sense (future). |
| Source context | Inspect parses are ephemeral, but feedback creates a retained parse session with source text for admin review. | Same, with clear privacy copy and retention/deletion controls. |

## Existing content and learning history

"Do not rewrite learner history" means the system should not edit past review
events or pretend the learner was shown different content. It does **not** mean
known-bad cards should keep appearing.

Target behavior after admin acceptance:

- Parser-identity fixes update future parser output through lexical overrides
  when global evidence is safe.
- Meaning-cue, contextual-sense, phrase-boundary, example-quality, and
  card-presentation fixes write overlay rows at the smallest durable layer.
- Bad current content can be quarantined: skipped in review/new-card queues,
  hidden from learner-facing deck recommendations, or shown with a reviewed
  replacement.
- Past review history and scheduler provenance remain auditable.

Minimal quarantine is a public-alpha gate. The rich overlay system can mature
later, but alpha should not knowingly keep accepted-bad content in learner study
queues.

## Global correction issue ledger

_Shipped 2026-07-04 (Phase 1c)._ Feedback submissions attach to a global
correction issue rather than staying isolated per-user rows. The alpha schema is
deliberately minimal:

- `parse_feedback` remains raw report intake.
- `correction_issues` owns global admin state: scope fingerprint
  (`lang, parser, norm_surface, lemma, pos`), `status`, `report_count`,
  `distinct_reporter_count`, first/last-reported timestamps, quarantine/fix
  metadata, `reopened_count`, alpha class, and admin note.
- `parse_feedback.correction_issue_id` links each report to its issue.

No separate `quarantine_targets` or rich event tables ship for alpha: the
duplicate/lifecycle evidence lives on the issue row plus its linked
`parse_feedback` rows. `report_count`/`distinct_reporter_count` are recomputed
from the linked rows on each submission, so they stay correct without an event
log. The lifecycle is expressed as the issue's `status`
(`open` → `quarantined` → `fixed` → `reopened`) plus the derived counts:

- first report creates the issue (`open`) with reporter time and scope.
- a duplicate report against the same scope bumps `report_count` (and
  `distinct_reporter_count` for a new reporter).
- an admin classifies the issue with a simple alpha class (triage).
- `quarantined`: confirmed-bad content is suppressed globally for matching
  learners.
- `fixed`: an admin restores/marks the issue fixed.
- `reopened`: a later report against a `fixed` issue flips it to `reopened` and
  bumps `reopened_count`, so a regression is distinguishable from a fresh case.

For alpha the scope is the conservative fingerprint above. Quarantine
suppression matches on the issue's `(lang, lemma, pos)` when present (the card
and its occurrences), else on `(lang, normalized surface)` (occurrence match).
Confirmed quarantine applies to every learner whose content matches the scope.
Raw unreviewed reports create/update the issue but never auto-hide content;
suppression requires an admin **Quarantine now** action. Rich correction
overlays and per-scope event logs remain future work.

## Report-to-quarantine workflow

_Shipped 2026-07-04 (Phase 1c); the overlay actions noted in steps 6/8 remain
future work._ The alpha flow works like this:

1. Learner submits feedback from Inspect, deck detail, or review.
2. Server computes a conservative scope fingerprint:
   `(lang, parser, normalized surface, proposed lemma, proposed pos)`. Flag-only
   and surface-only reports leave lemma/pos empty.
3. Server finds an open matching `correction_issues` row or creates one
   (`store.groupFeedbackIntoIssue`), in the same transaction as the insert.
4. The feedback row is stored and linked to the issue via `correction_issue_id`;
   `report_count`/`distinct_reporter_count` are recomputed from the linked rows.
5. The issue is visible in admin triage immediately, but content stays live by
   default.
6. Quarantine happens by the admin **Quarantine now** action, with a required
   reason and a prior alpha class. (Accepting a correction and choosing a
   quarantine/overlay action in one step is future overlay work; today an admin
   quarantines the issue as a separate action.)
7. Quarantine flips the issue to `quarantined`; the review, new-card,
   deck-stats, and comprehension queries skip matching content globally.
8. Restore/mark-fixed flips the issue to `fixed` with an optional fix note and
   returns the same content to circulation. (Overlay/replacement fixes are
   future work.)
9. A later report against a `fixed` scope reopens the issue (`reopened`) and
   bumps `reopened_count`, so admins can tell a regression from a missed scope.

For alpha, trusted thresholds are traceability-only. The system computes
duplicate counts, distinct-reporter counts, and a `threshold_candidate` badge
(≥3 distinct reporters), but it never auto-quarantines. Admin confirmation is
the only path to suppression.

Learner-facing quarantine is quiet: quarantined items disappear from
review/new-card queues globally with no scary copy. Full report/fix/quarantine
traceability lives in the admin views.

Current learner-facing stats follow the same rule: deck word counts, due
counts, new-card counts, and comprehension coverage/unlock projections exclude
quarantined content. Historical `review_log` rows and past reviews are
untouched, and admin views can include quarantined content with issue metadata.

Fixing (restoring) a quarantined issue returns the same study item to
circulation. Because suppression is a live query filter — no card or scheduler
rows are deleted at quarantine time — restore is a pure status flip and the
card's existing `card_state` (due date, history) is preserved. Quarantine pauses
circulation; it does not reset memory. Creating a new study item for a changed
learning target identity (wrong lemma/POS, homograph split, phrase replacing a
single token) remains future overlay work.

## Alpha admin classification

_Shipped 2026-07-04 (Phase 1c)._ Admin triage requires one simple category
before an issue can be quarantined or restored:

- `parser_issue` — parser identity or grammar analysis appears wrong.
- `bad_card_content` — learner-facing cue, explanation, sense, phrase boundary,
  or presentation is wrong or misleading.
- `source_extraction_issue` — source sentence/text extraction is faulty.
- `not_sure` — needs investigation before routing.

The full correction taxonomy stays available as optional detail for deeper
cleanup and reporting. Urgent quarantine is not blocked on precise taxonomy
labels.

For alpha, the admin UI is not a broad data editor. It supports classification,
notes, report grouping, duplicate counts, **Quarantine now**, and restore.
Parser-identity fixes can continue through the existing accepted lemma/POS
`custom_overrides` writeback. Meaning/card/source fixes should be handled by
manual code/data changes or future overlay work until real correction patterns
justify an in-app editor.

Use one combined admin feedback/issues queue for alpha. Do not add a separate
Issues page yet. The queue should expose issue-aware filters/statuses such as
`submitted`, `needs review`, `quarantined`, `fixed`, and `reopened`, plus report
counts and duplicate grouping. Split into a dedicated Issues page later only if
volume or workflow complexity demands it.

## Weekly admin triage target

The planned weekly run is not implemented today. A realistic first version is:

1. collect submitted and flag-only feedback with parse context;
2. group by language, parser mode, correction type, surface, and source type;
3. attach deterministic evidence: current parser result, dictionary candidates,
   Omorfi/EstNLTK comparison where available, and related prior feedback;
4. optionally ask an LLM to draft a summary, proposed classification, and
   questions for the admin;
5. require a human admin to accept/reject/edit before any writeback,
   quarantine, overlay, or eval-case promotion.

LLM output can support triage, but core lemma/POS/FEATS truth and write routing
must remain deterministic and human-approved.

## Admin triage

The shared queue is at `/api/admin/parse-feedback` (admin-only). Filters by
status and `flag_only`; admins can change status per submission. The same
combined admin page also renders the global correction-issue ledger from
`/api/admin/correction-issues` (admin-only, Phase 1c, shipped 2026-07-04):
list/filter issues by status, classify them, **Quarantine now** (required
reason), and restore/mark-fixed. A `threshold_candidate` badge appears at ≥3
distinct reporters but never auto-quarantines. No separate Issues page ships for
alpha.

When an admin marks feedback `accepted`, proposed lemma/POS corrections write
`forms` and `lemmas` rows with `source='custom_overrides'`,
`source_priority=1000`, and `parse_feedback_id` back-pointers. Later parses
rank those rows above lower-priority dictionary sources. Accepted grammar-label
corrections also map through `udfeats` (`featsFromCaseLabel`) onto the override
row's `forms.feats`, so corrected FEATS live only on the `custom_overrides` row
and a dictionary re-import can't silently revert or duplicate them.

Two safety rails close the loop (shipped 2026-07-02):

- **Eval-gated acceptance.** `make import-gold-surfaces` loads the frozen gold
  analyses into `gold_surfaces`; acceptance is refused (HTTP 409, full
  rollback) when ≥2 gold occurrences of the surface unanimously contradict the
  proposal. Runs in-transaction; an empty `gold_surfaces` table degrades to a
  no-op.
- **Gold-candidate promotion.** When 3 distinct users
  (`store.GoldPromotionThreshold`) have the same correction accepted, it
  upserts into `gold_candidates`; `make export-gold-candidates` prints pending
  rows as gold-token JSON for **manual** promotion into
  `testdata/parser-eval/*/gold` (auto-committing eval cases would let the
  system write its own exam).

Automatic source-priority re-ranking (Phase 5) stays parked deliberately; see
[`TODO.md`](../TODO.md) "Close the self-improving feedback loop".

Before accepting, admins should choose one primary correction type:

- parser identity;
- meaning cue;
- contextual sense;
- phrase boundary;
- example quality;
- card presentation.

Parser identity fixes can write lexical overrides and promote eval cases.
Meaning/card fixes should write the appropriate overlay row and add render tests
instead of pretending the parser lemma was wrong.

## Estonian uses the same path

The correction flow is shared — language-specific differences live in
analyzer choice and lexical sources, not in how users report mistakes
or how admins review them. ET-specific source choices live in
[`LEXICAL_PLAN.md`](LEXICAL_PLAN.md) "Estonian-specific source choices
and adapter contract".

The taxonomy is shared, but correction content stays language-specific. A
Finnish `sanoin` fix and an Estonian `peatus` fix use the same workflow; they
must not share overlay rows, morphology assumptions, or gold fixtures.

## See also

- [`docs/DECISIONS.md`](DECISIONS.md) Decision 4 — why parse feedback
  requires login.
- [`TODO.md`](../TODO.md) "Close the self-improving feedback loop" —
  the 5-phase plan to wire accepted corrections into lexical updates
  (Phases 1–4 live as of 2026-07-02; Phase 5 source re-ranking parked).
- [`docs/FEATURES.md`](FEATURES.md) "User correction loop" — how the
  feature is positioned to users.
