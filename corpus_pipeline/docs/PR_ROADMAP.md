# Corpus Pipeline PR Roadmap

This document records the near-term PR sequence for the corpus pipeline. Every
PR in this workstream should explain **why** it exists, not just what changed.
The corpus data is large and the schemas are easy to misunderstand later, so the
rationale is part of the artifact.

## PR Body Rule

Every corpus-pipeline PR should include:

- **Why:** the user or operational problem the PR solves.
- **What changed:** code, schema, docs, or report updates.
- **Artifact impact:** which generated files change, and whether old files can
  still be read.
- **Reconstruction notes:** how to recover omitted denormalized data from the
  canonical TSVs.
- **Verification:** commands run, or an explicit note when the change is
  docs-only.

Reports and decisions should stay append-only. When a run produces new numbers,
add a new timestamped report under `corpus_pipeline/reports/` instead of
rewriting old reports.

## Planned PRs

### 1. Drop Denormalized Example Text

**Why:** `example_text` repeats sentence or poem text on millions of wordlist
rows. It makes the canonical files much larger and slower to inspect, while the
same text is already available through `example_ref_type` and `example_ref_id`.

**Plan:** remove `example_text` from canonical `wordlist.tsv` and mining TSVs.
Keep `example_ref_type` plus `example_ref_id`.

**Reconstruction:** downstream code should recover examples with:

- `example_ref_type=sentence`: join `wordlist.example_ref_id` to
  `sentences.tsv.id`.
- `example_ref_type=poem`: join `wordlist.example_ref_id` to `poems.tsv.id`.

This keeps canonical corpus evidence normalized while preserving a stable path
to examples.

### 2. Add User-Friendly Word List

**Why:** `wordlist.tsv` is a canonical parser/corpus evidence table, not the
best human-facing export. The learner-facing artifact needs the surface form,
meaning, lemma, part of speech, and grammatical features up front.

**Plan:** add `wordlist_user_friendly.tsv` as a derived export. Initial columns
should prioritize:

- `surface`
- `meaning`
- `lemma`
- `pos`
- parsed morphology such as case, number, mood, tense, person, voice, verbform
- full `feats`
- prose, poetry, and total counts
- analysis rank and parser-choice marker
- example reference fields

**Meaning source:** meanings do not come from the raw corpus files. The first
implementation should use the existing dictionary DB (`lemmas.gloss` and
translation tables where available). Separate research can add external
dictionary/parallel resources later.

### 3. Add User-Friendly Sentence List

**Why:** canonical `sentences.tsv` is evidence. It intentionally keeps every
deduped sentence-like unit the extractor/parser emitted. User-facing examples
need a cleaner list that filters title-only rows, name-only rows, front matter,
ISBN/page markers, and other extraction residue.

**Plan:** add `sentences_user_friendly.tsv` as a derived export. Keep
`sentences.tsv` and `sentence_occurrences.tsv` canonical and auditable.

### 4. Improve EPUB Extraction

**Why:** EPUBs contain navigation pages, title pages, copyright pages, tables of
contents, publisher metadata, and short heading rows. These pollute canonical
sentences and downstream example selection.

**Plan:** improve `extract_epub` to skip obvious nav/front-matter resources and
paragraphs before aggregation. This reduces junk at the source while the
user-friendly sentence export catches any remaining bad rows.

### 5. Research Better Meaning Sources

**Why:** the existing dictionary DB gives useful gloss coverage, but the raw
corpus itself is not a bilingual dictionary. Better learner-facing meanings may
require external lexical resources, parallel corpora, or hand-curated mappings.

**Plan:** identify candidate Finnish and Estonian meaning sources, record
license/local-only constraints, and add only sources whose output can be kept
cleanly separate from raw corpus evidence.

### 6. Translation-Assist / Interlinear Prototype

**Why:** a surface form plus lemma, meaning, and morphology can power a useful
learner explanation layer. The realistic first product is not a full machine
translation engine; it is an interlinear or glossing aid that explains why a
surface form means what it does in context.

**Plan:** after `wordlist_user_friendly.tsv` exists, prototype a small local
tool that takes a sentence, looks up each surface, and emits lemma/gloss/FEATS
rows. Later parser work can improve disambiguation and ordering.

**Trigger:** start this when the app needs richer inspect/review cards than a
plain lemma + definition view.
