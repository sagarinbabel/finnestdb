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
row, eval-gated acceptance, and gold-candidate promotion (Phases 1-4, shipped
2026-07-02). Weekly admin reports, AI-assisted triage, flag-only feedback,
source-agnostic overlay tables, and faulty-content quarantine are not built
yet.

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
  user thinks it should say
- `note` — free-form context
- `status` — `submitted` / `accepted` / `rejected` / `needs_follow_up`
  (admin-managed)

Current implementation requires `proposed_lemma` and `proposed_pos`. The alpha
meaning-check flow needs the planned flag-only extension: nullable proposed
fields plus `flag_only=true`, so learners can report "none of these meanings
looks right" without inventing a correction.

## Current implementation vs alpha target

| Area | Current implementation | Alpha target |
|------|------------------------|--------------|
| Learner entry point | Per-row correction button opens a modal. | Hover/focus `Wrong?` entry point from results rows, review cards, and low-confidence meaning UI. |
| Feedback type | Exact correction only. The UI and API require proposed lemma and POS. | Two paths: flag-only issue report, or proposed correction with lemma/POS and optional grammar/note. |
| Schema/API | `parse_feedback.proposed_lemma` and `proposed_pos` are `NOT NULL`; `ParseFeedbackRequest` requires them. | Proposed fields nullable when `flag_only=true`; store model and API response expose the flag. |
| Admin triage | Admin can list by status and accept/reject/follow up. | Admin can filter flag-only feedback and classify whether an issue is parser identity, meaning cue, context sense, phrase boundary, example quality, or card presentation. |
| Acceptance behavior | Accepted feedback writes `custom_overrides` from proposed lemma/POS and changes future lookups; acceptance is eval-gated against `gold_surfaces` (HTTP 409 on contradiction) and repeat corrections auto-queue as `gold_candidates`. | Only accepted parser-identity corrections with concrete lemma/POS write lexical overrides. Flag-only reports do not write the lexicon until an admin supplies/accepts a concrete correction. |
| Grammar/FEATS | Accepted grammar labels map through `udfeats` onto the override row's `forms.feats` (shipped 2026-07-02); corrected FEATS live only on the `custom_overrides` row. | Done for parser-identity overrides; richer meaning/card layers go through future overlay work. |
| Existing learner decks/cards | Feedback does not immediately mutate the current deck/card. It changes future parser output after admin acceptance. There is no shipped quarantine/suppression path for already-created faulty cards. | Preserve learning history, but remove known-faulty content from circulation after admin acceptance: skip/suppress bad occurrences or cards, or render accepted overlays for cue/sentence/explanation/sense. |
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

Feedback submissions should attach to a global correction issue, not remain only
as isolated per-user rows. Keep the alpha schema minimal:

- `parse_feedback` remains raw report intake.
- `correction_issues` owns global admin state, duplicate counts, scope, status,
  quarantine/fix metadata, and reopened/regression markers.
- `parse_feedback.correction_issue_id` links each report to its issue.

Do not add separate `quarantine_targets` or rich event tables for alpha unless
the implementation cannot preserve traceability without them. The issue row plus
linked report rows should carry the first version of the lifecycle:

- `reported`: first report received, with reporter, time, source context, and
  original parser/card state.
- `duplicate_reported`: another learner reports the same scoped issue.
- `triaged`: admin classifies correction type and scope.
- `quarantined`: confirmed-bad content is suppressed globally for matching
  learners.
- `fixed`: overlay, lexical writeback, replacement, or reparse action applied,
  with fix version and admin.
- `reopened`: a later report shows the fix missed a scope or regressed.

Scope should be explicit: global parser identity, surface+sense, phrase target,
specific source occurrence class, sentence/cue/explanation, or card presentation.
Confirmed quarantine/fixes apply to every learner whose content matches the
scope. Raw unreviewed reports should normally create/update the issue without
auto-hiding content globally unless an admin or trusted threshold confirms it.

## Report-to-quarantine workflow

The alpha flow should work like this:

1. Learner submits feedback from Inspect, deck detail, or review.
2. Server computes a conservative scope fingerprint, such as language,
   correction type if known, normalized surface, lemma/POS if present, parser
   mode, source occurrence reference, and optional sentence/context hash.
3. Server finds an open matching `correction_issues` row or creates one.
4. Feedback row is stored and linked to the issue. The issue gets a `reported`
   or `duplicate_reported` event with reporter, timestamp, parse/deck context,
   original analysis, proposed correction if any, and note.
5. Issue remains visible in admin triage immediately, but content stays live by
   default.
6. Quarantine can happen by one of two admin paths:
   - admin accepts **Quarantine now**, with required reason and explicit scope;
   - admin accepts a correction and chooses a quarantine/overlay action.
7. Quarantine marks the scoped issue as globally suppressed. Review and new-card
   queries skip matching content, or render the accepted overlay if one exists.
8. Fix writes a `fixed` event with fix version, admin, and action taken.
9. Later reports against the same scope after a fix create `reopened` events, so
   admins can tell whether the fix regressed or missed part of the scope.

For alpha, trusted thresholds are traceability-only. The system can compute
duplicate counts, distinct reporter counts, and `threshold_candidate` badges for
admins, but it must not auto-quarantine globally. Admin confirmation is the
default path; emergency quarantine is the fast path.

Learner-facing quarantine should be quiet. Quarantined items disappear from
review/new-card queues globally. Deck detail may show neutral copy such as
"Removed from study after review" only where omitting the row would confuse the
learner. Full report/fix/quarantine traceability belongs in admin views.

Current learner-facing stats should follow the same rule: active deck word
counts, due counts, new-card counts, comprehension/coverage estimates, and
next-unlock projections exclude quarantined content. Historical/admin views can
include quarantined content with issue metadata.

Fixing a quarantined issue restores the same study item by default. Create a new
study item only when the learning target identity changes: wrong lemma/POS,
wrong sense, homograph split, phrase/MWE replacing a single-token target, or
invalid target retirement. Past review history remains historical; current
rendering uses the fixed content.

When the same study item is restored, keep its existing review/FSRS scheduler
state. Quarantine pauses circulation; it does not reset memory. Reset or
reintroduce scheduling only when the fix creates a new learning target identity.

## Alpha admin classification

Alpha admin triage should require one simple category before quarantine/fix:

- `parser issue` — parser identity or grammar analysis appears wrong.
- `bad card content` — learner-facing cue, explanation, sense, phrase boundary,
  or presentation is wrong or misleading.
- `source/extraction issue` — source sentence/text extraction is faulty.
- `not sure` — needs investigation before routing.

The full correction taxonomy stays available as optional detail for deeper
cleanup and reporting. Do not block urgent quarantine on precise taxonomy labels.

For alpha, the admin UI should not become a broad data editor. It should support
classification, notes, report grouping, duplicate counts, and **Quarantine now**.
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
status and language; admins can change status per submission. Issue-aware
filters and per-issue status changes arrive with the planned
`correction_issues` work (Phase 1c).

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
