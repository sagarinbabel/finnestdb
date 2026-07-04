# SRS and Deck System Draft

_Current as of 2026-07-04 — see [CHANGELOG.md](CHANGELOG.md) for revisions._

> **Implemented (2026-07-04):** Surface-form card identity and narrow FSRS both
> shipped in code. Review cards are keyed by
> `(user_id, lang, surface_norm, lemma, pos)` — surface-form-in-context, with
> `(lemma, pos)` as the sense discriminator (homographs are separate sense
> cards, not collapsed). Narrow FSRS (`go-fsrs/v3`, default parameters) is wired
> behind the `FINNESTDB_FSRS_ENABLED` flag, which **defaults OFF**; the step
> scheduler (`nextAlphaStepScheduleForRating`) remains the shipped runtime path
> until the flag is flipped as a deploy decision (staging validation first — see
> [DEPLOYMENT.md](DEPLOYMENT.md)). Deferred items below (parameter optimization,
> `fsrs-rs`, rescheduling tools, simulation dashboards, mature-card analytics,
> broad review UX redesign, and the ambiguity-eval-gated meaning-check UI)
> remain later work.
>
> **Note (2026-07-03):** Public alpha should ship real FSRS scheduling, but in a
> narrow runtime-only scope: default parameters, current Again/Hard/Good/Easy UI,
> feature flag, conservative migration/fallback, and regression tests. Treat
> parameter optimization, `fsrs-rs`, rescheduling tools, simulation dashboards,
> mature-card analytics, and broad review UX redesign as later work.
>
> Alpha card identity is also settled: migrate review cards to
> surface-form-in-context cards before attaching real FSRS memory.
>
> This supersedes the launch note of 2026-06-20, which planned to ship the
> fixed-step scheduler (`internal/store/db.go::nextAlphaStepScheduleForRating`)
> through public alpha and treat FSRS as post-launch. The 2026-07-03
> product-readiness grill (Decision 23) moved narrow FSRS into the public-alpha
> launch bar; as of 2026-07-04 it is implemented behind a default-off flag.

## Recommendation

Ship narrow FSRS scheduling for the public alpha, after the surface-card
identity migration (Decision 23).

Today the app still runs the deterministic alpha step scheduler
(`nextAlphaStepScheduleForRating`) with the same Again / Hard / Good / Easy
rating surface. For the alpha migration, use the FSRS algorithm at the product
level, prefer the official Go implementation for the app runtime, and keep
`fsrs-rs` as the future optimization path.

## Alpha FSRS Scope

After the surface-card identity migration, the alpha scheduler migration should
replace the hand-rolled step scheduler with real FSRS without turning "FSRS" into
a broad review-system rewrite.

Ship for alpha:

- Go runtime scheduling with default FSRS parameters.
- Current four-button rating UI: Again, Hard, Good, Easy.
- `next_due`, `last_answer_at`, and `introduced_at` preserved for queueing and
  reporting.
- Versioned FSRS state in `card_state` (explicit columns or `fsrs_json`).
- Feature flag with fallback to the current step scheduler until cutover.
- Conservative migration for existing rows: `NULL` state becomes a new FSRS
  card; legacy `Step`/`Streak` JSON is converted from the best available
  `last_answer_at` / `next_due` evidence without pretending precision.
- Tests for deterministic scheduling, due queue order, daily new-card limits,
  API rating behavior, and legacy migration.

Defer until after alpha:

- Personal FSRS parameter optimization.
- `fsrs-rs` in the production request path.
- Rescheduling tools and simulation dashboards.
- Derived retained/learning coverage views based on FSRS memory state.
- Full review UX redesign.

Why narrow scope:

- The current Review product already exists and should not keep a misleading
  step scheduler into public alpha.
- Default-parameter FSRS improves scheduling quality without blocking launch on
  optimization research.
- The known-vocabulary/card identity model is moving surface-first. FSRS memory
  should attach to those stable surface-form card IDs, not to temporary
  lemma/POS review identity.
