# SRS and Deck System Draft

## Recommendation

Use FSRS for scheduling.

Use the FSRS algorithm at the product level, but prefer the official Go implementation for the app runtime and keep `fsrs-rs` as the future optimization path.

Why:

- The current backend, API, and persistence layers are already Go- and SQLite-centric.
- Scheduling reviews is not computationally heavy enough to justify adding a second production FFI path.
- The official `fsrs-rs` project is strongest when we want parameter training and simulation.
- The official `go-fsrs` module is a cleaner fit for day-to-day scheduling inside this codebase.

Practical decision:

- Runtime scheduler: `github.com/open-spaced-repetition/go-fsrs`
- Future offline optimizer: `open-spaced-repetition/fsrs-rs`

## Product Model

### Core entities

- `deck`
  - A media object in a recursive hierarchy.
  - Can represent a top-level show/series, a season, or a studyable leaf like a book, movie, or TV episode.
  - Should carry its own `content_type` so media can be searched and filtered.
  - Only studyable leaf decks have parsed sentences and lemma occurrences.

- `card`
  - A user-level learning item keyed by `(user_id, lang, lemma, pos)`.
  - Global across all decks in the same language.
  - A lemma should not create duplicate cards just because it appears in multiple decks.
  - Carries a definition for study and an example sentence when available.

- `new-card source`
  - The active source for new-card introduction.
  - A selected deck.
  - Can be a studyable leaf deck or a parent deck that aggregates descendant leaf decks.
  - Determines which new lemmas are eligible to be introduced.

### Important distinction

Keep `content selection` separate from `card identity`.

- Content decides which new words are available.
- FSRS decides when already-added cards are due.
- A review session should mix:
  - due cards from the user's existing global card set
  - new cards drawn from the currently selected new-card source

## Proposed schema direction

The current schema already has good primitives for deck-backed parsed content, cards, card state, and known lemmas. The main missing pieces are deck hierarchy support, aggregated lemma frequency tables, and richer card content fields.

### Keep

- `decks`
- `sentences`
- `occurrence`
- `cards`
- `card_state`
- `user_known_lemmas`
- `user_ignored_lemmas`

### Extend

- `decks`
  - add `parent_deck_id` nullable
    - `parent_deck_id` points to the parent deck in the hierarchy
  - add `content_type`
    - examples: `book_series`, `book`, `tv_show`, `tv_season`, `tv_episode`, `movie`, `article`

- `cards`
  - add `lang`
  - add `definition_text`
  - add `definition_source`
  - add `shared_example_sentence_id`
  - add `custom_example_text`
  - add `custom_example_translation`

- `example_sentences`
  - `id`
  - `lang`
  - `text`
  - `translation_text`
  - `source_label`

### Add

This keeps one unified model for search, browsing, and study.

Recommended behavior:

