_Current as of 2026-07-03 - see [CHANGELOG.md](CHANGELOG.md) for revisions._

# FinnEst Features

FinnEst is a consumer language-learning product for Finnish and Estonian
readers. It is not a parser workbench. The workbench still exists, but it
is an internal admin surface, not the product users see.

This document describes the alpha product from a user perspective.

## Language Status

Finnish and Estonian have equal product status. The alpha should not present
either language as experimental, secondary, or best-effort. If one language has
a concrete parser, catalog, corpus, or UX gap, treat it as a specific readiness
issue to fix or document internally, not as a public demotion of that language.

## What FinnEst Is

FinnEst helps you read real Finnish and Estonian text by letting you
pre-learn the vocabulary that actually appears in what you want to read.

Instead of grinding generic frequency lists, you paste the text you want
to read - an article, a chapter, a song - and FinnEst currently:

- breaks it into the unique words it contains
- shows dictionary-backed lemmas, forms, definitions, examples, and token counts
- lets you manually mark parsed rows known or ignored
- lets you save the parsed vocabulary as a study deck
- gives you spaced-repetition review for those words
- tracks known-word and due-review counts
- imports known words from paste, simple files, or local Anki via AnkiConnect

The pitch: pre-mine vocabulary before reading, so the reading itself is
enjoyable instead of a dictionary lookup grind.

## Anonymous Parser Demo

Shipped 2026-07-04. Unsigned visitors can experience the parser before creating
an account:

1. paste a Finnish or Estonian text;
2. parse it;
3. get the parsed word list; and
4. explore that list.

That anonymous surface is intentionally narrow and stateless. It proves parser
quality, but it does not create a learner memory. Saving decks, reviewing,
importing known words, marking known/ignored state, submitting parser feedback,
viewing history, and account settings require sign-in.

Anonymous parsing uses a stricter, configurable text-size cap
(`FINNESTDB_ANON_MAX_CHARS`, default 300,000 characters) enforced server-side
before any parser work; the signed-in cap stays 1,500,000. Over-cap anonymous
requests get a 4xx error naming the limit and a sign-up CTA.

## How You Learn With FinnEst

The signed-in core loop is `paste -> inspect -> correct -> deck -> review`.