- Existing alpha users can be migrated conservatively; there is no need to
  derive perfect FSRS memory from old hardcoded intervals.

Why:

- The current backend, API, and persistence layers are already Go- and SQLite-centric.
- Scheduling reviews is not computationally heavy enough to justify adding a second production FFI path.
- The official `fsrs-rs` project is strongest when we want parameter training and simulation.
- The official `go-fsrs` module is a cleaner fit for day-to-day scheduling inside this codebase.

Practical decision:

- Launch runtime scheduler: `internal/store.nextAlphaStepScheduleForRating`
- Target runtime scheduler: `github.com/open-spaced-repetition/go-fsrs/v3`
- Future offline optimizer: `open-spaced-repetition/fsrs-rs`

### Implemented FSRS state model (2026-07-04)

FSRS lives in `internal/store/fsrs.go` behind `FINNESTDB_FSRS_ENABLED`
(default OFF). Both schedulers share the existing `card_state.fsrs_json`
column, distinguished by a version discriminator so a flag flip — or a
rollback — never corrupts a card:

- **Legacy step payload:** `{"step":N,"streak":M}` (no `v` field), written by
  `nextAlphaStepScheduleForRating`.
- **FSRS payload:** `{"v":2,"fsrs":{...go-fsrs Card...}}`, written by
  `FSRSScheduleForRating`.

`next_due`, `last_answer_at`, and `introduced_at` keep their meaning
regardless of which scheduler wrote the row, so the due queue, daily new-card
limit, and dashboard reporting are scheduler-agnostic.

**Lazy migration on first rating** (only when the flag is ON and the card has
no FSRS payload yet — there is no bulk `card_state` rewrite):

- `NULL`/empty state → a fresh FSRS *new* card; FSRS initializes
  stability/difficulty from the first rating.
- Legacy `step`/`streak` with a prior review → a conservative *Review*-state
  seed. Stability is set to the observed interval (`next_due − last_answer_at`,
  in days, clamped to ≥ 1), difficulty to the default initial value, and reps
  to the legacy streak. This uses the only real evidence the step scheduler
  left behind (the interval) without pretending FSRS-quality history.

**Rollback semantics** (flag turned OFF after FSRS touched a card): the step
scheduler still answers the card. Because an FSRS payload carries no
`step`/`streak`, the step path approximates a step from the current interval
(`next_due − now`) bucketed onto the step ladder, so the card keeps its
earned progress instead of snapping back to step 0.

## Product Model

### Core entities

- `deck`
  - A media object in a recursive hierarchy.
  - Can represent a top-level show/series, a season, or a studyable leaf like a book, movie, or TV episode.
  - Should carry its own `content_type` so media can be searched and filtered.
  - Only studyable leaf decks have parsed sentences and lemma occurrences.
  - Can be either a shared catalog deck or a private user-owned study deck.

- `card`
  - A user-level learning item keyed to the learner-visible
    surface-form-in-context unit, with resolved sense and lemma/POS/dictionary
    entries linked as derived support.
  - Current implementation (2026-07-04): keyed by
    `(user_id, lang, surface_norm, lemma, pos)` where `surface_norm =
    lower(trim(surface))`. `(lemma, pos)` is the sense discriminator, so
    homographs sharing a surface are distinct sense cards and are not collapsed;
    MWE cards carry `surface_norm = ''` and key on `mwe_id`.
  - Global across all decks in the same language when the same surface-card
    identity appears again.
  - Carries a definition, supporting dictionary evidence, and an example
    sentence when available. The review payload adds a homograph note when the
    learner has another card sharing the surface under a different sense.

- `new-card source`
  - The active source for new-card introduction.
  - A deck linked to the selected study-list entry.
  - Can be a studyable leaf deck or a parent deck that aggregates descendant leaf decks.
  - Determines which new lemmas are eligible to be introduced.