- top-level search results return parent decks (top-level TV show or book series) and standalone leaf decks (individual movies, books that aren't in a series, etc.)
- clicking a parent deck shows its child decks
- studying a parent deck means aggregating across descendant decks
- studying a leaf deck means using only that deck

Optional later deck metadata:

- `series_title`
- `season_number`
- `episode_number`
- `release_year`

- `deck_lemma_stats`
  - `deck_id`
  - `lemma`
  - `pos`
  - `token_count`
  - `sentence_count`
  - `first_sentence_id`
  - Unique on `(deck_id, lemma, pos)`

- `user_deck_status`
  - `user_id`
  - `deck_id`
  - `last_studied_at`
  - `is_pinned`
  - Optional per-user metadata for dashboards

### Possibly add later

- `card_sources`
  - Maps a card to the decks where it appears.
  - Not required for MVP if source membership can be derived from `deck_lemma_stats`.

## Aggregation rules

### Per-deck lemma stats

For each studyable deck, aggregate occurrences by `(lemma, pos)`:

- `token_count`: how many times the lemma appears in the deck
- `sentence_count`: how many sentences contain the lemma
- `first_sentence_id`: first useful example source

This table should be written when a deck is created, not recalculated live from raw occurrences for every request.

### Parent-deck stats

Parent decks do not need a dedicated physical table in MVP.

Parent-deck frequencies can be computed as:

- `SUM(token_count)` across all descendant studyable decks
- grouped by `(lemma, pos)`

If this becomes too slow later, add a materialized `deck_aggregate_lemma_stats` table.

## Learning state model

We need three different states, not one:

- `known`
  - The user already knows the lemma.
  - Comes from manual marking, import, or a mature-card rule.

- `studying`
  - The lemma has a card and is in FSRS, but is not yet considered known.

- `ignored`
  - The user does not want this lemma in study.

### Recommendation

Do not treat "has a card" as "knows the word."

That would overstate comprehension badly, especially for high-frequency words introduced recently.

## New card selection

When the user studies a selected deck, new cards should come from:

`eligible lemmas in source`
minus `known lemmas`
minus `ignored lemmas`
minus `existing cards`

### Ranking

Default ranking should be:

1. highest `token_count` in the selected source
2. earlier first appearance in the source
3. stable tie-break by `lemma`, `pos`

This keeps the queue aligned with actual content payoff.

### Scope behavior

- Studying a leaf deck pulls new words only from that deck.
- Studying a parent deck pulls new words from the union of its descendant studyable decks.
- Existing due cards are still global for that language and should appear regardless of which source the user selected.

### Naming recommendation

Use `new-card source` in the product and API instead of `study source`.

That is more precise because the selected deck only scopes which new cards can be introduced. It does not scope global due reviews.

## Coverage metrics

There should be two primary coverage numbers for each deck.

### 1. Lemma coverage

How many unique lemmas in the source the user knows.

Formula:

`known_unique_lemmas / total_unique_lemmas`

### 2. Token-weighted coverage

How much of the source the user knows when repeated words count more.

Formula:

`sum(token_count for known lemmas) / sum(token_count for all lemmas)`

This is the better proxy for "how much of this content can I understand?"

## What counts as "known" for coverage

This needs a rule. Recommended MVP rule:

- `known` if the lemma is in `user_known_lemmas`
- or the card has graduated beyond a maturity threshold

Recommended maturity threshold for MVP:

- interval at least 21 days

Alternative later:

- use FSRS memory state directly, such as a minimum stability threshold
- or expose multiple views: `learning coverage` vs `known coverage`

## Review session structure

Each study session should have two lanes:

- `reviews`
  - Cards due now according to FSRS

- `new`
  - New cards pulled from the selected deck, limited by `new_per_day`

### Review order

There should be three review-order modes:

1. `new_first`
   - Show all new cards before due reviews.

2. `mixed`
   - Interleave new cards and due reviews in one session stream.

3. `new_last`
   - Show due reviews first and new cards at the end.

Recommended default:

- `new_last`

This keeps the scheduler honest and prevents backlog growth, while still allowing users who prefer immediate content unlocks to choose `new_first` or `mixed`.

### Rating scale

Use the standard four-button FSRS scale:

- Again
- Hard
- Good
- Easy

## Card content

A lemma card should be global, but it should still carry study content on the card itself and show deck-specific context when available.

Minimum card payload:

- lemma
- part of speech
- definition text
- saved example data if available

Definition and example behavior:

- `definition_text` should be stored on the card when the card is created.
- `shared_example_sentence_id` should point to a reusable example from the shared corpus.
- `custom_example_text` and `custom_example_translation` should store a private user override on the card itself.
- Display rule:
  - if `custom_example_text` exists, show the custom example and its custom translation if present
  - otherwise load `shared_example_sentence_id`
  - otherwise show no saved example
- If the active new-card source contains a better example for the same lemma, the session payload can override the saved example for display without changing the card's saved default state.
- Parsed deck sentences should not automatically become part of a shared example corpus.

Recommended review payload:

- lemma
- part of speech
- definition text
- one example sentence from the active new-card source if available, otherwise the card's saved example
- source label (`Book 1`, `Episode 3`, etc.)
- optional alternate example from another deck where the lemma appears

## Example sentence policy

There are two safe product paths for examples:

- private user overrides on cards
  - A user writes their own example sentence for their own card.

- licensed or otherwise cleared corpus examples
  - The app uses sentences from a corpus we have explicit rights to use.

Recommendation:

- Do not design the system around reusing sentences from uploaded books, subtitles, or TV transcripts as a shared corpus unless rights are cleared first.
- Keep user-deck sentence context separate from the reusable example-sentence corpus.
- For now, treat `example_sentences` as a corpus-owned dataset only, not a user-contributed table.

## Suggested API shape

### Content and stats

- `GET /api/decks`
- `GET /api/decks/:id/stats`
- `GET /api/decks/:id/children`

### Study

- `POST /api/study/session`
  - input: `{new_card_source_type, new_card_source_id, lang, review_order_mode}`
  - returns due cards plus new-card candidates

- `POST /api/study/answer`
  - input: `{card_id, rating, answered_at, new_card_source_type, new_card_source_id}`
  - updates FSRS state

### User word state

- `POST /api/lemmas/known`
- `DELETE /api/lemmas/known`
- `POST /api/lemmas/ignored`
- `DELETE /api/lemmas/ignored`

## MVP sequencing

### Phase 1

- Add recursive deck hierarchy
- Add `deck_lemma_stats`
- Build leaf and parent-deck coverage endpoints

### Phase 2

- Integrate runtime FSRS scheduling
- Create global cards by lemma
- Support due reviews plus source-scoped new cards

### Phase 3

- Add mature-card coverage rules
- Add richer progress dashboards
- Evaluate whether offline FSRS parameter optimization is worth the added complexity

## Open decisions

- Whether cards should be keyed by `(lemma, pos)` or just `lemma`
  - Recommendation: keep `(lemma, pos)` to avoid noun/verb collisions.

- Whether to support arbitrary user-made bundles later
  - Recommendation: not in MVP; start with canonical parent/child deck hierarchy only.

- Whether coverage should include ignored lemmas in the denominator
  - Recommendation: yes, because ignored is not known.

- Whether proper nouns should enter study
  - Recommendation: allow them to be filtered out at the deck level, not hard-coded globally.

## Current recommendation in one sentence

Use FSRS, but do not wire `fsrs-rs` directly into the production request path yet; keep scheduling in Go, make cards global per lemma, use a recursive deck hierarchy as the source of new-card selection, store a definition and example sentence on cards when available, and compute comprehension with both unique-lemma and token-weighted coverage.
