# Source-Agnostic Correction Taxonomy

_Created 2026-05-15._

FinEstDB corrections must work for any source a learner brings into the
product: pasted text, EPUBs, articles, subtitle exports, Anki imports, and
future catalog decks. YLE-derived Anki fixes are useful because they are rich
examples, but the product model must not encode YLE or Anki as special cases.

This document defines the shared correction categories and the target overlay
model. The implementation should keep Finnish and Estonian correction content
separate while reusing the same schema, admin workflow, eval promotion, and
review-card renderer.

## Core Principle

Correct the smallest durable layer that is actually wrong.

Do not average parser, gloss, source-sentence, and card-presentation problems
into one manual note. A single learner-visible bad card may need more than one
correction, but each correction should land in the layer that can safely help
future parses and cards.

## Learning Targets

A review item should be keyed by a learning target, not only by a lemma.

Recommended target kinds:

| Kind | Use when | Example |
| --- | --- | --- |
| `lemma` | the dictionary entry is the learner target | FI `sanoa` |
| `surface` | a lexicalized form is the learner target | FI `kokonaan`, ET `välja` |
| `phrase` | the answer is a multi-word expression | FI `hädin tuskin`, ET adposition phrase patterns |
| `proper_name` | capitalization and named-entity identity are essential | FI `Maria`, `Norja` |

Every target should keep `lang`, target text, normalized key, POS or target
type, optional FEATS, and provenance. A target can link to many occurrences in
many saved sources.

## Correction Types

### Parser Identity

Use when the lemma, POS, or FEATS is wrong for the surface in context.

Examples:

- FI `sanoin` should be `sanoa/VERB`, not `sana/NOUN`.
- FI capitalized `Maria` should be a proper name, not `mari`.
- ET `peatus` may need the finite-verb reading `peatuma/VERB` in context.

Durable action:

- Write a high-priority lexical override when the correction is global.
- Add or update a language-specific analyzer-trap eval case.
- If the fix is contextual only, keep it out of global overlays and route it
  through future disambiguation or a contextual correction row.

### Meaning Cue

Use when the parser identity is right but the learner cue is misleading.

Examples:

- FI `kaupassa` as `shop / store`, not broad `trade / commerce`.
- FI `kokonaan` as `entirely / completely`, not compositional `as-size`.
- ET source-backed translations that are valid long-tail senses but bad
  learner primaries.

Durable action:

- Add a `surface` or `lemma` gloss override with source provenance.
- Keep sentence-specific senses in contextual overrides instead.

### Contextual Sense

Use when a surface has multiple normal meanings and the sentence selects one.

Examples:

- FI `käydä kaupassa` means go shopping / go to the shop.
- FI `päällä` varies between physical position, idiom, and postposition uses.
- ET adpositions such as `peale`, `eest`, and `taga` need the local phrase to
  choose the English cue.

Durable action:

- Add a `(lang, source_text_hash or sentence_id, target_surface)` contextual
  gloss row.
- The front cue and back target meaning must read from the same override.

### Phrase Boundary

Use when the analyzer selected one token but the learner should produce or
recognize the whole phrase.

Examples:

- FI `hädin tuskin`, not only `tuskin`.
- FI `jättää väliin`, `saada aikaan`, `olla pahoillani`.
- ET case/adposition constructions where teaching a single declined token
  hides the phrase pattern.

Durable action:

- Create or update an MWE/phrase target linked to token occurrences.
- Render cards from the phrase target while preserving token-level provenance.

### Example Quality

Use when the sentence itself makes the card ambiguous, misleading, or too noisy.

Examples:

- A source row joined unrelated neighboring dialogue.
- A cloze sentence swaps related-but-distinct lemma families.
- The surrounding sentence gives away the answer or tests two grammar points at
  once.

Durable action:

- Prefer a better occurrence from the same saved source when available.
- Otherwise add a reviewed sentence replacement scoped to the target and source.
- Do not rewrite source text globally unless the extraction itself is wrong.

### Card Presentation

Use when parser, meaning, and sentence are correct but the card still teaches
poorly.

Examples:

- Bad or answer-leaking image.
- Prompt wording teaches the wrong contrast.
- Back-side explanation restates the case name without explaining the local
  phrase.

Durable action:

- Store a reviewed card-field override or generated explanation override.
- Keep it scoped to the target, card type, source occurrence, and language.

## Language Boundaries

The correction infrastructure is shared. The correction content is not.

Finnish and Estonian should share:

- correction status workflow;
- target and overlay table shape;
- admin triage UI;
- eval promotion mechanics;
- review-card rendering contract.

Finnish and Estonian must keep separate:

- lexical overlays;
- bad-analysis blocklists;
- MWE seeds;
- morphology assumptions;
- frequency/register priors;
- gold fixtures.

A Finnish fix is evidence that the category is worth checking in Estonian. It
is not a portable rule.

## Review-Card Contract

Review payloads should be able to render these fields when available:

- target kind and target text;
- expected answer;
- source sentence with target span;
- target meaning;
- sentence translation;
- literal/local note;
- grammar explanation;
- morphology breakdown;
- source label and source type;
- correction/report affordance.

The current lemma-only review card can remain as a fallback, but new work
should move toward this occurrence-aware payload.

## Promotion Path

1. Learner submits feedback from Inspect, deck detail, or review.
2. Admin classifies it into exactly one primary correction type.
3. Acceptance writes the smallest durable overlay row.
4. Parser identity fixes add FI or ET eval cases when safe.
5. Meaning/card fixes add render tests so front and back remain consistent.
6. Weekly Track B reports group accepted corrections by language, source type,
   correction type, and parser mode.

## Source Metadata

Saved sources should carry enough metadata to evaluate correction quality:

- `source_type`: paste, EPUB, article, subtitle, Anki import, catalog deck.
- `register`: conversational, news, literary, mixed, unknown.
- extraction/version metadata when available.

This matters for comprehension prediction and for deciding whether a correction
is a global learner rule or a source/register-specific reading.