- `study list`
  - The ordered set of deck references a user has explicitly added for ongoing study.
  - Drives the user's study page and quick access to active decks.

- `study list entry`
  - A user-specific record that points at a deck.
  - Stores per-user list membership and ordering.
  - Should point only at a private user-owned study deck.

Relationship rule:

- The user selects a `study_list_entry` on the study page.
- That entry resolves to one underlying `deck`.
- That deck becomes the `new-card source` for the session.

Ownership rule:

- Shared catalog decks are used for search and browsing.
- Private user-owned study decks are used for the user's study list and sessions.
- Selecting media from the catalog creates a private study deck for that user.
- Deleting a study-list entry deletes its linked private study deck.

Access-control rule:

- The authenticated user ID should come from server-side session or auth context, never from client input.
- Shared catalog decks (`owner_user_id IS NULL`) can be returned through catalog deck endpoints.
- Private decks (`owner_user_id IS NOT NULL`) must only be returned when `owner_user_id == authenticated_user_id`.
- This ownership check applies anywhere a deck is fetched by ID, including stats and child-navigation endpoints.

### Important distinction

Keep `content selection` separate from `card identity`.

- Content decides which new words are available.
- FSRS decides when already-added cards are due.
- A review session should mix:
  - due cards from the user's existing global card set
  - new cards drawn from the currently selected new-card source

## Proposed schema direction

The current schema already has good primitives for deck-backed parsed content,
cards, card state, and lemma-backed known state. As of the 2026-07-03 product
readiness pass, the product direction for known vocabulary is **surface-first**:
store the exact forms a learner says they know, then resolve lemma/POS as
derived evidence for coverage, filtering, and cards. The main missing pieces are
deck hierarchy support, aggregated frequency tables, richer card content fields,
surface-form card identity, and a known-word model that preserves submitted
surface forms.

### Keep

- `decks`
- `sentences`
- `occurrence`
- `cards`
- `card_state`
- `user_known_lemmas` as the current implementation-backed table during the
  transition
- `user_ignored_lemmas`

### Extend

- `decks`
  - add `parent_deck_id` nullable
    - `parent_deck_id` points to the parent deck in the hierarchy
  - add `content_type`
    - examples: `book_series`, `book`, `tv_show`, `tv_season`, `tv_episode`, `movie`, `article`
  - add `owner_user_id` nullable
    - `NULL` means shared catalog deck
    - non-`NULL` means private user-owned study deck
  - add `source_deck_id` nullable
    - points back to the shared catalog deck a private study deck was created from

- known-word state
  - add a surface-first known-vocabulary table, for example
    `user_known_forms(user_id, lang, form, source, confidence, created_at)`
  - store resolved `(lemma, pos)` links as derived rows or cache columns, not as
    the only source of truth
  - keep `user_known_lemmas` until deck filtering and coverage have migrated

- `cards`
  - add `lang`
  - migrate primary review identity from lemma/POS to a stable
    surface-form-in-context card identity before FSRS cutover
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

- `study_list_entries`
  - `user_id`
  - `deck_id`
  - `added_at`
  - `sort_order`
  - Unique on `(user_id, deck_id)`

- `user_language_settings`
  - `user_id`
  - `lang`
  - `review_order_mode`
  - Unique on `(user_id, lang)`

Recommended semantics:

- A private study deck appears on the user's study page only after a `study_list_entry` is created for it.
- Adding a parent deck from the catalog creates a study-list entry for that parent deck only, not separate entries for all descendants.
- A user can later add a child deck separately if they want a more specific new-card source.
- `sort_order` stores the user's manual study-list order. The deck with sort_order == 0 is the one that will be used for new words.
- `review_order_mode` a user setting unique to each language telling when new words are introduced: `new_first`, `mixed`, or `new_last`.
- Deleting a study-list entry should delete the associated deck if it's a private deck, but not if it's a public deck.
- The UI should show a deletion confirmation dialog before deleting a study-list entry.

