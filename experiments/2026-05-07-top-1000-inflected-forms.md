# Top-1000 inflected forms - seed deck research plan

_Created 2026-05-07. Ships a static, register-aware top-1000 inflected-form
table for Finnish and Estonian, primarily so a brand-new user with no
text in hand can start studying immediately ("Start with the top 1000
Finnish words" cold-start CTA in
[`docs/USER_FLOWS.md`](../docs/USER_FLOWS.md))._

## Goal

Produce, for each of FI and ET:

1. A ranked TSV of the top 1000 **inflected surface forms** with
   parser-resolved `lemma`, `pos`, `definition`, occurrence count, and
   source-of-evidence.
2. Per-register sub-rankings (subtitle / written / user-EPUB) so we can
   show learners a register match instead of a single averaged list.
3. A reproducible build script committed to the repo.
4. A ready-to-import seed deck consumable by the consumer-alpha
   cold-start path.

This is **not** the same as the
[`TODO.md` research goal "Discover the most-frequent inflected forms
in user-pasted text"](../TODO.md) - that one is the live, UGC-driven
ranking that updates as users paste. This experiment is the **static,
shippable baseline** that exists before any user has pasted anything.
The two are complementary: the static list bootstraps the cold-start
deck; the live list will eventually replace it (and let us measure the
register drift).

## Why this matters

The dictation flagged the cold-start problem: a learner who shows up
with no Anki export, no EPUB on hand, and no specific text to read has
nothing to do. Forcing them to type/paste sample text just to see the
product work is a known drop-off. A top-1000 deck gets them studying
within seconds of sign-up.

It is also the cheapest way to ship a comprehension-prediction signal
*per language* before the user has any deck. Once we have the list
plus its coverage curve, we can show on the dashboard "you know N of
the top 1000 Finnish forms - that's ~X% of typical written Finnish."

## Source corpora

### Already in the repo (or one command away)

- `localdata/frequency/fi/opensubtitles-2018-fi-50k.txt`
- `localdata/frequency/et/opensubtitles-2018-et-50k.txt`
- `localdata/frequency/fi/UD_Finnish-TDT-forms.tsv`
- `localdata/frequency/et/UD_Estonian-EDT-forms.tsv`

Pulled by `cmd/fetchfrequency`. License notes in
[`docs/FREQUENCY_BASELINES.md`](../docs/FREQUENCY_BASELINES.md). The
top-1000 coverage numbers are already measured there: 65.2% for FI
subtitles, 40.1% for FI written, 72.9% for ET subtitles, 42.9% for ET
written. We will reproduce them, not replace them.

### Already in the repo, not yet wired into this pipeline

- `localdata/fi-corpus/` - the Finnish corpus the project is already
  carrying. Tokenize through `parsecore` and aggregate.
- `localdata/kaikki/` - kaikki.org JSONL imports. Not a frequency
  source, but the source of truth for definitions; every row in the
  output TSVs gets its definition from here via the existing dict
  lookup chain.

### Needs to be added by the user

- **EPUBs**. The dictation said: _"I have a bunch of EPUBs I can also
  add to that data."_ Put them in:

  ```
  localdata/corpus/epub/fi/   # Finnish EPUBs
  localdata/corpus/epub/et/   # Estonian EPUBs (when we have them)
  ```

  This directory is already covered by the `localdata/` gitignore.
  The build script extracts XHTML content via the same code path the
  consumer alpha will use for `POST /api/import/decks` (see
  [`TODO.md`](../TODO.md) "EPUB and file upload support") so we share
  one extractor.

## Method

Single Go binary, e.g. `cmd/buildtop1000`, with these stages:

1. **Collect**. For each `(lang, register)` pair, read every source
   file, extract raw text. For EPUBs, walk the manifest spine, parse
   each XHTML document, strip tags. For OpenSubtitles + UD, the data
   is already form/count tabular - preserve the upstream count column;
   we won't re-tokenize text we don't have.

2. **Tokenize through `parsecore`**. For raw-text sources (EPUBs,
   `fi-corpus`), use the project's own Rust tokenizer so the output's
   tokenization matches what the consumer alpha will produce when a
   user pastes the same text. Aligning tokenization is the single most
   important methodological choice - without it, the seed deck and
   the user's parses disagree on what counts as one form.

3. **Aggregate**. Per `(lang, register, form)` → integer count. Strip
   `PUNCT`, `SYM`, and forms that look like numerals. Keep proper
   nouns for now; we'll let the deck-level filter (per
   [`docs/srs-deck-spec.md`](../docs/srs-deck-spec.md)) handle them.

