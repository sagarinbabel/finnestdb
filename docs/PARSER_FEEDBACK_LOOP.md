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
dictionary/enrichment/parser improvements. Accepted lemma/POS fixes are
applied only after admin approval, as `custom_overrides` lexical rows.
Grammar/FEATS writeback and eval-gated promotion are tracked in
[`TODO.md`](../TODO.md) "Close the self-improving feedback loop".

Accepted feedback should be classified with
[`CORRECTION_TAXONOMY.md`](CORRECTION_TAXONOMY.md) before writeback. Not every
learner-visible bad card is a parser identity bug: some are meaning-cue,
contextual-sense, phrase-boundary, example-quality, or card-presentation fixes.
Writing each fix to the smallest durable layer keeps pasted text, EPUBs,
articles, subtitles, Anki imports, and future catalog decks on the same path.

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

## Admin triage

The shared queue is at `/api/admin/parse-feedback` (admin-only). Filters
by `status` and language; admins can change `status` per submission.

When an admin marks feedback `accepted`, proposed lemma/POS corrections write
`forms` and `lemmas` rows with `source='custom_overrides'`,
`source_priority=1000`, and `parse_feedback_id` back-pointers. Later parses
rank those rows above lower-priority dictionary sources. See
[`TODO.md`](../TODO.md) "Close the self-improving feedback loop" for the
remaining FEATS update and eval-gated promotion phases.

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
  the 5-phase plan to wire accepted corrections into lexical updates.
- [`docs/FEATURES.md`](FEATURES.md) "User correction loop" — how the
  feature is positioned to users.