1. Sign up or sign in, then paste or upload Finnish or Estonian text, or start
   from a curated embedded text. Inspect parses are ephemeral until you save a
   deck or submit parser feedback; see
   [What We Store During Alpha](#what-we-store-during-alpha).
2. Inspect the parsed result. It opens on the **Read** tab - the source text
   itself, with every parsed word colored by whether you already know it,
   already study it, or have yet to learn it. Tap a word for its meaning and to
   mark it known, add it to study, or ignore it. A **Words** tab holds the full
   table (every unique word, its lemma, its meaning) for scanning and export.
3. Correct the parser if it gets a word wrong (logged-in users only).
4. Save the parsed vocabulary as a deck.
5. Review the deck with spaced repetition.
6. Return to the source text with the reviewed vocabulary in context.

The embedded-text catalog uses fixed Easy / Medium / Hard labels when the app
does not know the learner's vocabulary yet. When known-word data exists, the
catalog should also show personalized fit, such as known-token coverage, so a
learner can pick a text that is challenging but not opaque.

Cards are global. Today the implementation treats a learned word as the same
`(lemma, POS)` across decks. The public-alpha target is surface-form-in-context
review cards: preserve exact known surface forms as first-class learner
evidence, then use lemma/POS resolution as derived data for coverage and card
selection. Deleting a deck does not erase what you have learned - it only
removes that particular source material.

## Language Selection

FinnEst uses a hybrid language policy so the common path stays fast without
silently parsing under the wrong language.

- High-confidence pasted or file-loaded text warns and blocks parsing when the
  detected language differs from the selector.
- The learner stays in control of the active language and switches explicitly
  before parsing under the detected language.
- Unknown-language warnings are advisory. You can still parse text that does
  not contain enough Finnish or Estonian signal.

## Current Inspect Results

Inspect currently shows dictionary coverage, not personalized comprehension.
For each parsed text, the app shows:

- unique lemmas and parts of speech
- forms found in the source text
- English definitions when present in the dictionary
- grammar labels when enrichment inferred them
- example sentences and token counts
- row-level Known / Ignore and correction actions for logged-in users
- a **coverage reveal** above the table (shipped 2026-07-04): for signed-in
  learners, "You already know X% of this text" plus a projection "Learn the top
  N words → Y%"; for anonymous visitors, the frequency framing "The N most
  frequent words carry Z% of it". X/Y/Z reuse the saved-deck comprehension
  token-mass formula (known-or-ignored counts as covered) - see USER_FLOWS §2.

Beyond the coverage reveal, automatic known/new labelling across the table and
leverage-based row ordering remain planned product work, not part of the
current Inspect result.

## Words With Multiple Senses

Some words in Estonian and Finnish look identical but can represent more than
one dictionary lemma. For example, Estonian **joon** can be the noun "line"
(`joon` SgN) or the 1st-person-singular form of the verb **jooma** ("to
drink") - morphology alone can't always tell which sense the writer meant.

FinnEst creates one card for the parser-selected sense when you save a parse.
The **Multiple possible meanings** flow shows other supported candidates; a
learner must explicitly choose an alternate before it becomes another card.
This keeps inflectional dictionary noise from turning one source token into a
set of unrelated study words while preserving intentional homograph study.

**Deck expansion vs. the meaning-check candidate set.** Two candidate views
exist and are deliberately kept separate. The **deck / import path** records
the parser-selected sense, so saving a deck creates one stable card per token.
The **meaning-check** candidate set (the **Multiple possible meanings** flow)
additionally merges FST-known
homograph readings the dictionary's form table omits - the classic Finnish
cross-POS homographs `kuusi` (NOUN spruce / NUM six), `tuli` (NOUN fire /
`tulla` VERB), and `voi` (NOUN butter / `voida` VERB) each store only one
reading in the imported dictionary, so their second sense would otherwise be
unofferable. Merging these FST senses raised measured FI ambiguity candidate
inclusion from 72.9% to 95.8% (see
[`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) §Ambiguity) without
changing deck word counts.

## Progress Tracking

You can see, across the whole app:

- known vocabulary in each language. Current counts are lemma-backed; the target
  model is surface-first known vocabulary.
- cards in active learning
- due review count
- per-deck known/unique counts

Today progress is tracked at the lemma level, not at the deck level, because
your knowledge of a word is a property of you, not of any single text. Public
alpha should move that learner state to surface-form cards and known surface
forms, with lemma-level views derived where useful. Review maturity is separate:
a mature FSRS card may inform retained-coverage estimates, but it should not
silently become a known-word claim.

When a known-word import contains a same-looking word with multiple supported
meanings, the app should not ask for abstract disambiguation upfront. It should
confirm the meaning later in sentence context. If the learner chooses **Study
this meaning** in parse results, the UI must say that this creates a review card
when the deck is saved. In saved deck or review contexts, it creates or keeps
the review card immediately. If the parser cannot confidently choose the
intended meaning, the UI should show **Multiple possible meanings** with
per-candidate known/study actions. **None of these looks right** is parser
feedback for bad analysis, not a study choice.

## Mobile Direction

FinnEst is a single responsive web app. Reviews and reading happen on
phones often, so the UI is designed to be usable at 375 px wide. The
alpha targets web first; native packaging is out of scope.

## Roles in the Product

- **Anonymous visitor**: can paste text, parse it, get a parsed word list, and
  explore that list. They can create an account or sign in to save anything.
  Decks, Review, known/ignored state, imports, corrections, history, and account
  settings require sign-in.
- **User**: can paste text, see a lightweight parse-inspection view,
  import known words, create and review decks, and submit parser
  corrections.
- **Admin**: everything a user can do, plus the parser workbench, the
  feedback triage queue, and weekly parser-quality reporting.

The full parser workbench is intentionally admin-only in alpha. End
users get a lightweight inspection view that is enough to read parse
output and submit corrections, without exposing internal parser knobs.

Anonymous correction submission is out of scope for alpha.

## What We Store During Alpha

- In the browser alpha, open signup is allowed, and the anonymous parser demo is
  a public product surface. Direct unauthenticated `POST /api/parse` is allowed
  for ephemeral paste/parse/list/explore use, guarded by rate limits and a
  stricter text-size cap than signed-in parsing, and does not return a stored
  parse ID.
- When you are **signed in**, the text you paste into Inspect is ephemeral by
  default. `/api/parse` returns results without creating a stored parse ID.
- Source text is stored only when you make the parse durable: saving it as a
  deck, or submitting parser feedback. Feedback stores the original context so
  admins can review the correction. Feedback comes in two forms: a concrete
  correction (base form + part of speech), or a **flag-only** report - "this
  looks wrong, I don't know the fix" - which stores the flagged word and context
  without a proposed answer (shipped 2026-07-04).
- Raw retained source text is kept for 30 days, then purged by the
  `make purge-parse-context` retention run. Decks, cards, feedback rows, and
  admin review status remain; the raw pasted text no longer appears in history
  or admin context after purge.
- The History page lists retained parse sessions and lets you delete one
  session or all retained sessions. Deleting a parse session removes that
  retained source context and parser feedback tied to it; saved decks remain.
- Accepted lemma/POS corrections write `custom_overrides` lexical rows after
  admin approval, so the same surface can change subsequent parser output.
  Accepted grammar labels become UD FEATS on the override row, acceptance is
  eval-gated against frozen gold analyses, and repeat corrections auto-queue
  as gold candidates (shipped 2026-07-02).
- Admin-quarantined faulty study content quietly disappears from learner
  review/new-card queues; active learner-facing counts and comprehension stats
  exclude it, while full report/fix traceability is admin-facing. Restoring a
  fixed issue returns the content with its scheduler state intact (shipped
  2026-07-04, Phase 1c).
- Account deletion is self-serve: the Languages page has an Account section
  with a "Delete account" control behind a confirmation dialog. Deletion
  removes the account and retained user data server-side, including decks,
  parse sessions, parser feedback, review cards, sessions, and known/ignored
  word lists.
- We do not sell, share, or use your pasted text to train external models.

## Technology Differentiators

FinnEst is positioned around four technical bets:

- **Fast parser**: a custom Rust analyzer optimized for the
  paste-to-deck flow, not for academic completeness.
- **Benchmarked quality**: every release is measured against an
  external reference - Omorfi for Finnish, EstNLTK / Vabamorf for
  Estonian - and against frozen gold datasets in `docs/baselines/`.
- **User correction loop**: real users submit parser corrections from inspect
  and deck-detail result rows; admins triage them; accepted lemma/POS *and
  grammar-label* corrections feed authoritative `custom_overrides` lexical
  rows (grammar labels become UD FEATS on the override) that change future
  parser output. Two safety rails close the loop (2026-07-02): acceptance is
  refused when the correction contradicts the frozen gold evaluation sets
  (`make import-gold-surfaces` loads the guard data), and a correction
  independently accepted for 3+ distinct users is auto-queued as a gold-case
  candidate for manual promotion (`make export-gold-candidates`).
- **Post-live improvement ideas**: once the app is shipped and live, the
  dictionary/lemma layer can absorb new sources of evidence into a
  canonical lexical knowledge graph over time. `AUTORESEARCH.md` is an
  idea parking lot, not current product scope.
- **Inflected-form-aware frequency and comprehension prediction**:
  unlike most public Finnish/Estonian frequency lists (which rank
  lemmas), FinnEst measures inflected-form frequency directly,
  because that is the unit a learner actually has to recognize when
  reading running text. Our 2026-05-07 baseline measurement found
  that the top-1000 inflected forms cover ~65–73% of subtitle text
  but only ~40–43% of written news/literature in both languages -
  register effects dwarf the FI-vs-ET gap. This drives how we
  calibrate comprehension prediction. See
  [`docs/CROSS_LANGUAGE_STRATEGY.md` "Measurable
  Divergences"](CROSS_LANGUAGE_STRATEGY.md) and
  [`docs/FREQUENCY_BASELINES.md`](FREQUENCY_BASELINES.md).

## Two Evaluation Tracks

Parser quality is measured on two tracks, and both must look healthy
before a release goes out.

- **Track A (offline)**: gold datasets plus the external benchmark for
  each language. Frozen reports live under `docs/baselines/`.
- **Track B (live)**: accepted-correction rates from real usage, segmented by
  language and parser mode. The weekly admin report is planned alongside the
  correction-overlay work.

Track A tells us whether we regressed against fixed reference data.
Track B tells us whether real users are actually being helped.

## Out of Scope for Alpha

- Anonymous full parser-correction submission.
- Native mobile apps.
- A user-facing parser workbench.
- A polished analytics dashboard for Track B (alpha ships a weekly
  admin report instead).
- Background/async deck creation (alpha parses synchronously; see
  `TODO.md` for the deferred work).