4. **Resolve**. For each top-N candidate, call the existing dict
   lookup chain (`internal/store.BatchLookupForms`) to resolve to
   `(lemma, pos)`, pick a definition, and pick a representative
   example sentence (preferring a UD-source sentence when available
   - UD is rights-cleared for non-commercial use; subtitle sentences
   are not).

5. **Rank and write**. For each `(lang, register)`, write
   `docs/baselines/top-1000-inflected-forms-{lang}-{register}.tsv`
   with columns:

   ```
   rank  form  lemma  pos  definition  count  example_sentence  example_source
   ```

   Plus one combined `top-1000-inflected-forms-{lang}-merged.tsv` that
   ranks across all registers using a normalized blend (we'll publish
   the blend formula; see §"Open methodological choices" below).

6. **Verify**. Re-compute the top-N coverage curves and diff them
   against
   [`docs/FREQUENCY_BASELINES.md`](../docs/FREQUENCY_BASELINES.md). If
   anything moves by more than ±2 pp, flag and investigate before
   merging.

7. **Productize**. Ship the merged TSV behind a feature flag. The
   `Start with the top 1000` cold-start CTA on the dashboard creates a
   private deck for the user with `source_label='top-1000-baseline'`
   and skips the parse step. The new-card source for that deck is
   itself.

## Output artifacts

Committed to git:

- `docs/baselines/top-1000-inflected-forms-fi-subtitle.tsv`
- `docs/baselines/top-1000-inflected-forms-fi-written.tsv`
- `docs/baselines/top-1000-inflected-forms-fi-epub.tsv`
- `docs/baselines/top-1000-inflected-forms-fi-merged.tsv`
- `docs/baselines/top-1000-inflected-forms-et-subtitle.tsv`
- `docs/baselines/top-1000-inflected-forms-et-written.tsv`
- `docs/baselines/top-1000-inflected-forms-et-merged.tsv`
- `docs/baselines/top-1000-coverage-curves.md` - top-N coverage
  table, mirroring the format in
  [`docs/FREQUENCY_BASELINES.md`](../docs/FREQUENCY_BASELINES.md).

Not committed:

- The intermediate tokenized form-count files
  (`localdata/derived/topN/...`) - re-derivable, tied to a specific
  `parsecore` build, not stable.

## Open methodological choices

**Whether to weight subtitle vs. written equally in the merged list.**
The dictation said "70% or 80% or whatever the distribution is" of
running text - but as
[`docs/FREQUENCY_BASELINES.md`](../docs/FREQUENCY_BASELINES.md) shows,
top-1000 coverage is 65–73% for subtitle and 40–43% for written. They
are different distributions, not noisy samples of one. Recommendation:
**don't merge into one list as the default**. Ship per-register lists
and let the cold-start UI ask "What kinds of texts do you want to
read most? Conversation / News & books / Mixed" once, on first run,
to pick which register's list seeds the deck.

