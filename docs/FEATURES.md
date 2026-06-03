_Current as of 2026-05-07 — see [CHANGELOG.md](CHANGELOG.md) for revisions._

# FinEstDB Features

FinEstDB is a consumer language-learning product for Finnish and Estonian
readers. It is not a parser workbench. The workbench still exists, but it
is an internal admin surface, not the product users see.

This document describes the alpha product from a user perspective.

## What FinEstDB Is

FinEstDB helps you read real Finnish and Estonian text by letting you
pre-learn the vocabulary that actually appears in what you want to read.

Instead of grinding generic frequency lists, you paste the text you want
to read — an article, a chapter, a song — and FinEstDB currently:

- breaks it into the unique words it contains
- shows dictionary-backed lemmas, forms, definitions, examples, and token counts
- lets you manually mark parsed rows known or ignored
- lets you save the parsed vocabulary as a study deck
- gives you spaced-repetition review for those words
- tracks known-word and due-review counts

The pitch: pre-mine vocabulary before reading, so the reading itself is
enjoyable instead of a dictionary lookup grind.

## How You Learn With FinEstDB

The signed-in core loop is `paste -> inspect -> correct -> deck -> review`.

1. Sign in, then paste or upload text in Finnish or Estonian. Inspect parses
   are ephemeral until you save a deck or submit parser feedback; see
   [What We Store During Alpha](#what-we-store-during-alpha).
2. Inspect the parsed result: every unique word, its lemma, its meaning.
3. Correct the parser if it gets a word wrong (logged-in users only).
4. Save the parsed vocabulary as a deck.
5. Review the deck with spaced repetition.
6. Return to the source text with the reviewed vocabulary in context.

Cards are global. A word you have learned in one deck stays learned in
every other deck that contains the same lemma. Deleting a deck does not
erase what you have learned — it only removes that particular source
material.

## Language Selection

FinEstDB uses a hybrid language policy so the common path stays fast without
silently parsing under the wrong language.

- High-confidence pasted or file-loaded text auto-switches to Finnish or
  Estonian when the detected language differs from the selector.
- Manually typed text and later selector changes keep the explicit guardrail:
  if selected and detected languages conflict, parse is blocked until you
  switch to the detected language.
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

Automatic known/new labelling, comprehension estimates, and leverage-based
ordering are planned product work, but are not part of the current Inspect
result.

## Words With Multiple Senses

Some words in Estonian and Finnish look identical but can represent more than
one dictionary lemma. For example, Estonian **joon** can be the noun "line"
(`joon` SgN) or the 1st-person-singular form of the verb **jooma** ("to
drink") — morphology alone can't always tell which sense the writer meant.

When the dictionary contains multiple candidates for a surface form,
FinEstDB creates a card for each candidate sense when you save that parse to
a deck, and the deck's word count reflects all candidates. You review them
independently; if one sense is irrelevant for your text, mark it known or
ignored. This behavior is dictionary-coverage dependent: if the current
dictionary only has one candidate for a surface form, only one card is
created.

If the dictionary has no entry for a word, the parser's single best guess
is used and only one card is created. The dictionary is only authoritative
about ambiguity when it actually knows the word.

## Progress Tracking

You can see, across the whole app:

- known lemmas in each language
- cards in active learning
- due review count
- per-deck known/unique counts

Progress is tracked at the lemma level, not at the deck level, because
your knowledge of a word is a property of you, not of any single text.

## Mobile Direction

FinEstDB is a single responsive web app. Reviews and reading happen on
phones often, so the UI is designed to be usable at 375 px wide. The
alpha targets web first; native packaging is out of scope.

## Roles in the Product

- **Anonymous visitor**: can see the landing/product explanation and
  sign in. Browser Parse, Decks, Review, and corrections require sign-in.
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

- In the browser alpha, the signed-in app remains the primary product loop.
  Direct unauthenticated `POST /api/parse` is intentionally allowed for
  ephemeral inspect/discovery use, guarded by rate limits, and does not return
  a stored parse ID.
- When you are **signed in**, the text you paste into Inspect is ephemeral by
  default. `/api/parse` returns results without creating a stored parse ID.
- Source text is stored only when you make the parse durable: saving it as a
  deck, or submitting parser feedback. Feedback stores the original context so
  admins can review the correction.
- Raw retained source text is kept for 30 days, then purged by the
  `make purge-parse-context` retention run. Decks, cards, feedback rows, and
  admin review status remain; the raw pasted text no longer appears in history
  or admin context after purge.
- The History page lists retained parse sessions and lets you delete one
  session or all retained sessions. Deleting a parse session removes that
  retained source context and parser feedback tied to it; saved decks remain.
- Accepted lemma/POS corrections write `custom_overrides` lexical rows after
  admin approval, so the same surface can change subsequent parser output.
  Grammar/FEATS corrections and eval-gated promotion remain future work.
- Account deletion removes the account and retained user data server-side,
  including decks, parse sessions, parser feedback, review cards, sessions,
  and known/ignored word lists.
- We do not sell, share, or use your pasted text to train external models.

## Technology Differentiators

FinEstDB is positioned around four technical bets:

- **Fast parser**: a custom Rust analyzer optimized for the
  paste-to-deck flow, not for academic completeness.
- **Benchmarked quality**: every release is measured against an
  external reference — Omorfi for Finnish, EstNLTK / Vabamorf for
  Estonian — and against frozen gold datasets in `docs/baselines/`.
- **User correction loop**: real users submit parser corrections from inspect
  and deck-detail result rows; admins triage them; accepted lemma/POS
  corrections feed `custom_overrides` lexical rows that can improve future
  parser output, while accepted-correction metrics guide later quality work.
- **Post-live improvement ideas**: once the app is shipped and live, the
  dictionary/lemma layer can absorb new sources of evidence into a
  canonical lexical knowledge graph over time. `AUTORESEARCH.md` is an
  idea parking lot, not current product scope.
- **Inflected-form-aware frequency and comprehension prediction**:
  unlike most public Finnish/Estonian frequency lists (which rank
  lemmas), FinEstDB measures inflected-form frequency directly,
  because that is the unit a learner actually has to recognize when
  reading running text. Our 2026-05-07 baseline measurement found
  that the top-1000 inflected forms cover ~65–73% of subtitle text
  but only ~40–43% of written news/literature in both languages —
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
