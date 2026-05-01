_Current as of 2026-04-29 — see [CHANGELOG.md](CHANGELOG.md) for revisions._

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

The core loop is `paste -> inspect -> correct -> deck -> review`.

1. Paste or upload text in Finnish or Estonian (stored on your account when
   signed in; ephemeral when not — see
   [What We Store During Alpha](#what-we-store-during-alpha)).
2. Inspect the parsed result: every unique word, its lemma, its meaning.
3. Correct the parser if it gets a word wrong (logged-in users only).
4. Save the parsed vocabulary as a deck.
5. Review the deck with spaced repetition.
6. Return to the source text with the reviewed vocabulary in context.

Cards are global. A word you have learned in one deck stays learned in
every other deck that contains the same lemma. Deleting a deck does not
erase what you have learned — it only removes that particular source
material.

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
  sign in. Cannot create decks, review, or submit corrections.
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

- When you are **not signed in**, parses are ephemeral. We do not store your
  pasted text on our servers.
- When you are **signed in**, the text you paste into Inspect is stored with
  your account. We do this so parser corrections can keep their original
  context and so a parse-history UI can exist later.
- Submitting a correction is the only way today to surface a parse to admins.
  Accepted corrections are used to improve parser quality.
- We do **not** yet have a delete-my-parse-history button. That is a planned
  follow-up. Until it exists, do not paste anything while signed in that you
  would not want stored.
- We do not sell, share, or use your pasted text to train external models.

## Technology Differentiators

FinEstDB is positioned around four technical bets:

- **Fast parser**: a custom Rust analyzer optimized for the
  paste-to-deck flow, not for academic completeness.
- **Benchmarked quality**: every release is measured against an
  external reference — Omorfi for Finnish, EstNLTK / Vabamorf for
  Estonian — and against frozen gold datasets in `docs/baselines/`.
- **User correction loop**: real users submit parser corrections from
  the inspection view; admins triage them; accepted corrections feed
  live quality metrics and future parser improvements.
- **Future autoresearch**: the dictionary/lemma layer is designed so
  that new sources of evidence can be merged into a single canonical
  lexical knowledge graph over time. See `AUTORESEARCH.md`.

## Two Evaluation Tracks

Parser quality is measured on two tracks, and both must look healthy
before a release goes out.

- **Track A (offline)**: gold datasets plus the external benchmark for
  each language. Frozen reports live under `docs/baselines/`.
- **Track B (live)**: accepted-correction rates from real usage,
  segmented by language and parser mode, surfaced in a weekly admin
  report.

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
