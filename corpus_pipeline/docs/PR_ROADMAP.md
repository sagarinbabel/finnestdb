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

## Planned Work

### 1. Canonical Cleanup + User-Friendly Wordlist

**Why:** `example_text` repeats sentence or poem text on millions of wordlist
rows, bloating canonical files. Meanwhile `wordlist.tsv` is parser evidence, not
a learner-facing export. These are one unit of work: normalize the canonical
data and ship the user-facing replacement together.

**Scope:**

- Remove `example_text` from canonical `wordlist.tsv` and mining TSVs. Keep
  `example_ref_type` plus `example_ref_id`.
- Add `wordlist_user_friendly.tsv` as a derived export with: `surface`,
  `meaning`, `lemma`, `pos`, parsed morphology (case, number, mood, tense,
  person, voice, verbform), full `feats`, counts, analysis rank, parser-choice
  marker, and example reference fields.

**Reconstruction:** downstream code recovers examples by joining
`example_ref_id` to `sentences.tsv.id` (when `example_ref_type=sentence`) or
`poems.tsv.id` (when `example_ref_type=poem`).

**Meaning source:** meanings come from the existing dictionary DB
(`lemmas.gloss` and translation tables where available), not from raw corpus
files.

**Trigger:** next implementation PR after this roadmap lands.

### 2. Sentence Export + EPUB Extraction Cleanup

**Why:** canonical `sentences.tsv` keeps every deduped sentence-like unit the
extractor emitted — including navigation pages, title pages, ISBN markers, and
other EPUB residue. Better extraction and a filtered user-facing export are
coupled: improving `extract_epub` reduces junk at the source, and the
user-friendly sentence export catches whatever remains.

**Scope:**

- Improve `extract_epub` to skip obvious nav/front-matter resources and short
  heading rows before aggregation.
- Add `sentences_user_friendly.tsv` as a derived export that filters title-only,
  name-only, front-matter, and extraction residue rows.
- Keep canonical `sentences.tsv` and `sentence_occurrences.tsv` auditable.

**Trigger:** sentence-bank examples are consumed by the app or by manual quality
review.

### 3. Later: Meaning Sources + Interlinear Prototype

**Why:** the dictionary DB gives useful gloss coverage, but better learner-facing
meanings may require external lexical resources. A surface form plus lemma,
meaning, and morphology can power an interlinear glossing aid — but that needs
the user-friendly wordlist and cleaner sentences first.

This is investigation/prototype work, not a normal shipping PR.

**Scope:**

- Identify candidate Finnish and Estonian meaning sources; record license and
  local-only constraints.
- Prototype a small local tool that takes a sentence, looks up each surface, and
  emits lemma/gloss/FEATS rows.

**Trigger:** DB gloss coverage is visibly inadequate for
`wordlist_user_friendly.tsv`, or the app needs morphology-aware learner
explanations beyond lemma + meaning.