**Whether to seed the deck with forms or with lemmas.** Forms are the
right learning unit (it's what the learner sees), but a deck of 1000
forms with each its own card buries common lemmas across many
inflections. Recommendation: **lemma-keyed cards, form-frequency
ranked**. Each card is a `(lemma, pos)` keyed deck row; the card's
"forms seen so far" field is enriched with the inflected forms above
the cutoff, and the card is ranked by its **highest-ranked form** in
the list.

**How to handle proper nouns.** Subtitle and EPUB corpora will
surface character names. Show in the artifact for transparency, then
filter out at deck-creation time using `pos == 'PROPN'`.

**Tokenization mismatch with upstream form-counts.** OpenSubtitles
and UD already publish per-form counts tokenized by *their* tools.
Re-tokenizing through `parsecore` from the source text would be
methodologically cleaner but requires fetching the raw OPUS dump
(>1 GB) and re-running. For v1, accept their tokenization for those
two sources and document the caveat in
`top-1000-coverage-curves.md`. For EPUB and `fi-corpus`, we
tokenize ourselves so at least one source uses our own tokenization
end-to-end.

## Schedule (rough)

The dictation asked for high priority. If we treat this as a
two-week research bet:

- **Day 1–2**. EPUB extractor in `cmd/buildtop1000` (shared with
  upcoming `POST /api/import/decks` EPUB support). Walk one EPUB end
  to end, write the raw text to disk. Accept the user's drop into
  `localdata/corpus/epub/fi/`.

- **Day 3–4**. Aggregation pipeline. Consume EPUB raw text +
  `fi-corpus` + the four published TSVs. Write per-register
  intermediate counts.

- **Day 5–6**. Dictionary resolution pass. For each top-N candidate,
  resolve `(lemma, pos)` and pull a definition. Decide on the
  per-card example-sentence rule (UD-only for redistribution safety).

- **Day 7**. Coverage-curve regression check against
  [`docs/FREQUENCY_BASELINES.md`](../docs/FREQUENCY_BASELINES.md).

- **Day 8–9**. Ship the TSVs as the four `docs/baselines/*.tsv`
  files. Hook them into the cold-start CTA.

- **Day 10**. Manual review of FI top-50 by a Finnish-fluent reviewer
  for sanity. (The Estonian top-50 will need an analogous reviewer
  before launch - flagged on
  [`TODO.md`](../TODO.md) when the deck-detail page rolls out.)

## Concrete asks for the user

To unblock day 1:

1. Drop your existing FI EPUBs into
   `localdata/corpus/epub/fi/`. Filenames don't matter; we walk the
   directory.
2. Confirm whether you have any ET EPUBs at hand. If not, the ET
   merged list will lean more on UD + OpenSubtitles, which is fine
   for v1 - flag it to me and I'll widen the EPUB-source weight in
   the FI merged list rather than wait for ET parity.
3. Decide whether the cold-start CTA should be visible to users who
   already have ≥1 deck. Recommendation: yes, but moved from the
   primary slot in the dashboard to the bottom of the Decks list as
   "More to study: top-1000 baseline".

## Connection to the live UGC-frequency research goal

[`TODO.md` "Discover the most-frequent inflected forms in
user-pasted text"](../TODO.md) is the live, growing version of this
list. Once that pipeline is shipped:

- The static seed-deck list defined here keeps shipping with the
  product (as a baseline; doesn't change after publication).
- A second, live ranking starts updating per language as users
  paste. We expose it on the admin dashboard first; once it has
  enough signal (probably 1M+ aggregated tokens per language), we
  start showing it next to the static list on the deck-detail page
  as "compared to people pasting in finne.st right now".
- Diffs between the two are themselves a research output: which
  written-Finnish forms are over- or under-represented in
  user-pasted reality vs. UD-news.

## Risks / things I expect to bite us

- **License surface for redistribution**. UD-Estonian-EDT is
  CC-BY-NC-SA. We can ship the *derived* top-1000 form list (the
  rank itself, not the counts) under our own license but should
  attribute and avoid commercial redistribution before talking to
  upstream. Same lawyering as
  [`docs/FREQUENCY_BASELINES.md`](../docs/FREQUENCY_BASELINES.md)
  already records.

- **Tokenization drift**. If we change the tokenizer between v1 and
  v2 of this list, top-1000 forms will reshuffle. Pin the tokenizer
  version in the artifact filename
  (`top-1000-inflected-forms-fi-merged-2026-05-07a.tsv`) per the
  baseline-naming convention already in use.

- **Definition staleness**. The list's `definition` column reflects
  the dictionary at build time. If the dictionary changes, the seed
  deck's definitions become inconsistent with the live dictionary
  for new parses. Mitigation: store cards by `(lang, lemma, pos)`
  only; pull definitions live from the dict at card-render time.

- **Adult content in subtitle data**. OpenSubtitles 2018 includes
  adult films. The top-1000 mostly washes that out (the head of the
  distribution is function words), but the rule-out is worth
  spot-checking.
