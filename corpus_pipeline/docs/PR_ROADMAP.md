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

## Tracked Work

### 1. Meaning Sources Research

**Status:** DONE — merged as PR #173
(`claude/meaning-sources-research`). This remains here as the rationale
record for the user-friendly wordlist dependency.

**Why:** the dictionary DB (`lemmas.gloss`) gives partial gloss coverage, but
the user-friendly wordlist needs reliable meanings for every lemma. This work
must land first so that item 2 can wire meanings into the export instead of
shipping empty gloss columns.

**Scope:**

- Audit current DB gloss coverage for FI and ET: what percentage of corpus
  lemmas have a non-empty meaning? Record numbers in a timestamped report.
- Identify candidate external meaning sources (Wiktionary dumps, Kielitoimiston
  sanakirja, Ekilex public data, other open lexical databases). Record license
  constraints and whether data can be redistributed or must stay local-only.
- Build or extend an import path so meanings reach the dictionary DB in a form
  the wordlist exporter can join on.
- Deliver a coverage report showing before/after gloss rates.

**Result:** ET meaning coverage work landed and is summarized in the
timestamped corpus reports. Future meaning-source changes should add a new
dated report rather than rewriting those results.

### 2. Canonical Cleanup + User-Friendly Wordlist

**Status:** DONE — merged as PR #174 (`claude/user-friendly-wordlist`).

**Why:** `example_text` repeats sentence or poem text on millions of wordlist
rows, bloating canonical files. Meanwhile `wordlist.tsv` is parser evidence, not
a learner-facing export. These are one unit of work: normalize the canonical
data and ship the user-facing replacement together.

**Dependency:** item 1 supplied the meaning coverage needed for this export.

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

**Result:** canonical exports keep example references; learner-facing exports
carry meaning, parsed morphology, counts, provenance, parser/FST fingerprints,
and example refs.

### 3. Sentence Export + EPUB Extraction Cleanup

**Status:** DONE — merged as PR #170 (`codex/sentence-export-epub-cleanup`).

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

**Result:** `sentences_user_friendly.tsv` exists as a filtered sentence bank
while canonical `sentences.tsv` and `sentence_occurrences.tsv` remain
auditable.

### 4. Later: Interlinear Glossing Prototype

**Why:** a surface form plus lemma, meaning, and morphology can power an
interlinear glossing aid for learners. This needs the user-friendly wordlist
(item 2) and cleaner sentences (item 3) to be useful.

This is investigation/prototype work, not a normal shipping PR.

**Scope:**

- Prototype a small local tool that takes a sentence, looks up each surface in
  the wordlist, and emits lemma/gloss/FEATS rows.
- Evaluate whether the output is useful enough to ship as a user-facing feature.

**Trigger:** items 2 and 3 are complete, so this can be picked up when the app
needs morphology-aware learner explanations beyond lemma + meaning.