Recommended dashboard behavior:

- study-list entries should be shown in the user's manual `sort_order`
- show study-list title, content type, and coverage summary
- for parent decks, show child counts and aggregate coverage

### Deck acquisition

Users should be able to get a deck into their study list in two ways:

1. select an existing deck from the media catalog
2. import their own text and create a new deck

These are different intake paths, but they should converge on the same `decks` and `study_list_entries` model.

#### Path 1: Select existing media

Flow:

1. search top-level decks
2. open a deck detail page
3. choose `Add to study`
4. a `study_list_entry` is created for selected catalog deck

Behavior:

- If the selected deck is a parent deck, it can still be added directly and used as an aggregate new-card source.
- If the selected deck is a studyable leaf deck, it is added directly and studied as-is.

#### Path 2: Import text

Flow:

1. user pastes or uploads text
2. user provides minimum metadata
3. system parses the text into sentences, occurrences, and lemma stats
4. system creates a new, private deck
5. the deck is immediately added to study

Required metadata for imported decks:

- `title`
- `lang`
- `content_type`

Recommended imported-deck behavior:

- imported decks should be root decks by default
- imported decks should be directly studyable
- imported decks should be private to the creating user
- imported decks should appear on that user's study surfaces
- imported decks should not appear in public or shared catalog search results
- imported decks should carry an ownership/visibility flag so catalog search can exclude them by default

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

We need four different states, not one:

- `known`
  - The user already knows the relevant vocabulary unit. Product direction is
    surface-first; current implementation may still derive this from
    lemma-backed state.
  - Comes from explicit learner evidence: manual marking, import, test-out, or
    marking a review card known. FSRS maturity must not silently write known
    state.
  - For ambiguous context-free imports, the surface can be known while the
    specific surface+sense remains unconfirmed until a contextual meaning check.

- `studying`
  - The vocabulary unit has a card and is in FSRS, but is not yet considered
    known.

- `review_maturity`
  - A derived scheduler state from review history, such as learning, retained,
    or mature. This may inform due dates and optional comprehension estimates,
    but it is not the same as explicit known vocabulary.

- `ignored`
  - The user does not want this vocabulary unit in study.

### Recommendation

Do not treat "has a card" or "has a mature FSRS state" as "knows the word."

That would overstate comprehension badly, especially for high-frequency words introduced recently.

## New card selection

When the user studies a selected deck, new cards should come from:

`eligible lemmas in source`
minus `known vocabulary coverage`
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

### 1. Unique vocabulary coverage

How many unique vocabulary units in the source the user knows.

Formula:

Current implementation:

`known_unique_lemmas / total_unique_lemmas`

Target surface-first model:

`known_unique_surface_occurrences / total_unique_surface_occurrences`

### 2. Token-weighted coverage

How much of the source the user knows when repeated words count more.

Formula:

Current implementation:

`sum(token_count for known lemmas) / sum(token_count for all lemmas)`

Target surface-first model:

`sum(token_count for known surface forms) / sum(token_count for all surface forms)`

This is the better proxy for "how much of this content can I understand?"

Current learner-facing coverage and count denominators exclude quarantined
content (shipped 2026-07-04, Phase 1c). When an admin globally quarantines a
faulty occurrence/card via its correction issue, deck word counts, due counts,
new-card counts, token-weighted coverage, and next-unlock projections act as if
that content is not currently studyable. Historical/admin audit views may still
include the quarantined rows with their correction-issue metadata.

