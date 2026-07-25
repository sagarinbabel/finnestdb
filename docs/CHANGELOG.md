# Documentation Changelog

This file tracks notable changes to FinnEst planning, architecture, and
product documentation. Code changes belong in git history, not here.

Entries are reverse-chronological. Each entry links to the docs it
introduced or modified so the docs index stays navigable.

**Cross-reference convention:** CHANGELOG records what changed; [`DECISIONS.md`](DECISIONS.md)
records why we chose to change it that way. Where the same event appears
in both files, both entries cross-link.

## 2026-07-04 - Docs: anonymous cap default corrected to 300,000 characters

The runtime default for `FINNESTDB_ANON_MAX_CHARS` was raised from 20,000 to
300,000 characters (commit 7bff399) shortly after the anonymous demo shipped,
but the current-state docs kept the launch-era 20,000 value (flagged by the
2026-07-04 full-app walkthrough audit). Corrected the docs that describe
current behavior; older entries below record the value that shipped at the
time and are intentionally unchanged. The load-test progress note in
`TODO.md` now also records that the 300,000 default has not itself been
load-tested yet.

- Modified: [`FEATURES.md`](FEATURES.md) - Anonymous Parser Demo cap default.
- Modified: [`USER_FLOWS.md`](USER_FLOWS.md) - anonymous cap default.
- Modified: [`CONTEXT.md`](../CONTEXT.md) - Anonymous Parser Demo vocabulary
  entry.
- Modified: [`TODO.md`](../TODO.md) - demo/abuse-control bullets and the
  load-test progress note.
- Modified: `web/app.ts` - stale fallback-comment default (the code constant
  was already 300,000).
## 2026-07-04 - First-experience RC pack completed: zero pending journeys

Follows up the "First-experience RC pack skeleton" entry below: every
journey the skeleton marked `automation:"pending"` now has real automated
coverage, because the underlying features (anonymous parser demo, embedded
catalog, ambiguity meanings panel, known-word import) shipped earlier the
same day. `make first-experience-rc` now runs the full 18-case manifest with
zero pending skips.

- Modified: `testdata/first-experience-rc/manifest.json` - flipped
  `fi/et-anonymous-demo`, `fi/et-known-word-import`, and `et-parser-feedback`
  from `automation:"pending"` to `automation:"playwright"` with honest
  `expect` blocks (including a new `known_lemma_pos` field for the
  known-word-import cases). Added cross-reference notes to the
  `ambiguity-homograph` cases pointing at their new Playwright UI-level
  coverage. No case remains `automation:"pending"`.
- Modified: `web/tests/first-experience-rc.spec.ts` - implemented one
  Playwright journey pass per newly-flipped case, reusing existing spec
  conventions rather than duplicating their detailed coverage:
  - `anonymous-demo` (FI+ET): unsigned paste on the landing form using the
    manifest's own embedded-text fixture, parse, then explore via the Words
    tab, asserting signed-in-only controls (save-as-deck, corrections, known
    pills) stay absent. Detailed landing/export behavior stays in
    `anonymous-parser-demo.spec.ts` / `landing-prototype.spec.ts`.
  - `known-word-import` (FI+ET): pastes the manifest's fixture word list on
    `/#/vocab` and asserts every fixture word resolves to the manifest's
    expected (lemma, POS). Detailed import/remove/unresolved-word coverage
    stays in `parse-results.spec.ts`.
  - `parser-feedback` (ET parity): the FI correction-submit test was
    generalized into a parametric FI/ET helper driven by per-language
    fixtures instead of duplicating the flow, closing the FI-only gap.
  - `ambiguity-homograph` (FI+ET, new UI-level pass alongside the existing
    Go parser-quality cases): parses a mocked ambiguous-surface payload for
    the journey's homograph pair, opens the "Multiple possible meanings"
    panel, and asserts >=2 candidates plus the flag-only "None of these
    looks right" escape. Deliberately does not grade which candidate is
    correct - that is the Go runner's `lemma_pos` assertions and the eval
    slice's job (`PARSER_EVAL_METHODOLOGY.md`).
- Modified: `docs/GO_LIVE_CHECKLIST.md` and `TODO.md` - recorded the
  completed automated-coverage state; the manual product walkthrough and the
  go/no-go call remain the outstanding human step.
- `cmd/firstexperiencerc` (the Go runner) is unchanged: no manifest field it
  parses changed shape, only which cases carry `automation:"playwright"`.

## 2026-07-04 - Landing: port the Claude Design "Aalto edition" prototype