> **Implementation notes (2026-07-02, shipped as `GET /api/decks/{id}/comprehension`
> and `comprehension_pct` on the deck list):**
>
> - **Ignored lemmas count as covered.** "Ignore" means "don't make me study
>   this" (typically proper names); coverage is a reading-comprehension proxy,
>   not a study queue, and a name the user chose to skip should not depress
>   their percentage.
> - **Token identity is a position, not a row.** Multi-lemma homonym expansion
>   stores one occurrence row per candidate; a token position counts as covered
>   when ANY of its candidates is known.
> - **Deck expansion stays dictionary-driven.** The candidates a deck save
>   expands come from `store.BatchLookupAllForms` (dict-only, filtered by
>   `filterLowValueAlternatives`), so deck word counts are stable and cards are
>   well-glossed. The FST-known homograph senses merged for the meaning-check
>   candidate set (2026-07-04, parser `2026.05.15b`;
>   `BatchLookupAllFormsWithOptions{MergeFSTReadings}`) are deliberately gated
>   off this path — they raise ambiguity offerability without inflating decks
>   with no-gloss or inflectional-form cards. See
>   [`FEATURES.md`](FEATURES.md) "Words With Multiple Senses".
> - **Coverage is lemma-level** for v1; form-level display is a possible later
>   toggle.

### 3. Comprehension prediction (before/after)

Beyond showing current coverage, the UI should project how coverage changes
as the user learns more words. For a given deck:

- "You currently understand ~72% of this content"
- "Learning the top 20 new words brings you to ~85%"
- "Learning the top 50 brings you to ~91%"

This is computed by iterating through the ranked new-card list and
cumulatively adding each word's token_count to the known total.

This before/after framing is a strong motivator for learners (see Surasura's
comprehension prediction UX for prior art).

### 4. Cross-deck comprehension gain

When a user has multiple decks in their study list, a word may appear in
several of them. The marginal comprehension gain of learning a word is the
sum of token_counts it unlocks across all active decks, not just the
currently selected source.

Formula for cross-deck gain of a candidate lemma:

`sum(token_count for lemma across all study-list decks where lemma is unknown)`

This can serve as an alternative ranking mode for new-card selection (see
§New card selection). It answers: "which single word would unlock the most
text across everything I'm studying?"

## What counts as "known" for coverage

This needs a rule. Recommended MVP rule:

- current implementation: `known` if the lemma is in `user_known_lemmas`
- target model: `known` if the learner has explicitly known surface-form
  evidence for this occurrence, or if derived lemma coverage is intentionally
  accepted for the language/feature class
- FSRS maturity is a separate derived state. It can support additional
  `retained coverage` or `learning coverage` views, but it must not silently
  convert into known-word evidence.

Recommended retained-coverage threshold for MVP:

- interval at least 21 days

Alternative later:

- use FSRS memory state directly, such as a minimum stability threshold, for a
  derived retained-coverage view
- expose multiple views: `explicit known coverage` vs `retained coverage`

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

A surface-form card should be global for the learner when the same review unit
appears across decks, but it should still carry study content on the card itself
and show deck-specific context when available. Lemma/POS/dictionary entry data is
supporting metadata, not the alpha card's primary identity. For homographs and
multi-lemma surfaces, use a separate sense-aware surface card when
parser/dictionary evidence supports distinct meanings.

When a quarantined card/content issue is fixed, the shipped alpha behavior
(2026-07-04, Phase 1c) restores the existing review item and returns it to
circulation with its existing `card_state` scheduler state and due history
preserved — restore is a status flip, no scheduler reset. Creating a new card
for a changed learning target identity (wrong lemma/POS, wrong sense, homograph
split, phrase/MWE replacement, or invalid target retirement) and rendering
corrected content remain future overlay work. Past review history is never
rewritten to pretend the learner saw the fixed content earlier.
Reset/reintroduce scheduling only for a new learning target
identity.

Minimum card payload:

- surface form
- resolved sense key when needed for homographs
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
- If the active new-card source contains a better example for the same
  surface-card identity, the session payload can override the saved example for
  display without changing the card's saved default state.
- If the same-looking surface has another supported meaning, the card should say
  so directly and point to the distinction, such as noun versus verb form.
- Parsed deck sentences should not automatically become part of a shared example corpus.

Recommended review payload:

- surface form
- resolved sense or disambiguation label when needed
- lemma
- part of speech
- definition text
- one example sentence from the active new-card source if available, otherwise the card's saved example
- homograph note when another supported card has the same surface
- source label (`Book 1`, `Episode 3`, etc.)
- optional alternate example from another deck where the surface-card appears

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

## Known-word bootstrap via external import

Coverage metrics and study sequencing are only useful if the user's known-word
state reflects reality. A returning learner who already knows 2,000 Finnish
words will see 0% comprehension until those words are marked known.

### Recommended import sources

- **AnkiConnect**: already implemented in the web app. Pull selected decks from
  a local running Anki desktop collection, extract chosen fields, clean common
  textbook notation, and submit surface strings through `/api/known-words`.
- **Anki `.apkg` upload**: future offline path. Extract front-field text and
  send it through the same known-word import pipeline.
- **CSV/TSV/text file**: already implemented as one word per line or first-column
  import for `.txt`, `.csv`, `.tsv`, and `.md`. Add clearer learner guidance;
  decide separately whether first-column parsing is enough or whether quoted CSV
  needs a real parser.
- **Bulk mark from parse results**: let the user select words from a parse
  result and mark them as known in bulk (this is already somewhat possible
  via the UI but should be a first-class flow)

### Ambiguous imported known words

Do not ask learners to resolve every ambiguous imported surface during import.
That would turn onboarding into a disambiguation task.

Import behavior:

- Store the submitted surface as known evidence.
- If the surface has one supported sense, it can resolve directly to that
  surface+sense for filtering and comprehension.
- If the surface has multiple supported senses, mark the sense as needing lazy
  confirmation.

Meaning-check behavior:

- Trigger only when the ambiguous surface appears in a useful sentence context,
  such as parse results, deck creation, review, or test-out.
- Parse results should carry enough ambiguity metadata for this flow:
  candidate meanings, selected candidate when available, and parser confidence.
  Parser confidence means measured contextual sense-selection confidence, not
  learner knowledge.
- Confidence thresholds must be calibrated against eval slices before they
  simplify the learner UI. Start Finnish-first with contextual homographs such
  as `kuusi` (six/spruce), `tuli` (came/fire), and `voi` (can/butter), then add
  Estonian parity cases.
- High-confidence branch:
  - Show the sentence, the intended meaning, and the same-looking alternative
    meaning.
  - Primary action: `I know this meaning` records the surface+sense as known and
    skips the review card for that sense.
  - Secondary action: `Study this meaning` must include helper copy that matches
    the current context.
    - In parse results before a deck exists: `Creates a review card when you save`.
      It marks this surface+sense for study in the pending save/add-to-deck
      payload; no card exists if the learner never saves the parse.
    - In a saved deck, test-out, or review: `Creates/keeps a review card`. It
      creates or keeps the review card for that surface+sense.
- Low-confidence branch:
  - Do not ask `Do you know this meaning?` as if the parser knows the intended
    sense.
  - Show `Multiple possible meanings` with the candidate meanings.
  - Each candidate has `I know this meaning` and `Study this meaning` actions
    with the same context-aware helper text as above.
  - Add `None of these looks right` as parser feedback. This is not a study
    choice; it reports that the app's analysis/candidate list is wrong.
- `Not sure` should behave conservatively like study: keep/create the review
  card.

### Import endpoint

Current endpoint:

`POST /api/known-words`

Earlier draft endpoint:

`POST /api/import/known-words`

Input: `{lang, source_type, data}` where `data` is either a file upload or
a JSON array of `{form}` entries.

Behavior:

- Resolve each form against the dictionary using the existing fallback chain
- Current code inserts resolved `(lemma, pos)` pairs into `user_known_lemmas`.
- Target behavior should persist the submitted surface forms first, then store
  resolved `(lemma, pos)` evidence as derived/cached data.