Re-skinned the anonymous landing to the Claude Design prototype
(`design/aalto-landing.jsx`) after the owner's verdict that "nothing has been
carried through from the design prototype." The landing now leads with the serif
hero **"Paste your _Suomi_ or _Eesti_. Lift the words out."** (italic-blue
Suomi/Eesti), the owner-override subtitle, a truthful `FREE · NO ACCOUNT · NO
HISTORY SAVED` eyebrow with a live pulse dot, the birch-lined paste box (`⌘↵`
hint, char meter against the real anon cap), the "or try →" demo chips, the
three-cell freemium band, and the Aalto decorations (vertical Alvar-Aalto
wordmark, a slowly drifting Savoy-vase silhouette that respects
`prefers-reduced-motion`, colophon with palette swatches). See
[`USER_FLOWS.md` §1](USER_FLOWS.md#1-landing--inline-parse-anonymous).

- **Aalto is now the default skin.** A first-time visitor with no saved choice
  lands on `data-skin="aalto"` · Paimio light; the Ink skin stays selectable in
  the picker and saved choices are honored (only the fallback default flipped -
  one line in `readThemeSkin`/`readThemeMode` reverts it).
- **Anonymous demo texts:** new stateless `GET /api/demo/text/{id}` endpoint
  serves a fixed allowlist of three curated, license-clean embedded fixtures
  (Sauna / Hiiri-Pekka / Linnu keel) without auth. The full `/api/catalog`
  surface stays signed-in only; any id outside the allowlist 404s.
- **Anonymous word-list export:** the results view gains **Copy list** +
  **Download CSV**, generated client-side from the in-memory parse response, so
  freemium cell ii ("Copy or download") is truthful while honoring the anonymous
  ephemeral guarantee. Truth adjustments to prototype copy: cell iii's "Free
  Google sign-in" → "Free sign-up." (OAuth not shipped yet); the "Ephemeral OFF"
  toggle was not ported (anon parses are always ephemeral).
- Tests: updated the landing-copy assertions in `parse-results`,
  `anonymous-parser-demo`, `coverage-reveal`, `reading-surface`, and
  `theme-picker`; added `landing-prototype.spec.ts` (exact hero, eyebrow,
  freemium band, demo-chip loading, anon CSV export, 375 px cleanliness) and Go
  tests pinning the demo-text allowlist endpoint.

## 2026-07-04 - Reading surface: text-first results with tap-to-mark words

Inverted the `/results` page into a **text-first** experience (aha moment #2,
"the living text"). The page now opens on a **Read** tab that renders the source
text with paragraph structure preserved, in reading typography, with every
parsed word as a tappable span colored by its learner state (`--new` /
`--learning` / `--known`; ignored words render neutral). A **Words** tab holds
the existing lemma table with unchanged behavior; the tab choice persists in
`localStorage` (default Read). See
[`USER_FLOWS.md` §2](USER_FLOWS.md#2-inline-results-anonymous) and the new
**Reading Surface** entry in [`CONTEXT.md`](../CONTEXT.md).

- **No new server data:** the Read view tokenizes `state.currentSourceText`
  client-side and matches each surface back to its `WordEntry` via
  `WordEntry.forms` (the exact case-preserved surfaces each `(lemma, pos)`
  resolved from). Homograph surfaces map to more than one row and route to the
  ambiguity popover. No new `/api/parse` fields.
- **Tap popover:** anchored on desktop, a scrollable bottom sheet at ≤440px with
  ≥44px tap targets. Known / Study / Ignore reuse the table's endpoints and the
  chip's pending-deck `selected_senses` model; an ambiguous surface reuses the
  **Multiple possible meanings** panel (`renderAmbiguityPanel` +
  `wireAmbiguityControls`) verbatim, including the "None of these looks right"
  flag-only escape. One popover at a time; ESC / tap-outside closes.
- **Shared chrome:** the summary (parser pill, compact coverage gauge, stats,
  save-as-deck CTA) plus the anonymous sign-up ribbon and privacy footer moved
  above the tabs, so save works identically from either tab. The animated
  coverage reveal is re-homed to the top of the Read tab; the compact gauge stays
  in the shared summary.
- **Anonymous & decks:** the Read tab renders for anonymous parses in neutral
  coloring with a gloss-only, sign-in-nudged popover. Saved decks carry no raw
  source text, so they keep the Words table with the tab bar hidden.
- Docs touched: [`FEATURES.md`](FEATURES.md) "How You Learn" step 2,
  [`USER_FLOWS.md`](USER_FLOWS.md) §2, [`CONTEXT.md`](../CONTEXT.md).

## 2026-07-04 - Post-parse coverage reveal (aha moment #1)

Added an **animated coverage reveal** above the Inspect/anonymous results table:
the first thing a learner feels after a parse. For signed-in learners it counts
up to "You already know **X%** of this text" and projects "Learn the top **N**
words → **Y%**"; for anonymous visitors it reframes as "The **N** most frequent
words in this text carry **Z%** of it" (projection-from-zero, doubling as the
sign-up hook the existing ribbon follows).

- **Number source:** all figures reuse the saved-deck comprehension token-mass
  formula (`store.DeckComprehension`) - a token position counts as covered when
  its (lemma, pos) is **known OR ignored**, weighted by occurrence count. The
  projection ranks the parse's unknown lemmas by token mass exactly as
  `DeckComprehension`'s top-unlocks SQL does. Computed client-side from the
  fields `/api/parse` already returns (`count`, `learning_state`); no new wire
  fields. Copy is hedged with `≈` when a whole-percent hides a fraction, and
  carries no exclamation marks.
- **Motion:** ~1.2s ease-out count-up + two-segment bar fill;
  `prefers-reduced-motion` collapses to the final state instantly. No animation
  libraries. Built as a self-contained unit (`renderCoverageReveal` +
  `.coverage-reveal` CSS block) so the queued reading-surface redesign can
  re-home it cheaply.
- **Docs:** [`USER_FLOWS.md` §2](USER_FLOWS.md) documents the reveal as part of
  the results moment; [`FEATURES.md`](FEATURES.md) "Current Inspect Results"
  now lists it.
- **Tests:** `internal/api` guards the parse-response contract the reveal
  depends on (per-word `count` + known/ignored `learning_state`, ignored counts
  as covered); `web/tests/coverage-reveal.spec.ts` asserts the signed-in and
  anonymous reveals render plausible numbers, the count-up settles on the
  API-derived value, and the reduced-motion path collapses instantly.
## 2026-07-04 - Smart display titles for pasted-text parses and decks

Raw pastes used to get a useless deck-name default (`"Finnish: <first 48
chars>"`) or a raw 240-char `source_preview` dump in History. Added
deterministic **title derivation** so a pasted paragraph displays as nicely as
a curated catalog text, without an LLM call per paste.

- **`store.DeriveTitle(sourceText, lang string) string`**
  ([`internal/store/titles.go`](../internal/store/titles.go)): first
  clause/sentence, cleaned of surrounding whitespace/quotes/markdown
  artifacts, cut at a sentence end (`. ! ?`, URL/abbreviation/decimal periods
  excluded via a "followed by space or end-of-string" check) or clause
  boundary (`, ; :`) closest under 60 chars, with an ellipsis only when
  truncated mid-clause. Degenerate input (one word, a bare URL, digits-only)
  falls back to the first 2-4 words; empty input falls back to
  `DefaultTitleForLang` ("Untitled Finnish text" / "Untitled Estonian text").
  Table-driven tests in
  [`internal/store/titles_test.go`](../internal/store/titles_test.go) pin the
  59/60/61-char boundary and Finnish/Estonian diacritics.
- **Deck save** (`POST /api/decks`): a blank `title` no longer 400s - the
  server derives one from `text`/`lang` via the same rule, so the API
  contract stays honest for non-browser callers
  ([`internal/api/handlers.go`](../internal/api/handlers.go) `handleCreateDeck`).
  The save modal still prefills the suggestion client-side (a TS port of
  `DeriveTitle` in [`web/app.ts`](../web/app.ts)) so the learner sees and can
  edit it before saving - the server-side derivation is the fallback for a
  cleared/omitted field, not the primary UX path.
- **Parse-session History**: `parse_sessions` has no title column, so
  `ListUserParseSessions` derives a `title` field at read time from a
  newline-preserving head of `source_text`, alongside the existing flattened
  `source_preview` (kept for other consumers). History rows now show the
  derived title instead of the raw truncated preview. Parse-session titles
  are derived-only for alpha - no rename plumbing; deck rename already
  existed and still works.
- **Docs modified:** [`USER_FLOWS.md`](USER_FLOWS.md) §5 (Parse, signed-in)
  now points at the shipped History behavior instead of only the proposed
  spec.
## 2026-07-04 - Aalto skin (Paimio / Sanatorium) as an opt-in second skin

Added a second visual **skin** alongside the default INK/PAPER "Ink" look:
**Aalto**, an Alvar-Aalto-inspired skin whose light mode is **Paimio** (warm
birch-cream paper, soft Nordic blue) and dark mode is **Sanatorium** (deep
blue-black ground). Theming is now two-dimensional - skin (`data-skin`:
`ink` | `aalto`) crossed with mode (`data-theme`: `light` | `dark`), both on
the root element. The old single 🌓 toggle is replaced by a nav **theme
picker** offering Ink · Light / Ink · Dark / Aalto · Paimio / Aalto ·
Sanatorium, persisted in `localStorage` (`skin` + `theme` keys); the default
is unchanged (Ink + the user's saved mode), so Aalto is strictly opt-in. Fonts
Newsreader + Inter Tight were added; under `data-skin="aalto"` the font/colour
role-tokens switch to the prototype's values (copied verbatim from
[`design/claude-design/finnest-prototype.html`](../design/claude-design/finnest-prototype.html)).
Introduced word-status role-tokens `--known` / `--learning` / `--new` under
both skins for the upcoming reading surface. Documented the token mapping and
prototype pointer in [`DESIGN_AI_PROMPTS.md`](DESIGN_AI_PROMPTS.md) ("Aalto
skin"). CSS-first skin, no layout/markup restructuring beyond the picker
control; both skins verified at 375 px with no horizontal overflow.

## 2026-07-04 - FSRS enabled by default after staging validation

Made **FSRS the default review scheduler** after the documented staging gate came
back green. The `FINNESTDB_FSRS_ENABLED` flag flips from opt-IN to **opt-OUT**:
unset → FSRS on (the shipped default); `0`/`false`/`no`/`off` → the deterministic
step scheduler, which is now the rollback fallback rather than the runtime
default.

- **Validation harness:** [`internal/store/fsrs_validation_test.go`](../internal/store/fsrs_validation_test.go)
  runs the DEPLOYMENT.md "FSRS scheduler rollout" gate as in-suite Go tests on
  temp DBs (the shared `finnestdb.db` is never written): seeded-history
  validation across new/learning/mature/legacy/NULL shapes, 1k-card lazy-migration
  scale check, a rollback round trip, and a read-only real-DB smoke (4031 sampled
  real cards, 0 corrupt). All green - see the report below.
- **Report added:** [`launch-readiness/2026-07-04-fsrs-validation.md`](launch-readiness/2026-07-04-fsrs-validation.md)
  records each drill's result in tables (interval ordering per button, monotonic
  stability growth, real-DB shape distribution).
- **Flag semantics:** `store.FSRSEnabled()` is now opt-out; the flag-off
  byte-identical regression pin
  (`TestRecordReviewAnswerFlagOffByteIdenticalToStepScheduler`) still guards the
  rollback path, and a new `TestFSRSEnabledDefaultsOnOptOut` pins the default-on
  parsing.
- **Docs modified:** [`DEPLOYMENT.md`](DEPLOYMENT.md) (flag table + rewritten
  rollout/rollback section), [`srs-deck-spec.md`](srs-deck-spec.md)
  (current-scheduler statements + Implemented FSRS state model),
  [`../CONTEXT.md`](../CONTEXT.md) ("Alpha Step Scheduler" is now the fallback),
  [`../TODO.md`](../TODO.md) ("Review readiness" gate ticked with the report
  pointer).

Rollback remains a single flag flip (`FINNESTDB_FSRS_ENABLED=0` + restart), no
data migration; FSRS and step state coexist in `card_state.fsrs_json` via the
version discriminator, and FSRS-touched cards keep their earned progress on
rollback.
## 2026-07-04 - Starter-deck cards carry curated corpus example sentences

The cold-start "Top N words" official starter deck now attaches a real corpus
example sentence to each card instead of showing only the bare headword form.

- **New tool `cmd/pickexamples`:** for each lemma in seedcolddeck's Top-N
  ranking (shared via the new `internal/starterdeck` package), it indexes
  candidate sentences from the corpus pipeline's per-surface-form example index
  (`wordlist_user_friendly.tsv`'s `example_ref_id`) - not a 66M-line text scan -
  then fetches just the needed sentences in one streamed pass over
  `sentences_user_friendly.tsv`. Both passes are bounded-memory streaming scans;
  a full FI/ET run is ~20s at ~1.1 GB RSS.
- **Deterministic "beautiful evocative" heuristics** (documented as named
  constants in `cmd/pickexamples/select.go`): complete sentence, 4–14 words, no
  digits/URLs/ALL-CAPS/quote fragments/subtitle artifacts (leading dashes,
  speaker colons, dialogue-line joins, OCR mid-word-cap garble), a preference
  for a non-sentence-initial target form and high-frequency surrounding words,
  plus a coarse foreign-language guard against corpus language contamination.
- **Checked-in artifact** `testdata/starter-examples/{fi,et}-examples-v1.tsv`
  (~790 FI / ~764 ET lemmas covered), attributed and licensing-noted in its
  [README](../testdata/starter-examples/README.md) per the owner's
  individual-sentence call.
- **Wiring:** `cmd/seedcolddeck -examples <file>` seeds each matching card with
  the corpus sentence and the inflected form as the highlighted occurrence;
  lemmas without a curated example fall back to the prior representative-form
  sentence. The example reaches the review payload through the existing
  deck-sentence mechanism. See
  [`srs-deck-spec.md`](srs-deck-spec.md) "Example sentence policy".
## 2026-07-04 - Learner-facing copy sells the pre-learn proposition

Rewrote the persuasion copy across every learner-facing surface (landing hero,
subtitle, value grid, anonymous results ribbon, About page, landing/About CTAs,
sign-in lede, and the landing "sign in" link) to lead with the pre-learn
promise - paste any text, see every word with its meaning and inflected-form
frequency, and learn it before you read - instead of flat "sign in to save your
work" framing. Value-grid cards now describe the learner outcome (pre-learn →
read smoothly → remember in context → know how much you'll understand). The
comprehension prediction is presented honestly (per-deck "how much of this text
you already understand"), and the anonymous privacy footer stays byte-identical
and truthful about ephemerality. No layout or feature changes; grill-settled
functional strings (meaning-check copy, quarantine copy, privacy footer) are
unchanged. Updated [`USER_FLOWS.md`](USER_FLOWS.md) §1/§2 mock copy to mirror the
shipped UI and the Playwright copy assertions that pin these strings. Copy pass
only - no product behavior changed.
## 2026-07-04 - 375px results-table layout repaired

Fixed BROKEN mobile layouts found in a 375x812 audit of every learner surface
(landing, results, signup, dashboard, embedded catalog, Inspect, decks,
review, vocab, history, feedback/ambiguity modals). Scope was limited to
BROKEN findings only (clipped/unreachable controls); cosmetic-only spacing
was left alone per [`FEATURES.md`](FEATURES.md) Mobile Direction's usable-at-375px bar.

- **Results table (`web/styles.css` `@media (max-width: 600px)`):** `.col-actions`
  carried a desktop `min-width: 13rem` (208px) that, combined with the fixed
  `.col-lemma`/`.col-count` widths, pushed the row past 500px even after
  `.col-def`/`.col-grammar` were hidden - so the Known/Ignore/Suggest-fix
  controls (`.word-pill-known`, `.word-icon-ignore`, `.correction-btn`) were
  scrolled off the right edge by default on every results/deck-detail view.
  Narrowed `.col-row`/`.col-lemma`/`.col-count`/`.col-actions` at the existing
  375px breakpoint so all four visible columns fit inside the viewport without
  requiring horizontal scroll to reach per-word actions; the "Occurrences ↓"
  header now wraps instead of visually overlapping "Status".
- **Ambiguity panel (Multiple possible meanings flow, shipped same day):** the
  panel renders in a `colspan` cell that inherited the table's fixed-column
  width sum rather than the scroll container's visible width, so at 375px it
  extended ~140px past the right edge with the "Not sure" action clipped.
  Clamped `.ambiguity-panel` to `calc(100vw - 3rem)` and bumped
  `.ambiguity-candidate-actions button` height (22px → 28px) so "I know this
  meaning" / "Study this meaning" / "Not sure" stay on-screen and easier to tap.
- **Left cosmetic-only, unfixed:** the embedded-catalog "Read this text" cards
  (`.catalog-card`) are dense/cramped at 375px (tight tag row, wrapped
  long titles) but remain readable and tappable - no BROKEN classification.
  A dedicated reading-surface redesign is tracked separately; this audit's
  before/after evidence is attached to that follow-up.

No new breakpoints were introduced; both fixes reuse the existing "Mobile
(375 px) tweaks" `@media (max-width: 600px)` block in `web/styles.css`.
## 2026-07-04 - First manual test-run fixes: logo case, due-count semantics, sentence chip

Four fixes from the owner's first manual test of the app.

- **Nav logo capitalization:** `.nav-logo::before` in `web/styles.css`
  (the "Design v2 alignment" cascade block, which wins over the earlier
  `.nav-logo` rule and the correct `FinnEst` text already in
  `web/index.html`) injected the literal lowercase string `"finnest"` via
  CSS `content`, overriding the real DOM text with `font-size: 0`. Changed
  the injected content to `"FinnEst"` to match [CONTEXT.md "Product
  Name"](../CONTEXT.md) and grill Q53. Titles, About-page copy, and Sign-in
  already said `FinnEst` correctly - only the CSS injection was wrong.
  Added a Playwright assertion on the logo's rendered `::before` content.
- **Dashboard due-count semantics (deliberate change):** `CountDueCards` and
  `GetUserDeckStats`'s `due_count` treated `card_state.next_due IS NULL` -
  the default for a card that has never been introduced - as "due now". A
  fresh account adding the two Top-1000 starter decks immediately saw "Due
  to review: 2,000" even though none of those cards had ever been shown to
  the learner (the correctly-gated "New words today" tile showed 20).
  "Due" now additionally requires `introduced_at IS NOT NULL OR
  last_answer_at IS NOT NULL` - the same "introduced" predicate
  `CardsInReview` already used. Never-introduced cards now surface only
  through the existing `new_capacity_today` / "New words today" tile; no
  new dashboard tile was added. `GetNextReviewCard`'s new/due pooling and
  the daily-new-card limit are unchanged. Added
  `TestCountDueCardsExcludesNeverIntroducedCards` in
  `internal/store/db_test.go` pinning the new semantics.
- **"Sentence card" chip on sentence-less cards:** `HandleReviewNext`
  hard-coded `CardResponse.Mode` and `Front.Type` to `"sentence"`
  regardless of whether the card actually had a source sentence. Starter
  decks (no source sentences) showed a "Sentence card" chip over a
  front-text line that just repeated the surface heading below it, with
  nothing in between. Mode/front type are now `"word"` when
  `card.SentenceText` is empty; `web/app.ts` hides `#review-card-front`
  (chip + front text) entirely in that case so the card reads cleanly:
  surface, lemma, gloss, deck tag. Added a Playwright case for the
  no-sentence card.
- **Top-1000 FI starter deck lemma resolution (`ase` → `asea`, ledgered, not
  fixed):** diagnosed `cmd/seedcolddeck` resolving surface `ase` ("weapon")
  to lemma `asea`/VERB ("synonym of asettaa") instead of `ase`/NOUN. Root
  cause is a stale dictionary import, not `seedcolddeck` ranking or Decision
  19/20 filtering: `finnestdb.db`'s `forms` table has every inflected form
  of noun `ase` (aseen, aseita, aseessa, ...) except the bare nominative
  self-mapping row `ase→ase/NOUN`, while `ase→asea/VERB` is the only row
  keyed by that surface - so the ranker never had a competing candidate to
  prefer. `dict_metadata` shows the live FI import ran 2026-03-13; the
  on-disk `kaikki.org-dictionary-Finnish.jsonl.gz` is dated 2026-05-07 and
  does contain the missing nominative-singular form tag for `ase`. Logged
  as `DICT-1` in [`TODO.md` "Alpha launch issue
  ledger"](../TODO.md#alpha-launch-issue-ledger) with the re-import +
  reseed + re-freeze-baseline exit condition, per instruction not to
  work around a data-staleness issue in ranking code.

## 2026-07-04 - Multiple possible meanings flow shipped (Ambiguous meaning flow gate)

Shipped the learner-facing **Multiple Possible Meanings** flow, closing the
"Ambiguous meaning flow" public-alpha gate as the alpha default. Per
[`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) §4 no ambiguity class
qualifies for the single confident Meaning Check on the v1 slice, so only the
Multiple-possible-meanings branch is built - no confidence is presented.

- **Metadata delivery:** `/api/parse` enriches signed-in responses with a
  top-level `ambiguous_surfaces` list (surface, first-occurrence example,
  candidate meanings). Chosen inline over a lazy per-row endpoint after measuring
  a large parse: ~18–20% of unique surfaces are ambiguous and candidate metadata
  adds ~200 bytes per ambiguous row (~18% payload growth, bounded by ambiguity
  rate, not token count) - cheaper than a round-trip-per-expansion that would
  re-resolve the same candidates. New `store.SurfaceCandidates` is the single
  source of truth (FST-merged inclusion via `MergeFSTReadings`, quarantine-filtered
  through the new `QuarantinedSenseFilter`, glossed). Anonymous parses are
  unchanged and carry no ambiguity metadata (stateless demo scope).
- **Results-row UI (signed-in):** an unobtrusive "Multiple possible meanings"
  chip expands to the sentence context + candidate meanings with per-candidate
  **I know this meaning** (records `(lemma,pos)` known, excludes from pending
  deck), **Study this meaning** ("Creates a review card when you save."), and
  **Not sure** (conservative = study). **None of these looks right** opens the
  flag-only correction path with surface/context prefilled - parser feedback,
  never a study/known action.
- **Explicit FST-sense deck save:** an explicitly selected FST-only sense creates
  its card on save via a narrow, validated bypass of PR #269's dict-only deck
  expansion (`CreateDeckRequest.selected_senses` → `injectSelectedSenses`,
  validated against the real candidate set).
- **Known-word import summary:** `POST /api/known-words` returns
  `needs_sense_confirmation` for the honest lazy-resolution summary line.
- **Review card back:** the same flag-only "None of these looks right" escape,
  plus the already-shipped homograph note.

Docs: [`USER_FLOWS.md`](USER_FLOWS.md) §5, [`srs-deck-spec.md`](srs-deck-spec.md)
ambiguous-imports, [`CONTEXT.md`](../CONTEXT.md) Multiple Possible Meanings,
[`TODO.md`](../TODO.md) Ambiguous meaning flow gate.

## 2026-07-04 - FI candidate-inclusion gap closed (FST-merged ambiguity candidate set)

Merged FST-known homograph readings into the ambiguity / meaning-check
candidate set so cross-POS second senses become offerable. kaikki's `forms`
table stores only one reading per homograph surface (`kuusi`→NUM, `tuli`→VERB,
`voi`→NOUN), so the second sense was previously absent from the candidate set
even though the FST knows it. New
`store.BatchLookupAllFormsWithOptions(..., AllFormsOptions{MergeFSTReadings: true})`
appends FST-only `(lemma, POS)` readings, deduped against dict rows and ranked
below authoritative dict/override candidates (source-priority model), with
analyzer emission order preserved. `cmd/ambiguityeval` now measures this merged
set. FI ambiguity **candidate inclusion 72.9% → 95.8%**; selection accuracy
unchanged at 70.8%; FI + ET headline baselines byte-stable. The deck / import
expansion path keeps the dict-only `BatchLookupAllForms`, so learner-facing deck
word counts are unchanged (deliberate gating - see
[`FEATURES.md`](FEATURES.md) multi-lemma and
[`srs-deck-spec.md`](srs-deck-spec.md)). Parser stamp `2026.05.15a` →
`2026.05.15b` ([`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) §2026-05-15b,
[`SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md),
[`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) §Ambiguity).

## 2026-07-04 - `cmd/ambiguityeval` + `make compare-ambiguity` shipped

Implemented the runner specced earlier the same day in
[`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) §Ambiguity and
meaning-check calibration: `cmd/ambiguityeval` loads a `slice:"ambiguity"` gold
file, runs `parsecore.Analyze(...,"custom")` for the pick and
`store.BatchLookupAllForms` for the candidate set per target token, and reports
candidate-inclusion, selection-accuracy, and proxy-stratified accuracy keyed by
`ambiguity_class`, plus which classes clear the §4 threshold rule. Wired into
`make compare-ambiguity` via `scripts/compare-ambiguity.sh`, which discovers
`testdata/parser-eval/*-ambiguity/*.json` and writes a dated JSON report under
`reports/parser-eval/`, parallel to `scripts/parser-comparison{,-et}.sh`.

Ran it for real against the production DB + FST tables (parser `2026.05.15a`):
**FI selection 34/48 = 70.8%, candidate inclusion 35/48 = 72.9%; ET selection
6/13 = 46.2%, candidate inclusion 13/13 = 100.0%.** Candidate inclusion matches
the spec's hand-verified baseline exactly on both languages. Selection is lower
by 2 FI cases and 1 ET case; root-caused (not tuned away) in
`PARSER_EVAL_METHODOLOGY.md`'s "Runner shipped" note: two are a sentence-initial
casing gap in the exact-`Form`-match occurrence lookup shared with
`internal/eval.findOccurrence` (the parser's pick is actually correct in both),
and one (`fi-amb-kayda-2`, the compound `lääkärikäynti`) is a genuine,
newly-discovered candidate-set gap - the same failure category as the headline
`kuusi`/`tuli`/`voi` cases, just via compound-splitting instead of a bare
cross-POS homograph. No FI or ET class meets the threshold rule on this run.
`docs/baselines/README.md` documents the `fi-ambiguity`/`et-ambiguity` dataset
suffix for a future frozen baseline; no baseline is frozen by this change - that
stays a maintainer action per `PARSER_EVAL_METHODOLOGY.md` §6.

## 2026-07-04 - Ambiguity eval slice specced + verified FI/ET gold cases

Wrote the Finnish-first ambiguity eval slice spec (the measurement foundation the
"Ambiguous meaning flow" gate depends on) as an expansion of
[`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) §Ambiguity and
meaning-check calibration - no new planning doc (Decision 24 / Q60). Covers what
is measured (candidate inclusion, selection accuracy, calibration), an honest
confidence *proxy* (there is no numeric confidence today; the `custom` ranking is
a stable sort over discrete signals), the minimal gold-format extension, the
per-class threshold→UI rule (single Meaning Check only when selection ≥ 90% AND
candidate inclusion = 100% AND N ≥ 4), the ET parity plan, and the
`cmd/ambiguityeval` runner plan.

Committed verified gold data under `testdata/parser-eval/fi-ambiguity/`:
`fi-ambiguity-v1.json` (48 cases, 21 classes) + `et-ambiguity-v1.json` (13 cases,
6 classes) + `README.md`. Baseline against the real DB + full FST tables (parser
`2026.05.15a`): **FI selection 75.0% / candidate inclusion 72.9%; ET selection
53.8% / candidate inclusion 100%.** Headline finding recorded in
[`TODO.md`](../TODO.md): FI and ET fail *differently* - FI's blocker is candidate
inclusion (kaikki `forms` stores one reading per cross-POS homograph, so
`kuusi`/`tuli`/`voi` second sense is absent from the candidate API though the FST
knows it), ET's blocker is selection ranking (Ekilex supplies all candidates but
the pick prefers VERB on cross-POS collisions). No class is single-check-eligible
on v1, so the honest alpha default is Multiple possible meanings everywhere.

Sequenced implementation tasks (build `cmd/ambiguityeval`, `make
compare-ambiguity`, close the FI candidate gap, expand + freeze) added under the
existing ambiguity items in `TODO.md`.

## 2026-07-04 - FI catalog fully human-reviewed; article-genre calibration signal

Sagar reviewed the replacement Wikipedia sauna article: easy-medium, overriding
the model's medium-hard. All three FI texts now carry approved reviews. Second
consecutive two-band overrate on an everyday-topic article - recorded in the
difficulty-model calibration notes (`GO_LIVE_CHECKLIST.md`): familiar concrete
topics read easier than the lexical/structural signals suggest.

## 2026-07-04 - Catalog texts moved to real published sources

A naturalness review of the agent-written "sauna" article found it stilted, so
the four machine-written catalog texts were replaced with real published,
redistributable sources. Policy going forward: catalog texts come from real
published sources (public domain or CC; Gutenberg, Wikisource, Wikipedia);
agent-authored content is a last resort requiring explicit owner approval.

- Replaced: `fi-sauna-article` (FI Wikipedia "Sauna", CC BY-SA 4.0),
  `et-tallinn-vanalinn-article` (ET Wikipedia "Tallinna vanalinn", CC BY-SA
  4.0), and - replacing the two remaining ET originals - `et-mesipuu-poem`
  ("Ta lendab mesipuu poole", Juhan Liiv, d. 1913, public domain, from
  Estonian Wikisource) and `et-linnu-keel-story` ("Linnu keel" folk tale from
  Juhan Kunder's *Eesti muinasjutud* 1885, d. 1888, public domain). The ET set
  now mirrors FI: one article + one story + one poem.
- Provenance and share-alike obligations recorded per text in
  `internal/catalog/specs.json` (source URL, license, attribution).
- Review pin corrected: `fi-sauna-article`'s review was removed with its text;
  the two kept FI Gutenberg texts stay approved. The catalog and API tests now
  derive "approved iff signed off in reviews.json" instead of pinning
  "FI approved / ET pending", so a pending new FI text no longer breaks them.
- Updated: [`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md) sourcing policy,
  [`../TODO.md`](../TODO.md) catalog gate, [`../CONTEXT.md`](../CONTEXT.md)
  Embedded Text entry.

## 2026-07-04 - Five-level catalog difficulty + FI human review recorded

The first human difficulty review (Sagar, FI, 3 texts) found every text on a
bucket boundary and one model-vs-human ordering inversion, so the Global
Difficulty scale is now five-level with a human-override mechanism.

- Added: `internal/catalog/reviews.json` - reviewer sign-offs (reviewer, date,
  note, optional difficulty override) merged by `cmd/gencatalog`; approved
  entries carry `difficulty_review: "approved"` + reviewer metadata, and the
  model's verdict is preserved in `difficulty_computed`.
- Modified: `internal/catalog/difficulty.go` - four cut points (0.29 / 0.39 /
  0.53 / 0.63) yielding easy / easy-medium / medium / medium-hard / hard;
  thresholds re-pinned in `difficulty_test.go`; calibration notes in
  [`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md).
- FI verdicts applied: sauna easy-medium (model said medium-hard), Kesäaamu
  medium-hard (model: hard), Hiiri-Pekka medium-hard (model: medium). ET
  entries remain `pending` for an Estonian reviewer.

## 2026-07-04 - Overnight launch-gate run (PRs #250–#259)

Ten PRs merged in one overnight run, closing or advancing most public-alpha
gates: grill-docs promotion, FOR_MICHAEL guide, FI/ET parity audit, flag-only
feedback, RC pack skeleton, anonymous parser demo, correction issues +
quarantine, parser backpressure + load test, embedded catalog, and the
surface-card identity migration with narrow FSRS behind a flag.

- Added: [`launch-readiness/2026-07-04-overnight-report.md`](launch-readiness/2026-07-04-overnight-report.md)
  - the consolidated report and go-live handoff (gate status, human actions).
- Modified: `TODO.md` "Public alpha gates" - twelve gates ticked with evidence
  pointers; open gates annotated with shipped-tonight progress and exact
  remaining steps.

## 2026-07-04 - Surface-form review-card identity + narrow FSRS behind a flag

Implements the review-readiness launch gate (Decision 23). Review cards move
from `(user_id, lang, lemma, pos)` to `(user_id, lang, surface_norm, lemma,
pos)`: the normalized surface form the learner encountered joins the key, with
`(lemma, pos)` kept as the sense discriminator so homographs are distinct sense
cards and are not collapsed (MWE cards carry `surface_norm = ''`). Migration
(`ensureSurfaceScopedCardsTable`) rebuilds `cards`/`card_state` via the
established table-rebuild pattern, backfilling `surface_norm` from each card's
most-frequent occurrence surface (falling back to the lemma) and carrying card
ids forward so scheduler state is preserved; card count is asserted unchanged.
Card creation on deck save/subscribe seeds one card per `(surface, lemma, pos)`;
the review payload gains the surface (primary identity) and a homograph note.
Quarantine `(lemma,pos)` issues still suppress all surface cards of a sense, and
surface-only issues now also suppress cards by `surface_norm`.

Adds narrow FSRS (`go-fsrs/v3`, default parameters) behind
`FINNESTDB_FSRS_ENABLED` (default OFF). The step scheduler stays the shipped
fallback. FSRS and legacy state coexist in `card_state.fsrs_json` via a version
discriminator; migration is lazy on first rating (NULL → new card; legacy
step+interval → conservative Review-state seed), and a flag rollback keeps
FSRS-touched cards answerable through the step scheduler without losing
progress.

- Modified: [`srs-deck-spec.md`](srs-deck-spec.md) - card identity + narrow FSRS
  marked implemented (flag off by default); documents the `fsrs_json` versioning
  and lazy-migration derivation; Open decisions sense-key resolution recorded.
- Modified: [`DEPLOYMENT.md`](DEPLOYMENT.md) - `FINNESTDB_FSRS_ENABLED` env var +
  "FSRS scheduler rollout" (staging-first with seeded histories, then flip).
- Modified: [`CONTEXT.md`](CONTEXT.md) - **Card** term updated to the implemented
  surface-form key.
- Related: [`DECISIONS.md`](DECISIONS.md) Decision 23 (surface-first learner
  model + narrow FSRS).

## 2026-07-04 - Embedded text catalog mechanism shipped

Shipped the curated Embedded Text catalog mechanism for signed-in cold start
(TODO.md gate "Curated embedded text catalog"; USER_FLOWS.md §4; DECISIONS 23
cold-start portion, 27). New `internal/catalog` package embeds checked-in
metadata (`catalog.json`) plus one plain-text fixture per text via `go:embed`,
so production carries no corpus-pipeline dependency. `cmd/gencatalog`
regenerates the catalog deterministically: each text is parsed through the real
custom-mode pipeline, text-level difficulty metrics are computed, and an
Easy/Medium/Hard bucket is assigned by documented thresholds
(`docs/GO_LIVE_CHECKLIST.md` "Embedded catalog difficulty model"). Each entry
carries a precomputed `(lemma, pos)` list so per-learner known-token coverage
(Personalized Text Fit) is a cheap set intersection at request time. New
signed-in endpoints `GET /api/catalog` (metadata + coverage) and
`GET /api/catalog/{id}/text` (lazy full text) back dashboard and Inspect
cold-start empty states. Initial coverage is honest, not the full 36-text
matrix: 3 FI texts (Gutenberg public-domain poem + short story, one original
CC0 article; medium/hard) and 3 ET texts (original CC0; easy/medium) -
Estonian Gutenberg material was effectively unavailable, so ET ships original
CC0 texts. Every entry ships `difficulty_review: "pending"`; the full matrix
and human sanity-check remain open and the gate stays unchecked. See
`docs/USER_FLOWS.md` §4, `CONTEXT.md` "Embedded Catalog", `TODO.md`.
## 2026-07-04 - Parser backpressure and launch load test

Implements the parser concurrency/backpressure and load-test bullets of the
"1,000-concurrent-user launch target" gate. A new counting semaphore in
`internal/api` (`parser_limiter.go`) bounds concurrent calls into the parser
(`/api/parse` and deck-save), independent of the existing per-IP/per-account
rate limiters. Anonymous parse requests draw from a smaller sub-pool (half
the total slots) before the shared pool, so anonymous load sheds first under
saturation and cannot starve signed-in deck/review traffic. A request that
cannot get a slot within the queue timeout returns 503 with `Retry-After`;
the existing 429 rate-limit path now also carries `Retry-After`. New
`cmd/loadtest` is a dependency-free Go client that models the GO_LIVE traffic
mix (anonymous parse / signed-in parse / review-deck reads) and reports
per-endpoint latency percentiles, throughput, and 429/503/error counts.
Staged local runs (50/200/500/1000 concurrent virtual users, laptop against
the production-size local DB) confirm the shedding mechanism and that
deck/review reads stay unaffected under full saturation; production-host
re-validation remains before the launch gate can close.

- Added: `internal/api/parser_limiter.go` - the semaphore, wired into
  `HandleParse` and `handleCreateDeck`.
- Added: `internal/api/parser_limiter_test.go` - saturation/shedding/timeout/
  non-parse-bypass unit and handler-level tests.
- Modified: `internal/api/rate_limit.go` - `allowLimiter`'s 429 now sets
  `Retry-After: 60`.
- Added: `cmd/loadtest` - the load-test tool.
- Added: [`launch-readiness/2026-07-04-load-test.md`](launch-readiness/2026-07-04-load-test.md)
  - method, hardware caveat, stage results, shedding evidence, anonymous-cap
  recheck, recommended production env values, and the production-host re-run
  instruction.
- Modified: [`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md) "Capacity and
  Graceful Degradation" - shipped items checked off with evidence links;
  production-host re-run and monitoring wiring left open.
- Modified: [`DEPLOYMENT.md`](DEPLOYMENT.md) - `FINNESTDB_PARSER_MAX_CONCURRENCY`
  and `FINNESTDB_PARSER_QUEUE_TIMEOUT_MS` added to the environment-variable
  table; load-test usage and a parser-saturation monitoring note added.
- Modified: [`../TODO.md`](../TODO.md) - both 1,000-concurrent-user bullets
  get progress notes; neither is ticked (production-host run remains).
## 2026-07-04 - Correction issues + admin-only quarantine (Phase 1c)

Ships the global correction-issue ledger and admin-only faulty-content
quarantine (public-alpha gate). New `correction_issues` table plus
`parse_feedback.correction_issue_id` (both via idempotent CREATE/ALTER,
Decision 12). Feedback submission groups each report into a found-or-created
issue by a `(lang, parser, norm_surface, lemma, pos)` scope fingerprint and
recomputes report/distinct-reporter counts; a report against a `fixed` issue
reopens it. Admins classify an issue (one of `parser_issue`, `bad_card_content`,
`source_extraction_issue`, `not_sure`), then **Quarantine now** (required
reason) suppresses matching content globally - review and new-card queues, deck
word/due/new-card counts, and `DeckComprehension` coverage/unlocks all exclude
it - while `review_log` history stays untouched. Restore is a status flip that
returns content with `card_state` intact. A `threshold_candidate` badge appears
at ≥3 distinct reporters but never auto-quarantines. One combined admin queue;
no separate Issues page.

- Modified: [`PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md) - "Global
  correction issue ledger", "Report-to-quarantine workflow", "Alpha admin
  classification", "Admin triage", the intro paragraph, and the current-vs-target
  table row moved from target-tense to shipped.
- Modified: [`FEATURES.md`](FEATURES.md) "What We Store During Alpha" -
  quarantine bullet notes restore preserves scheduler state.
- Modified: [`../TODO.md`](../TODO.md) - Phase 1c ticked; "Parser feedback alpha
  gate" and "Quarantine behavior" public-alpha bullets ticked with shipped notes.
- Modified: [`../CONTEXT.md`](../CONTEXT.md) - Correction Issue, Faulty Content
  Quarantine, Trusted Quarantine Threshold, and Emergency Quarantine updated to
  shipped phrasing.
- Modified: [`srs-deck-spec.md`](srs-deck-spec.md) - coverage/quarantine and
  restore paragraphs moved to shipped tense.
- Cross-reference: [`DECISIONS.md`](DECISIONS.md) Decision 25.

## 2026-07-04 - Anonymous parser demo shipped

The public-alpha anonymous parser demo landed: the landing page now carries a
paste-first parse form (FI/ET selector, char counter, Parse button) that calls
`/api/parse` unauthenticated and renders explore-only results (POS filters,
sorting, row expansion, definitions/forms/examples/counts). Signed-in-only
actions stay hidden via `data-role-show`; anonymous results carry a
dismiss-per-session sign-up ribbon and a privacy footer. A new
`FINNESTDB_ANON_MAX_CHARS` cap (default 20,000) is enforced server-side before
parser work and surfaced to the client via `/api/me`.

- Modified: [`USER_FLOWS.md`](USER_FLOWS.md) §1/§2 - status flipped to shipped;
  cap/ribbon/footer behavior documented.
- Modified: [`FEATURES.md`](FEATURES.md) - Anonymous Parser Demo section shipped
  phrasing plus the cap details.
- Modified: [`GO_LIVE_CHECKLIST.md`](GO_LIVE_CHECKLIST.md) "Abuse Controls" -
  anon-cap bullet marked shipped-with-default.
- Modified: [`DEPLOYMENT.md`](DEPLOYMENT.md) - `FINNESTDB_ANON_MAX_CHARS` added
  to the environment-variable table.
- Modified: [`../TODO.md`](../TODO.md) "Public alpha gates" - Anonymous parser
  demo gate ticked with a shipped note; remaining work is load-test cap tuning.
- Modified: [`../CONTEXT.md`](../CONTEXT.md) - Anonymous Parser Demo term updated
  to shipped phrasing.
## 2026-07-04 - Flag-only parser feedback (Phase 1b)

Documents the public-alpha flag-only feedback path: signed-in learners can
report "this analysis looks wrong" without proposing a fix. Schema adds
`parse_feedback.flag_only` via idempotent ALTER (proposed columns kept
`NOT NULL`, stored empty for flag-only rows). Flag-only acceptance writes no
lexical override until an admin supplies a concrete lemma/POS, converting it into
a normal parser-identity correction.

- Modified: [`PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md) - schema field
  list, the "current implementation" paragraph, and the current-vs-target table
  (feedback type, schema/API, admin triage, acceptance behavior).
- Modified: [`FEATURES.md`](FEATURES.md) "What We Store During Alpha" - mentions
  flag-only reports.
- Modified: [`TODO.md`](../TODO.md) - Phase 1b ticked with a shipped note; the
  "Parser feedback alpha gate" bullet gets a progress note.
- Modified: [`../CONTEXT.md`](../CONTEXT.md) "Flag-Only Parser Feedback" - updated
  from planned to shipped.
## 2026-07-04 - First-experience RC pack skeleton

Ships the checked-in, repeatable release-candidate pack described in
`GO_LIVE_CHECKLIST.md` "First-experience quality check" and `CONTEXT.md`
"Release-Candidate Pack Manifest": one canonical manifest plus a Go runner
and a Playwright spec that both consume it, so the launch gate cannot drift
across separate case lists.

- Added: `testdata/first-experience-rc/manifest.json` - 18 cases covering
  all 8 first-experience journeys (anonymous-demo, embedded-text,
  own-text-inspect, deck-save, first-review, known-word-import,
  ambiguity-homograph, parser-feedback) with explicit FI and ET cases for
  every journey, plus short self-written fixture `.txt` files in the same
  directory, including the `kuusi`/`tuli`/`voi` homograph fixtures from
  `PARSER_EVAL_METHODOLOGY.md` "Ambiguity and meaning-check calibration".
- Added: `cmd/firstexperiencerc` - loads the manifest and runs every
  `automation:"parser"` case through the real custom-mode parser pipeline
  (`internal/parsecore`), printing PASS/FAIL/SKIP-pending/MANUAL and a
  summary; exits nonzero only on an automated FAIL.
- Added: `web/tests/first-experience-rc.spec.ts` - generates one Playwright
  test per `automation:"playwright"` manifest case (with `test.skip` stubs
  for everything else), reusing the existing Inspect/save-deck/review and
  parser-feedback correction-submit patterns from `parse-results.spec.ts`.
- Added: `make first-experience-rc` - runs the Go runner, then the RC
  Playwright spec, then points at the manual walkthrough instructions in
  `GO_LIVE_CHECKLIST.md` (no separate walkthrough doc).
- Modified: `GO_LIVE_CHECKLIST.md` and `TODO.md` - record which journeys are
  automated today (embedded-text, own-text-inspect, ambiguity-homograph via
  the Go runner; deck-save, first-review, and FI parser-feedback via
  Playwright) versus still pending (anonymous-demo FI+ET, known-word-import
  FI+ET, parser-feedback ET).
- This is a skeleton per Q58/Q60: it is expected to gain automated coverage
  (not fixture text) as the pending journeys land, not to be treated as the
  final pass/fail launch gate yet.
## 2026-07-04 - FI/ET equal-status parity audit

Journey-first audit of the public-alpha "Equal-Status Alpha Gate" from
[`CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md#equal-status-alpha-gate),
run against a live server and the production-size local DB.

- Added: [`launch-readiness/2026-07-04-fi-et-parity-audit.md`](launch-readiness/2026-07-04-fi-et-parity-audit.md)
  - per-journey FI/ET evidence table (anonymous parse, signed-in Inspect,
  deck save/detail/review, known-word import, parser feedback, admin queue,
  data readiness, eval baselines, tests, embedded catalog, starter decks),
  classified alpha-blocking / language-specific / post-alpha, plus a cleanup
  appendix of every row the audit wrote to the shared DB.
- Modified: [`../TODO.md`](../TODO.md) - added ledger row `PARITY-1` (official
  "Top 1000" starter decks absent from the DB for both languages despite a
  prior "shipped, verified end-to-end" claim) and appended an audit-run note
  under the "FI/ET equal-status parity audit" gate bullet.
- Verdict: conditional pass. All exercised learner journeys showed full
  FI/ET parity; the one alpha-blocking finding is a runbook-execution gap
  (starter decks never seeded in this DB), not a code or design asymmetry
  between the two languages.

## 2026-05-15 - FI manual-card trap promotions

Documents the parser-stamp bump for promoting recent manual Finnish card
fixes into source-agnostic parser behavior: `sanoin` resolves as
`sanoa/VERB`, exact capitalized `Maria` as a proper name, and exact
capitalized `Norjan` as the genitive of `Norja`.

- Added: [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) entry
  `2026-05-15a` - records the behavior changes, source-agnostic
  rationale, and focused verification command.
- Modified: [`SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) and
  [`TODO.md`](../TODO.md) - current parser behavior tag is
  `2026.05.15a`. The latest frozen baseline remains
  `parser-baseline-2026-05-12-b-T1606Z`; a new freeze is still needed
  for headline aggregate numbers.

## 2026-05-15 - Source-agnostic correction taxonomy

Documents how learner-reported card and parser fixes should land in FinEstDB
without tying the workflow to any one source corpus or Anki. The new taxonomy
keeps Finnish and Estonian correction content separate while sharing the same
admin workflow, target model, and overlay categories.

- Added: [`CORRECTION_TAXONOMY.md`](CORRECTION_TAXONOMY.md) - learning targets
  can be lemma, surface, phrase, or proper-name entries; accepted fixes are
  classified as parser identity, meaning cue, contextual sense, phrase boundary,
  example quality, or card presentation.
- Modified: [`PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md) - admin
  acceptance should classify feedback before writeback, because many
  learner-visible bad cards are not parser-identity bugs.
- Modified: [`INDEX.md`](INDEX.md) and [`TODO.md`](../TODO.md) - add the new
  correction taxonomy to canonical navigation and open implementation work.

## 2026-05-13 - ET review follow-ups parser stamp (PR #205)

Documents the parser-stamp bump for the PR #205 follow-ups that landed
on top of the `2026.05.12d` source-backed ET cleanup. The behavior
changes are real (basic-mode special-cap FEATS parity, attribute-based
ET verb dictionary-form check, explicit `TA` lex-overlay bypass) but
narrow; each is pinned by a unit test.

- Added: [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) entry
  `2026-05-12e` - records the parser stamp bump, the three behavior
  changes, and the verification commands.
- Modified: [`SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) and
  [`TODO.md`](../TODO.md) - current parser behavior tag is
  `2026.05.12e`. The latest frozen baseline remains
  `parser-baseline-2026-05-12-b-T1606Z`; a new freeze is still needed
  for the combined `2026.05.12e` state.

## 2026-05-12 - ET verb-inflection bias baseline merge

Documents the PR #203 merge resolution after the source-backed ET learner
cleanup landed first on `main`.

- Modified: [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) - keeps the
  `2026-05-12b-T1606Z` baseline as historical evidence and adds
  `2026.05.12d` as the combined current parser stamp.
- Modified: [`SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) and
  [`TODO.md`](../TODO.md) - update the current parser behavior tag to
  `2026.05.12d` while noting that a new freeze is needed for combined
  headline numbers.

## 2026-05-12 - Source-backed ET learner cleanup

Documents the follow-up parser/importer change from the Sõnaveeb/Ekilex
audit of high-frequency Estonian learner rows. The fix keeps source
claims tied to the reduced Ekilex artifacts or Sõnaveeb pages, but stops
letting source-side long-tail translations and duplicate morphology rows
become misleading learner primaries.

- Added: [`DECISIONS.md`](DECISIONS.md) Decision 22 - why ET learner
  corrections stay deterministic, source-audited, and small: exact
  capitalization for special dictionary lemmas, invariant closed-class
  morphology cleanup, basic-mode direct-dict source filtering, ET verb
  dictionary-form display, runtime compatibility for already-imported
  DBs, and curated overrides only for high-frequency rows with verified
  bad learner primaries.
- Added: [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) entry
  2026-05-12c - records the parser stamp bump, changed behavior, and
  focused verification commands.
- Modified: [`FST_LEMMATIZER.md`](FST_LEMMATIZER.md) "Store-level
  candidate merge" section - documents the ET overlay additions and
  the dictionary-side special-capitalization / invariant-FEATS guards.
- Modified: [`SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) - updates
  the current parser dated tag to `2026.05.12c`.
- Modified: [`INDEX.md`](INDEX.md) and [`TODO.md`](../TODO.md) -
  refresh the current decision count and parser behavior stamp after
  Decision 22.

## 2026-05-12 - Documentation state refresh

Refreshes the living status docs after reviewing the last 20 merged PRs
on `main` (#176, #177, #179-#181, and #183-#197).

- Modified: [`docs/INDEX.md`](INDEX.md) and [`TODO.md`](../TODO.md) -
  corrected the decisions count, current parser-v2 status, recent upload
  support, and open-PR snapshot.
- Modified: [`corpus_pipeline/v2plan.md`](../corpus_pipeline/v2plan.md) -
  removed duplicate superseded v2.4-v2.8 follow-up blocks so the roadmap
  no longer reports the same work as both done and not started.
- Modified historical docs and reports to replace stale local Markdown links
  with valid file links or plain local-only paths.

## 2026-05-12 - Deck/parse low-value dict-alternative filter (PR #185)

Records the deck/parse expansion change in PR
[#185](https://github.com/sagarinbabel/finnestdb/pull/185): when a surface
form has multiple dict candidates and at least one has a non-empty gloss,
empty-gloss alternatives and Wiktionary form-of alternatives are suppressed
when a lexical-base alternative exists. Form-of detection is structural
(`candidate.Lemma == form`, single-clause gloss with `<allowed morphology>
of <single-word target>`), so common lexical glosses whose body text
happens to mention grammatical terms (`vana/ADJ`, `oma/ADJ`, `mennä/VERB`)
are not affected. Unresolved/gap surfaces - every candidate gloss empty,
or no lexical-base alternative - are preserved as-is.

- Added: [`docs/DECISIONS.md`](DECISIONS.md) §Decision 19 - context,
  structural detector, and false-positive reasoning behind the filter.

## 2026-05-12 - Analyser-quality learnings from yle_subs (PR #183)

Documents the parser/dict/ingest changes shipped in PR
[#183](https://github.com/sagarinbabel/finnestdb/pull/183) - five
runtime fixes that pull learner-quality corrections from
`yle_subs` back into finnestdb's parser and dict layer.

- Added: [`DECISIONS.md`](DECISIONS.md) Decision 20 - why the
  lexical-overlay short-circuit runs at Step 0 of
  `BatchLookupForms` (not inside `Lemmatize`), why it's
  custom-mode-only, and why the bad-lemma blocklist is two-tiered
  (never-legitimate fragments + (surface, lemma) pairs).
- Added: [`PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) entry
  2026-05-12a - describes the parser-behavior bundle
  (lexadverbs overlay, `NormalizeMaInfinitive`, MA-infinitive
  ranking bias, bad-lemma blocklist, structural-gloss filter at
  kaikki ingest, `BatchLookupSenses` API, FI+ET analyser-traps
  gold fixtures, low-value dict-alternative suppression, and
  source-priority-first dict/FST ranking) and reports the
  measurement against the new #183 gold sets (FI custom
  lemma=95.2%, ET 100%).
- Modified: [`FST_LEMMATIZER.md`](FST_LEMMATIZER.md) "Store-level
  candidate merge" section - documents the new Step 0
  lex-overlay short-circuit, the MA-infinitive ranking bias, the
  two-tier bad-lemma filter, and the documented residual on
  `tarjoamaan` pending FST table regeneration with MA-infinitive
  surfaces.

## 2026-05-09 - Architecture and corpus documentation audit

Refreshes the living docs after the corpus pipeline and baseline-compression
PRs landed, while leaving historical reports and frozen baselines
append-only.

- Modified: [`ARCHITECTURE.md`](../ARCHITECTURE.md) - updated the current
  product surface, Mermaid diagram, layer responsibilities, data flows,
  localdata/FST boundaries, and near-term direction to include the corpus
  pipeline.
- Modified: [`corpus_pipeline/docs/CORPUS_PIPELINE.md`](../corpus_pipeline/docs/CORPUS_PIPELINE.md) -
  replaced stale future-work wording with built-vs-deferred status, refreshed
  profile guidance, and corrected FST-table troubleshooting.
- Modified: [`corpus_pipeline/docs/PR_ROADMAP.md`](../corpus_pipeline/docs/PR_ROADMAP.md),
  [`corpus_pipeline/v2plan.md`](../corpus_pipeline/v2plan.md), and
  [`corpus_pipeline/Makefile`](../corpus_pipeline/Makefile) - aligned roadmap
  statuses and profile help text with the landed corpus PRs.
- Modified: [`docs/FST_LEMMATIZER.md`](FST_LEMMATIZER.md) - documented the
  ET generated-table command and remaining production-promotion conditions.
- Modified: [`docs/INDEX.md`](INDEX.md), [`README.md`](../README.md),
  [`TODO.md`](../TODO.md), and [`finnestdb-prd-alpha.md`](../finnestdb-prd-alpha.md) -
  refreshed canonical navigation, PR state, and the historical PRD's
  implementation snapshot.
- Added: [`docs/qa-reports/2026-05-08T2229Z-doc-architecture-corpus-audit.md`](qa-reports/2026-05-08T2229Z-doc-architecture-corpus-audit.md) -
  timestamped full-doc audit and Git/GitHub branch-state report.

## 2026-05-09 - Typographic quote tokenization docs (PR #171)

Documents the current Rust parser contract after PR
[#171](https://github.com/sagarinbabel/finnestdb/pull/171): leading/trailing
punctuation cleanup includes common typographic quote marks, and opening
punctuation labels are part of sentence-text spacing reconstruction.

- Modified: [`README.md`](../README.md) - clarified the known-limitations
  summary for tokenizer punctuation and opening-quote spacing behavior.

## 2026-05-07 - Voikko `[P4]` Voice + participle field cleanup (PR #158)

Closes the Voice accuracy gap flagged in the parser audit (FI custom
5.3% vs omorfi 89.7% on fi-ftb) by fixing two specific bugs in
`voikkomap` on top of the rich Voice/VerbForm/PartForm extraction that
already landed in PRs [#154](https://github.com/sagarinbabel/finnestdb/pull/154)
and [#155](https://github.com/sagarinbabel/finnestdb/pull/155):

1. **`[P4]` no longer leaks as `Person=4`.** Finnish passive is
   grammatically the "4th person" in Voikko's tag set, but UD `Person`
   is 1/2/3. `[P4]` now sets `Voice=Pass` and leaves `Person` empty;
   `[P1-P3]` set `Voice=Act` alongside the UD `Person` value, so
   active finite verbs no longer compose FEATS without Voice.
2. **`applyParticiple` clears finite-only fields.** Defense-in-depth:
   when `[R*]` wins, Mood/Tense/Person are reset so a participle
   never composes contradictory FEATS like `Tense=Past|VerbForm=Part`
   - UD encodes the past/present distinction in `PartForm=`, not
   `Tense=`.

The shared Voice/VerbForm plumbing (Analysis fields, `applyParticiple`
per-tag mapping, `[Tn1-n5]` → `VerbForm=Inf`, Giellalt Act/Pass/Inf
extraction) all landed in PRs #154 and #155; PR #158 fills in the two
Voikko-specific gaps those PRs left open.

The `[E*]` tags were investigated as a possible voice signal and found
to encode connegative status (Ef=false, Et=true, Eb=both) - confirmed
from libvoikko's `FinnishVfstAnalyzer.cpp::parseBasicAttributes`.
Documented in the voikkomap header. Not projected to UD because the
runtime already gets `Connegative=Yes` from the orthogonal `[Cn]` tag.

- Modified: [`docs/FST_LEMMATIZER.md`](FST_LEMMATIZER.md) - new
  "Voikko Voice extraction" subsection covering the `[P*]` Voice
  derivation and participle field cleanup; updated stale 5-param
  `Compose` reference.
- Modified: [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md) - new
  entry for PR #158.

## 2026-05-07 - Baseline filename convention + freeze-baseline script

Standardizes the baseline filename convention so every baseline has a
**date AND time** stamp and the `docs/baselines/` directory stays
**append-only** in practice, not just in policy.

Canonical form going forward:

```
docs/baselines/YYYY-MM-DD<rev>-T<HHMM>Z-<dataset>.<ext>
```

- matching the `2026-05-07k-T1118Z-fi-core.json` logical baseline name
introduced with the [#140](https://github.com/sagarinbabel/finnestdb/pull/140)
baseline. Raw JSON files are now stored with a `.gz` suffix to keep repository
line counts manageable. Older tagged-style files
(`2026-05-06-final-*`, `2026-05-07-feats-rich-*`, etc.) are left as-is -
renaming them would break PR/commit cross-references the
[#141](https://github.com/sagarinbabel/finnestdb/pull/141) append-only history section was meant to
preserve.

- New [`scripts/freeze-baseline.sh`](../scripts/freeze-baseline.sh)
  takes the comparison-script `RUN_TS`, reads the parser-version letter
  from `parsecore.ParserVersion`, derives the date + UTC HHMM, and
  compresses per-dataset JSONs and copies cross-language summaries from
  `reports/parser-eval/` into `docs/baselines/` under the canonical
  name. **Refuses to overwrite** an existing target file - append-only
  is enforced mechanically, not just by convention. Override the
  parser-version letter with `-rev <letter>` for the rare case of
  freezing a measurement made before a same-day version bump.
- [`docs/baselines/README.md`](baselines/README.md) gains a
  "Filename convention" section spelling out the spec, with examples,
  the rationale for `T<HHMM>Z` (multiple same-day same-`<rev>`
  re-measures), and a pointer to the script.
- [`docs/PARSER_EVAL_METHODOLOGY.md §5 Freeze a baseline`](PARSER_EVAL_METHODOLOGY.md)
  replaces the manual `cp` recipe with a one-line `scripts/freeze-baseline.sh "$RUN_TS"`
  invocation.

## 2026-05-07 - Newcomer experience: `make doctor` + setup symmetry

Closes the gap between "the docs say setup is one command" and "the
parser silently runs in degraded mode because something didn't fetch".

- New [`make doctor`](../cmd/doctor/main.go) reports DB presence + per-
  source row counts, FST table presence, analyzer venv presence
  (`.venv`), Ekilex shard presence, UD cache,
  frequency baselines, and the Rust parser shared library. Each missing
  piece carries a one-line hint and a "go to" target. Returns 0 unless
  the DB or the FI/ET dictionary is missing entirely; everything else is
  informational so the user understands the *degraded modes* their setup
  implies. Added to [`docs/INDEX.md`](INDEX.md) and the
  [`README.md`](../README.md) Quickstart.
- [`scripts/parser-comparison.sh`](../scripts/parser-comparison.sh) and
  [`scripts/parser-comparison-et.sh`](../scripts/parser-comparison-et.sh)
  now slug from the dataset *file basename* rather than the JSON `name`
  field. Pre-fix, `fi-manual-v1.json` and `fi-manual-v2.json` both
  declared `name="fi-manual"` and silently overwrote each other in
  `reports/parser-eval/`. Fix called out in
  [`docs/PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) §pitfalls.
- `make setup-nlp` creates a unified `.venv/` containing both omorfi and
  estnltk instead of pip-installing into the active interpreter.
  `parser-comparison.sh` auto-constructs `FINNESTDB_OMORFI_CMD` from
  the venv when present, matching the EstNLTK auto-detection. Closes the
  open issue noted in
  [`docs/PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) §pitfalls
  about asymmetric venv discovery.
- New `BackfillLegacyKaikkiProvenance` migration in
  [`internal/store/db.go`](../internal/store/db.go) labels FI/ET rows
  that legacy importers left with empty `source` / `source_priority=0`
  as `(source='kaikki', source_priority=10)`. Idempotent - runs every
  startup but matches no rows once applied. Surfaced as a WARN in
  `make doctor` until a server start has run the migration.

## 2026-05-07 PM - Park autoresearch as post-live idea

Clarifies that autoresearch is an idea parking lot for after the app is shipped
and live, not active roadmap work.

- Added root [`AGENTS.md`](../AGENTS.md) with LLM-facing instructions:
  ignore autoresearch unless the user explicitly asks for it, and do not block
  unrelated parser/product work on `cmd/autoresearch` behavior.
- Added the same guardrail to [`docs/INDEX.md`](INDEX.md) and
  [`docs/AUTORESEARCH.md`](AUTORESEARCH.md).
- Relabeled top-level references in [`README.md`](../README.md),
  [`ARCHITECTURE.md`](../ARCHITECTURE.md), [`TODO.md`](../TODO.md),
  [`docs/FEATURES.md`](FEATURES.md), and [`docs/DECISIONS.md`](DECISIONS.md)
  so autoresearch reads as deferred post-live exploration.

## 2026-05-07 PM - Docs restructure + LLM-friendly navigation

Restructures the spine docs so a reader (human or LLM) can answer
"what's shipped, what's next, what's open, why" without cross-doc
detective work.

- [`TODO.md`](../TODO.md) restructured 765 → 646 lines: replaced "Roadmap
  Status" Phase 1–5 with explicit "What's in main" / "What's not in
  main yet" / "Open PRs" / "Research Goals" / "Notes & historical"
  sections. Implementation Backlog 1–19 distributed by area (Parser
  quality, Learner experience, Self-improving feedback loop, etc.).
  Critical Findings reframed as historical traceability.
- [`docs/DECISIONS.md`](DECISIONS.md) reordered latest-first (995
  lines): 4 new 2026-05-07 decisions added (Single-folder bootstrap
  rule; FST as parallel scorer; ESTONIAN_LEXICAL_PLAN consolidation;
  IMPLEMENTATION.md split). Absorbed 8 "Locked Decisions" from
  LEXICAL_PLAN.md as Decisions 7–14 (2026-05-06). Header renamed to
  "Decisions Log" (roadmap moved to TODO.md). Project Roadmap section
  preserved as historical with status updated.
- [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md) trimmed: "Locked Decisions"
  section moved to DECISIONS.md, "Phasing" section moved to
  PARSER_EVOLUTION.md as historical entries, "Migration Framework
  Plan" moved to TODO.md "What's not in main yet". Doc now focuses on
  current architecture (schema, resolver, importer pattern,
  per-language source choices) rather than mixing architecture + plan
  + decisions.
- [`README.md`](../README.md) Project Structure expanded with 5
  previously-missing cmd binaries (`importkotus`, `importud`,
  `scrapegutenberg`, `fetchfrequency`, `genlemmatizertables`) and the
  `pkg/lemmatizer-fi-et` package. Custom parser description now
  mentions FST candidate scoring (post-#127). `localdata/` bullet
  expanded to enumerate every gitignored runtime artifact.

**See also:** DECISIONS.md Decisions 17 (ESTONIAN_LEXICAL_PLAN consolidation)
and 18 (IMPLEMENTATION.md split) - both also record the 2026-05-07 AM PR #135
that did the first round of doc consolidation.

## 2026-05-07 AM - Doc parity sweep + 07k baseline freeze (PR #135)

Doc-parity sweep driven by an audit of all spine docs against the day's
PRs (#127–#134). Plus the `2026-05-07k-T0944Z` baseline freeze
(companion to the PARSER_EVOLUTION.md `2026-05-07k` row).

- Date headers refreshed in [`ARCHITECTURE.md`](../ARCHITECTURE.md),
  [`docs/FEATURES.md`](FEATURES.md),
  [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md).
- "FST step 5 fallback" description corrected (post-#127/#129) in
  [`docs/PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md),
  [`docs/SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md), and
  [`ARCHITECTURE.md`](../ARCHITECTURE.md) Custom parser bullet.
- [`ARCHITECTURE.md`](../ARCHITECTURE.md): removed obsolete
  `data/voikko/...` Voikko-seed paragraph; updated FI lexical pipeline
  status (Phases 1–3 shipped, Phase 4 superseded, Phase 5 partial);
  removed stale "in flight as #78"; added `cmd/importkotus`,
  `cmd/genlemmatizertables`, `cmd/fetchfrequency` to the cmd list.
- [`docs/PARSER_EVOLUTION.md`](PARSER_EVOLUTION.md): broken
  `FINNISH_LEXICAL_PLAN.md` link fixed to `LEXICAL_PLAN.md` (renamed
  in #112); `data/kotus/` path fixed to `localdata/kotus/`.
- [`docs/ESTONIAN_LEXICAL_PLAN.md`](LEXICAL_PLAN.md) merged into
  [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md) as the "Estonian-specific
  source choices and adapter contract" section. Original file deleted.
- [`docs/IMPLEMENTATION.md`](IMPLEMENTATION.md) split: "Suggest fix"
  UX → new [`docs/PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md);
  IMPLEMENTATION.md replaced with a redirect stub.
- [`IMPLEMENTATION_ANALYSIS.md`](../IMPLEMENTATION_ANALYSIS.md) gained
  a historical banner pointing readers to ARCHITECTURE.md and
  DECISIONS.md Decision 1.
- 2026-05-07k baseline freeze: 8 files under
  `docs/baselines/2026-05-07k-T0944Z-*` (FI + ET, JSON reports +
  summary markdown). `parsecore.ParserVersion` bumped to
  `2026.05.07k`.

**See also:** DECISIONS.md Decisions 16, 17, 18 (the three doc/code
decisions this PR enforced).

## 2026-05-07 - Single-folder data root + ET UD gold materialized

Consolidates every gitignored runtime data artifact under
[`localdata/`](../localdata/) so a single tarball captures the entire
bootstrap state. Materializes the ET UD parser-eval gold and the
FI/ET UD train splits that PR #113 (Plan C / PR 1) had documented but
not yet generated on the user's machine.

- New [`docs/data_enhancement.md`](data_enhancement.md): single-source-of-truth
  ledger of every gold/silver/dictionary corpus the project pulls in.
  Each row tracks source URL, license, size, path, added date, and
  last-refreshed date. Update on every import.
- Path consolidation:
  - `data/ud-cache/` → `localdata/ud-cache/`
  - `testdata/parser-eval/fi/gold-train/` → `localdata/parser-eval/fi/gold-train/`
  - `testdata/parser-eval/et/gold/ud-et-*.json` → `localdata/parser-eval/et/gold/`
  - `testdata/parser-eval/et/gold-train/` → `localdata/parser-eval/et/gold-train/`
  Committed FI dev/test gold under `testdata/parser-eval/fi/gold/` is
  unchanged (still byte-identical after re-import).
- [`scripts/fetch-and-import-ud.sh`](../scripts/fetch-and-import-ud.sh)
  writes to the new locations.
- [`scripts/parser-comparison.sh`](../scripts/parser-comparison.sh) and
  [`scripts/parser-comparison-et.sh`](../scripts/parser-comparison-et.sh)
  auto-discover from both `testdata/parser-eval/<lang>/gold/` and
  `localdata/parser-eval/<lang>/gold/`. Held-out discipline preserved
  (still excludes `*-dev-v*` and `gold-train/`).
- [`scripts/setup-local.sh`](../scripts/setup-local.sh) summary lists
  every `localdata/` subtree on completion and emits the bootstrap
  tar instruction (`tar czf finnestdb-bootstrap.tgz localdata/ finnestdb.db`).
- [`.gitignore`](../.gitignore) collapsed: the `localdata/` blanket rule
  covers everything; legacy `data/` paths kept as a belt-and-braces guard.
- [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) gains a "Single-folder
  bootstrap rule" section documenting the invariant.
- [`ARCHITECTURE.md`](../ARCHITECTURE.md) §Evaluation Stack treebank
  table updated with the new path strings; Estonian-EWT train count
  corrected (5,375 actual vs. 5,380 documented).
- Local gold available after this PR + a `make import-ud-gold` run:
  ~37k FI cases / 339k FI tokens, ~37.9k ET cases / 437k ET tokens -
  ~7.5× the cases and ~8.5× the tokens previously visible in `git`.

**Why now:** the previous layout had three gitignored data roots
(`localdata/`, `data/ud-cache/`, two carve-outs under
`testdata/parser-eval/`). Handing a teammate a "fast bootstrap" zip
required either three separate archives or a custom recipe that knew
the carve-outs. Consolidating to one root removes the foot-gun.

## 2026-05-07 - Runtime docs parity pass

Aligns user-facing docs with the E2E behavior report in
[`docs/qa-reports/2026-05-06-e2e-doc-behavior-report.md`](qa-reports/2026-05-06-e2e-doc-behavior-report.md).

- [`README.md`](../README.md) now distinguishes unknown-language advisory
  warnings from blocking Finnish/Estonian mismatch warnings in the language
  detection overview.
- [`docs/FEATURES.md`](FEATURES.md) now describes the signed-in browser Parse
  flow, clarifies that direct unauthenticated API parses are ephemeral
  development behavior, and frames multi-candidate deck cards as
  dictionary-coverage dependent rather than guaranteed for the `joon` example.

## 2026-05-06c - UD treebank gold expansion (Plan C / PR 1)

Lifts the parser-eval gold set from ~166 cases to ~14k cases (committed
FI) / ~22k cases (FI committed + ET local) by ingesting the published
Universal Dependencies treebanks for Finnish and Estonian.

- Added [`cmd/importud`](../cmd/importud/main.go): pure-Go CoNLL-U →
  parser-eval gold JSON converter. Skips MWT range rows and elliptical
  nodes; preserves full UD FEATS string in a new `feats` field on each
  token (forward-compat for the planned per-attribute eval); projects
  `Case=Xxx` into the legacy `grammar_label` field for back-compat with
  the existing case-only metric.
- Added [`scripts/fetch-and-import-ud.sh`](../scripts/fetch-and-import-ud.sh):
  clones each UD treebank under `data/ud-cache/` (gitignored) and runs
  the importer over each train/dev/test split.
- Added Makefile targets `make import-ud-gold-fi`, `make
  import-ud-gold-et`, `make import-ud-gold` (both).
- New committed FI gold (CC BY / CC BY-SA): ~9.8k cases / ~86k tokens
  across UD-Finnish-TDT/FTB/PUD/OOD test+dev splits.
- New local-only ET gold (CC BY-NC-SA - gitignored under
  `testdata/parser-eval/et/gold/ud-et-*.json`): ~8k cases / ~115k
  tokens across UD-Estonian-EDT/EWT test+dev.
- Train splits go under `testdata/parser-eval/{fi,et}/gold-train/`
  (gitignored) so headline `make compare-parsers` runs don't get
  bloated by 30k-sentence files. Used for OOV/coverage analysis with
  explicit `-dataset` flags.
- Held-out discipline: [`scripts/parser-comparison.sh`](../scripts/parser-comparison.sh)
  and [`scripts/parser-comparison-et.sh`](../scripts/parser-comparison-et.sh)
  default discovery now excludes `*-dev-v*` files. Test sets are the
  held-out anchor; dev is for per-commit watching (run explicitly with
  `-dataset`).
- [`ARCHITECTURE.md`](../ARCHITECTURE.md) §Evaluation Stack updated with
  per-treebank table, license info, and held-out workflow.

**Why now:** the old gold was 22 cases on fi-manual-v1, 4 on
et-manual-v1. Any number computed on a 22-case set is one bad sentence
away from a 4.5pp swing. UD gives us train/dev/test splits with
human-checked morphology; we pay nothing to use them.

**FST migration link:** still on the roadmap - see PRs
[#106](https://github.com/sagarinbabel/finnestdb/pull/106) /
[#107](https://github.com/sagarinbabel/finnestdb/pull/107). The expanded
gold makes that migration's regression checks meaningful (a 3pp lemma
gain on 22 cases is noise; on 86k tokens it's signal).

## 2026-05-06b - Eval harness parity + grammar-label stopgap

Two changes to the parser-evaluation pipeline, plus a recorded decision on
how *not* to fix grammar accuracy.

- **Always benchmark against the analyzer baseline.**
  [`scripts/parser-comparison.sh`](../scripts/parser-comparison.sh) now
  *requires* omorfi for Finnish; [`scripts/parser-comparison-et.sh`](../scripts/parser-comparison-et.sh)
  requires estnltk/Vabamorf for Estonian. Both fail with `exit 2` and a
  setup hint when the analyzer is missing. A `--allow-missing-baseline`
  flag remains for ad-hoc local experiments - committed reports must
  include the analyzer column.
  - Why: dict-only basic/custom numbers were being read in isolation,
    masking that grammar accuracy was 0% across all FI and ET datasets in
    [`docs/baselines/2026-05-06b-summary.md`](baselines/2026-05-06b-summary.md).
    The analyzer column is the upper bound; without it there is no way to
    tell whether 88% lemma is "good enough" or a regression. Locking it in
    by default closes the eval-harness gap.
- **Stopgap grammar-label attachment on dict hits.**
  [`internal/store/dict.go`](../internal/store/dict.go) `BatchLookupForms`
  now runs the case-suffix matcher additively when a direct dict hit
  succeeds (custom mode only), attaching a case label when the
  suffix-strip lemma matches the dict lemma exactly. Previously
  `grammar_label` was empty on every direct hit, which is why grammar
  accuracy was structurally 0%. Stopgap; will be removed once the FST
  runtime in [`pkg/lemmatizer-fi-et/`](../pkg/lemmatizer-fi-et/) emits
  FEATS for direct hits - see PRs
  [#106](https://github.com/sagarinbabel/finnestdb/pull/106) /
  [#107](https://github.com/sagarinbabel/finnestdb/pull/107).
- **Recorded the decision not to extend the suffix table.**
  [`docs/DECISIONS.md`](DECISIONS.md) Decision 5 explains why suffix-table
  extension is the wrong investment direction (stem alternation,
  suffix-shaped lemmas, ambiguity, compound interaction) and why the FST
  runtime is. TODO items #15 (ternary compounds) and #16 (consonant
  gradation) are gated behind the FST migration as a result.

## 2026-05-07 - 3-column comparison reports + bootstrap CIs (Plan C / PR 2)

Restructures `cmd/parser-compare` so committed comparison reports answer the
right question by default: "did *our* parser regress against the analyzer
upper bound?" Three-column headline (custom-prev / custom-now / analyzer)
replaces the legacy "every parser side-by-side" framing, with case-level
bootstrap CIs so 22-case noise can no longer be misread as signal.

- Added `-baseline-dir` flag to [`cmd/parser-compare`](../cmd/parser-compare/main.go).
  When set, each "now" report is paired by dataset name with a prior report
  in that directory; the headline becomes `(custom-prev, custom-now, Δ,
  analyzer)`. Without `-baseline-dir` the legacy table is the only output
  (back-compat).
- Added `-bootstrap N` flag (default 1000). Each accuracy cell shows
  `82.3% ±0.4` - half the 95% case-level bootstrap CI width. Set
  `-bootstrap 0` to disable. Deterministic seed by default so committed
  reports diff cleanly.
- Added `-main-parser` flag (default `custom`) to control which parser is
  treated as "now" in the headline.
- Legacy "all parsers" table moves to an appendix when `-baseline-dir` is
  set; remains the default output otherwise.
- 4 unit tests covering the per-case stats extractor, bootstrap
  half-width on uniform vs heterogeneous accuracy, and analyzer-parser
  detection.

**Why now:** the eval harness changes in
[#109](https://github.com/sagarinbabel/finnestdb/pull/109) and
[#113](https://github.com/sagarinbabel/finnestdb/pull/113) gave us reliable
gold + always-present analyzer columns. The remaining gap was the report
structure itself: today's reports compare basic-vs-custom head-to-head, but
the meaningful comparison is custom-prev vs custom-now (did we improve)
against the analyzer (how far is the upper bound). Bootstrap CIs make it
honest - a 2.2pp gain on 22 cases stops being headline-worthy.

**Generated-table migration link:** future production FI/ET morphology
tables will reuse the same `-baseline-dir` machinery. Gold case files
already carry a `feats` field after PR #113, so the per-attribute
extension is purely on the report side.

## 2026-05-07 - Gutenberg-FI silver corpus scraper (Plan C / PR 3)

First silver-tier corpus source. Scrapes public-domain Finnish books from
Project Gutenberg (https://www.gutenberg.org/ebooks/search/?query=l.fi),
strips PG boilerplate, saves cleaned text + a JSONL manifest under
`localdata/silver-fi/` (gitignored).

- Added [`cmd/scrapegutenberg`](../cmd/scrapegutenberg/main.go): polite
  HTTP scraper (1.5s between requests, transparent User-Agent, single
  connection). Tries cache-epub UTF-8 → files-0 UTF-8 → files-8
  ISO-8859-1 in order; decodes ISO-8859-1 via golang.org/x/text/encoding;
  strips Project Gutenberg "*** START OF" / "*** END OF" boilerplate;
  rejects non-Finnish leaks (English-authored books with l.fi metadata)
  via an ä/ö frequency + common-particle heuristic.
- Added `localdata/silver-fi/` with 14 books (~511k
  tokens) on first run: Kalevala, Aleksis Kivi (Seitsemän veljestä),
  Minna Canth, Aleksis Kivi-era prose, Finnish translations of Jack
  London / Molière / Drachmann, plus modern works (Pekkarinen,
  Haanpää, Järnefelt). Manifest at
  `localdata/silver-fi/manifest.jsonl`
  records id, title, author, source URL, encoding, fetched_at, token
  count per book.
- Added Makefile target `make scrape-gutenberg-fi` (overridable
  `TARGET_TOKENS=N`).
- Idempotent - already-fetched books are skipped on re-run.

**Why now:** with ~900k UD gold tokens and ~500k Gutenberg silver
tokens, we're at the corpus scale where bootstrap CIs from Plan C / PR 2
are tight (±0.4–0.6pp) and "did our parser regress" is answerable
with confidence. Next silver sources (runosto.net for poetry, ET
Wikisource for Estonian, Wikipedia FI/ET for breadth) follow the same
pattern; this PR establishes the scaffolding.

**Silver tagging deferred:** the actual morphological annotation
(Voikko + Omorfi agreement filter for FI; Vabamorf + Ekilex for ET)
ships in Plan C / PR 4. This PR delivers the raw corpus only.

**Generated-table migration link:** the silver tagger can use future
production generated morphology tables as one half of the agreement
filter. Omorfi via the Python adapter remains the other FI comparison
path.

## 2026-05-06 - Numeric-hyphen tokenization (FI + ET)

Surfaced by manual testing on Estonian text containing `65-aastane`. The
shared Rust tokenizer at [`parser/src/lib.rs`](../parser/src/lib.rs) was
keeping `65-aastane`, `1990-luvulla`, `250 000`, etc. as opaque single tokens
or pairs of NOUN stubs, with no `NUM` POS for digit forms. Confirmed Finnish
had the identical bug (the tokenizer ignores its `_lang` parameter).

Following [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) on
shared error categories and shared-pipeline investments, the fix is four
tokenizer-only rules (R1–R4) - no per-language rule tables.

- Added [`docs/qa-reports/2026-05-06-numeric-hyphen-tokenization.md`](qa-reports/2026-05-06-numeric-hyphen-tokenization.md):
  bug repro, root cause, R1–R4 with worked examples in both languages,
  measured impact (zero regression on all 6 existing gold datasets), and
  follow-ups.
- Added Decision 6 to [`docs/DECISIONS.md`](DECISIONS.md) recording that
  numeric-hyphen handling lives in the shared tokenizer rather than in
  language-specific rule tables.

## 2026-05-06 - Lexical pipelines: ET ships, FI plan locks

Locks the dictionary layer as multi-source with row-level provenance and
priority, ships the Estonian source-data pipeline end-to-end, and stages
the Finnish equivalent at the schema layer with a fully scoped plan.

- Added `docs/ESTONIAN_LEXICAL_PLAN.md` (later consolidated into
  [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md)):
  EstNLTK/Vabamorf as the analyzer baseline, EKI/Ekilex as the
  sanctioned lexical-data source, attribution requirements per import,
  parity correction flow shared with Finnish.
- Added [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md): Kotus
  sanalista + Voikko (offline paradigm computation) + kaikki.org
  Wiktionary as the three open-source pillars; Kielitoimiston
  deliberately excluded; five-phase rollout (Phase 1 schema delta
  shipped, Phases 2–5 staged).
- Added "Making it AI native" section in
  [`docs/ideas.md`](ideas.md): five-phase roadmap for layering Claude
  features (grounded `/api/explain`, agentic tutor, LLM morphology
  fallback, embeddings, optional speech) onto the rule-based pipeline.
- Updated [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md)
  with EstNLTK adapter wiring and the dictionary-attribution metadata
  contract.

Locked decisions captured in this round of docs:

- The dictionary tables carry row-level `source` and `source_priority`,
  not a single dominant source. Per-language priority order is
  `custom_overrides` (1000) > rich generators/curated (20–30) > kaikki
  (10), with ties broken deterministically.
- Finnish paradigm coverage is *computed* from Kotus class + Voikko
  rather than scraped, and ships as a static JSONL artifact under
  `data/voikko/` rather than via runtime libvoikko.
- Translations and definitions live in dedicated tables (not
  `lemmas.gloss`); schema groundwork (`paradigm_class`, `feats`,
  `translations`, `definitions`) ships before the FI adapters that
  populate them.
- Schema migrations stay on the established idempotent
  `ALTER TABLE`/`CREATE TABLE IF NOT EXISTS` pattern with grouped
  `EnsureXxx` helpers in `internal/store/db.go`. A real migration
  framework is deferred until non-additive migrations or merge-conflict
  pressure force the move.
- Wikisanakirja (via kaikki.org's Finnish edition) covers monolingual
  FI definitions for alpha; Kielitoimiston is not bulk-imported.
- The Ekilex pipeline is four binaries with distinct roles:
  `cmd/fetchekilex` (resumable scrape against `/api/word/details`),
  `cmd/reduceekilex` (golden-tested reduction into sharded JSONL/TSV),
  `cmd/importekilexdetails` (bulk-load the reduced data drop into
  `lemmas`/`forms`), and `cmd/importekilex` (the lighter public-headword
  snapshot loader). `cmd/importdict -source-key ekilex` remains for
  on-demand API queries.
- Ambiguous surface forms get one row per `(lemma, pos)` candidate.
  `forms` PK is `(form, lang, lemma, pos)` and the deck-ingest path
  uses `BatchLookupAllForms` so, when the dictionary has multiple
  candidates for a surface form such as ET `joon`, the saved deck gets
  one occurrence row (and one card) per dict candidate; the parser's
  single pick is only used when the dict is silent. Migration handled by
  `EnsureMultiLemmaSchema` / `rebuildIfLegacyKey` in `internal/store/db.go`.

## 2026-05-01 - Architecture diagram and subsystem versioning

Separates architecture visibility from subsystem behavior tracking.

- Added a Mermaid architecture diagram to [`ARCHITECTURE.md`](../ARCHITECTURE.md)
  with explicit parser and deck/review system boundaries.
- Added [`docs/SYSTEM_VERSIONING.md`](SYSTEM_VERSIONING.md) to track parser
  behavior, parser baselines, deck review behavior, API contracts, and data
  schema versions independently.
- Updated [`docs/architecture.md`](architecture.md) to point to the canonical
  architecture and subsystem-versioning docs.

## 2026-04-29 - Consumer alpha execution plan

Locks the alpha as a consumer language-learning product with
Finnish/Estonian parity, an admin-only parser workbench, a logged-in
correction loop, and dual evaluation tracks.

- Appended the execution plan to [`TODO.md`](../TODO.md) under the
  "2026-04-29 - Consumer alpha execution plan" section.
- Added [`docs/FEATURES.md`](FEATURES.md): user-perspective product
  description, learn-before-reading framing, leverage/comprehension
  concept, mobile direction, and the technology differentiators as
  described at the time. The autoresearch idea mentioned in that round
  was later parked as post-live exploration; see the 2026-05-07 PM
  guardrail entry above.
- Added [`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md):
  how Finnish and Estonian improve together via shared infrastructure,
  shared evaluation, and a shared error taxonomy without copying
  morphology blindly between languages.
- Added this changelog ([`docs/CHANGELOG.md`](CHANGELOG.md)).
- Added "Current as of 2026-04-29" headers and changelog cross-links
  to the active authoritative docs:
  [`ARCHITECTURE.md`](../ARCHITECTURE.md),
  [`TODO.md`](../TODO.md),
  [`docs/IMPLEMENTATION.md`](IMPLEMENTATION.md),
  [`docs/DECISIONS.md`](DECISIONS.md),
  [`docs/srs-deck-spec.md`](srs-deck-spec.md).

Locked decisions captured in this round of docs:

- The product is a consumer language-learning app, not a parser
  workbench. The workbench remains, but admin-only.
- Logged-in users get a lightweight parse-inspection view and may
  submit parser corrections. Anonymous correction submission is out of
  scope for alpha.
- Cards are global. Deck deletion does not erase learning state.
- Two evaluation tracks: Track A (offline gold + external benchmark)
  and Track B (live accepted-correction metrics).
- Finnish external benchmark: Omorfi.
- Estonian external benchmark: EstNLTK / Vabamorf.
- [`docs/baselines/`](baselines/) is the single canonical frozen
  baseline store.
- Cross-language improvement is shared at the
  infrastructure/evaluation/error-taxonomy layer, not by copying
  morphology rules between languages.

Companion docs that will be added in later PRs and are referenced from
the execution plan but not yet present:

- `docs/SYSTEM_OVERVIEW.md`
- `docs/PARSER.md`
- `docs/PARSER_FEEDBACK_LOOP.md`
- `docs/EVAL_AND_CI.md`
- `docs/KNOWN_WORDS.md`
- `docs/MICHAEL_TODO.md`
- `docs/SECURITY_REVIEW_ALPHA.md`