- Return a summary such as
  `{imported_surfaces, confirmed_single_sense, needs_sense_confirmation, skipped_unknown, skipped_duplicate}`.
- Do not create cards during import for ambiguous known surfaces. In parse
  results, `Study this meaning` only marks the pending deck-save payload. Create
  or keep a card when the learner saves/adds the deck, or immediately in an
  already-saved deck/review/test-out context.
- `None of these looks right` should use parser feedback. Current code requires
  proposed lemma/POS; the alpha target needs the planned flag-only feedback
  shape with nullable proposed fields and `flag_only=true`.

## Suggested API shape

### Content and stats

- `GET /api/decks`
  - top-level search and filter endpoint
- `GET /api/decks/:id/stats`
  - must enforce private-deck ownership before returning data
  - learner-facing stats exclude globally quarantined content
- `GET /api/decks/:id/children`
  - must enforce private-deck ownership before returning data

### Study list

- `GET /api/study/decks`
  - returns the user's study-list entries with deck summaries

- `POST /api/study/decks`
  - input: `{deck_id}`
  - creates a study-list entry for a deck

- `DELETE /api/study/decks/:deck_id`
  - removes the study-list entry for a deck
  - if the deck was created by the user, the deck is also deleted
  - the client should require explicit user confirmation before calling this endpoint

- `PATCH /api/study/decks/reorder`
  - input: `[{deck_id, sort_order}]`
  - rewrites the user's full study-list order in one transaction

### Language settings

- `GET /api/settings/languages/:lang`
  - returns user language-level study settings

- `PATCH /api/settings/languages/:lang`
  - updates language-level settings such as `review_order_mode`

### Import

- `POST /api/import/decks`
  - input: `{title, lang, content_type, text}`
  - parses imported text, creates a new private deck, and adds it to the user's study list

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

- Migrate review card identity to stable surface-form cards — **done
  (2026-07-04)**
- Integrate narrow runtime FSRS scheduling for alpha — **done behind
  `FINNESTDB_FSRS_ENABLED`, default off (2026-07-04)**
- Attach scheduler state to those stable surface-card IDs — **done: FSRS state
  lives in `card_state.fsrs_json` keyed to the surface-card id (2026-07-04)**
- Support due reviews plus source-scoped new cards

### Phase 3

- Add derived retained/learning coverage views from FSRS maturity
- Keep retained/learning coverage as derived views, not writeback into
  known-word state
- Add richer progress dashboards
- Evaluate whether offline FSRS parameter optimization is worth the added complexity

## Open decisions

- Exact schema shape for sense-aware surface-card keys — **resolved &
  implemented (2026-07-04)**
  - Resolved direction: alpha review cards are surface-form-in-context cards
    keyed by normalized surface plus resolved sense when parser/dictionary
    evidence supports distinct meanings. Do not collapse clear homographs into a
    pure surface key, and do not create one permanent card per occurrence.
  - Implemented: the DB stores the sense key as `(lemma, pos)`, so the card key
    is `(user_id, lang, surface_norm, lemma, pos)`. A parser candidate ID or an
    explicit sense table remains possible future work, but is not needed for the
    alpha: `(lemma, pos)` distinguishes the homographs the parser/dictionary can
    already tell apart.

- Whether to support arbitrary user-made bundles later
  - Recommendation: not in MVP; start with canonical parent/child deck hierarchy only.

- Whether coverage should include ignored lemmas in the denominator
  - Recommendation: yes, because ignored is not known.

- Whether proper nouns should enter study
  - Recommendation: allow them to be filtered out at the deck level, not hard-coded globally.

## Current recommendation in one sentence

Use FSRS for alpha, but narrowly: keep scheduling in Go with default parameters
and a migration/fallback path, do not wire `fsrs-rs` directly into production,
and do not bundle scheduler migration with parameter optimization or a broad
review redesign. Migrate review card identity to stable surface-form cards first,
then attach FSRS state to those stable card IDs.
