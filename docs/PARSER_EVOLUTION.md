# Parser Evolution

The history layer of the parser workbench. The browser Inspect page
shows current parser behavior on arbitrary text; the eval CLI freezes
parser behavior on gold sets ([`docs/baselines/`](baselines/)); this
doc threads the frozen measurements together so the parser's
trajectory over time is legible at a glance.

**Forward convention**: every parser-affecting PR adds an entry here
when it lands. If a PR is parser-affecting and you can't articulate
the entry, the change isn't ready to merge.

For the per-measurement deep dives, see the dated summary docs in
[`baselines/`](baselines/). For the doc/strategy changelog (separate
from this), see [CHANGELOG.md](CHANGELOG.md).

## Trend at a glance

Custom parser, headline metrics. The estnltk row is a fixed ceiling
reference (Estonian external analyzer), not a measurement event —
it shows what "good" looks like and what `custom` should approach
once Voikko-equivalent enrichment lands for FI.

| Date | Commit | FI fi-manual-v1 lemma | ET et-grammar-v1 lemma | FI grammar | ET grammar | ET coverage |
|---|---|---:|---:|---:|---:|---:|
| 2026-05-06i (FST PR 4/4 ships, full suite re-measured) | [`91fecbf`][c-2026-05-06i] | **82.9** | 88.6 | **1.4** | **2.0** | 100.0 |
| 2026-05-06h (FST PR 3/4: Giellalt ET) | [`5944733`][c-2026-05-06h] | — | 88.6 | — | **2.0** | **100.0** |
| 2026-05-06g (FST PR 2/4: Giellalt FI HFST) | [`7d5cafd`][c-2026-05-06g] | — | — | 1.4 | — | — |
| 2026-05-06f (FST PR 1/4: Voikko VFST) | [`d9937c0`][c-2026-05-06f] | — | — | **1.4** | — | — |
| 2026-05-06d (Phase 3 ships) | (PR 3.2 head) | 81.4 | 88.6 | 0.0 | 0.0 | 100.0 |
| 2026-05-06c (Phase 2 ships) | [`615556e`][c-2026-05-06c] | 81.4 | 88.6 | 0.0 | 0.0 | 100.0 |
| 2026-05-06b (post-priority-fix) | [`b327d4f`][c-2026-05-06b] | **81.4** | **88.6** | 0.0 | 0.0 | **100.0** |
| 2026-05-06 | [`46d8b77`][c-2026-05-06] | 81.4 | 80.0 | 0.0 | 0.0 | 100.0 |
| 2026-05-05 | [`af111c2`][c-2026-05-05] | — | 88.6 | — | 2.0 | 94.6 |
| 2026-05-05 (estnltk ceiling) | [`af111c2`][c-2026-05-05] | — | **98.1** | — | **92.2** | 100.0 |
| 2026-04-28 | [`bb744ba`][c-2026-04-28] | 72.9 | 87.6 | 0.0 | 2.0 | 94.6 |

[c-2026-05-06i]: https://github.com/sagarinbabel/finnestdb/commit/91fecbf
[c-2026-05-06h]: https://github.com/sagarinbabel/finnestdb/commit/5944733
[c-2026-05-06g]: https://github.com/sagarinbabel/finnestdb/commit/7d5cafd
[c-2026-05-06f]: https://github.com/sagarinbabel/finnestdb/commit/d9937c0
[c-2026-05-06c]: https://github.com/sagarinbabel/finnestdb/commit/615556e
[c-2026-05-06b]: https://github.com/sagarinbabel/finnestdb/commit/b327d4f
[c-2026-05-06]: https://github.com/sagarinbabel/finnestdb/commit/46d8b77
[c-2026-05-05]: https://github.com/sagarinbabel/finnestdb/commit/af111c2
[c-2026-04-28]: https://github.com/sagarinbabel/finnestdb/commit/bb744ba

## Entries

### 2026-05-06i — FST PR 4/4 ships: change document + final eval freeze

**Commit**: [`91fecbf`][c-2026-05-06i] (PR [#112](https://github.com/sagarinbabel/finnestdb/pull/112))
**Detail**: [`baselines/2026-05-06-final-fi.md`](baselines/2026-05-06-final-fi.md), [`baselines/2026-05-06-final-et.md`](baselines/2026-05-06-final-et.md)

PR 4 of the four-PR FST stack ([#107](https://github.com/sagarinbabel/finnestdb/pull/107) → [#108](https://github.com/sagarinbabel/finnestdb/pull/108) → [#110](https://github.com/sagarinbabel/finnestdb/pull/110) → [#112](https://github.com/sagarinbabel/finnestdb/pull/112)). Documentation-only: ships [`docs/FST_LEMMATIZER.md`](FST_LEMMATIZER.md) (the per-package architecture doc), [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) (no upstream `.vfst`/`.hfstol` blobs in git; derived factual tables only), and the bilingual lexical-architecture section of [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md). Re-runs the full eval suite to confirm post-PR3 numbers held after merge cleanup; first run that measures **fi-manual-v1 (22 cases)** under the full FST stack.

**Headline numbers** (custom parser):

| Dataset (cases) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| fi-grammar (80) | 97.4 | 98.7 | 1.4 | 51.9 | 100.0 |
| fi-core (6) | 85.0 | 90.0 | 0.0 | 35.0 | 100.0 |
| fi-manual-v1 (22) | **82.9** | 87.1 | **13.3** | 62.9 | 96.9 |
| fi-manual-v2 (4) | 88.9 | 100.0 | 0.0 | 55.6 | 100.0 |
| et-grammar (50) | 88.6 | 96.2 | 2.0 | 42.9 | 100.0 |
| et-manual (4) | 88.9 | 77.8 | 16.7 | 22.2 | 100.0 |

**Changes since 2026-05-06h**: docs only. No code paths affected.

**Net effect** (vs. 2026-05-06h):

- Per-dataset numbers held identical to PR3 head on every dataset PR3 measured. Cleanup commits between PR3 head and PR4 merge did not move metrics — verified file-by-file against `2026-05-06-post-pr3-*.json` and `2026-05-06-post-pr2-hfstol-fi-*.json`.
- **First FST-stack measurement of `fi-manual-v1` (22 cases): lemma 81.4 → 82.9 (+1.4pp), grammar 0.0 → 13.3 (+13.3pp), coverage 91.2 → 96.9 (+5.7pp).** This is the FST stack's largest accuracy lift on a non-trivial Finnish set — the v2 (4-case) set used at PR1/PR2 didn't have headroom.

**Open issues this surfaced**:

- Grammar accuracy still low across all sets (1.4–16.7%). The FST analysers produce UD FEATS but `internal/store/dict.go` doesn't yet consume Number/Tense/Mood/Person from FST output beyond the Voikko grammar-label heuristic. Tracked as the FEATS-migration follow-up (was [#118](https://github.com/sagarinbabel/finnestdb/pull/118), to be cherry-picked onto the cleaned base).
- Tracked tables `pkg/lemmatizer-fi-et/tables/{fi_min.json,fi_wordlist.txt,et_min.json}` are derived data and should move under `localdata/` per the new artifact policy. Pending the artifact-policy cleanup PR.

---

### 2026-05-06h — FST PR 3/4 ships: Giellalt ET HFST analyser

**Commit**: [`5944733`][c-2026-05-06h] (PR [#110](https://github.com/sagarinbabel/finnestdb/pull/110))
**Detail**: [`baselines/2026-05-06-post-pr3-et.md`](baselines/2026-05-06-post-pr3-et.md)

Plugs Giellalt's `lang-est` HFST analyser into Step 5 for ET via the `pkg/lemmatizer-fi-et/hfstol/` runtime that PR2 introduced for FI. Extends `pkg/lemmatizer-fi-et/giellaltmap/` with ET-specific Giellalt tag mappings. `internal/store/dict.go` Step 5 dispatches to ET FST analyses when `lang=ET`.

**Headline numbers** (custom parser, ET only — PR3 didn't re-measure FI):

| Dataset (cases) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| et-grammar (50) | 88.6 | 96.2 | 2.0 | 42.9 | **100.0** |
| et-manual (4) | **88.9** | 77.8 | **16.7** | 22.2 | **100.0** |

**Changes since 2026-05-06g**:

- [#110](https://github.com/sagarinbabel/finnestdb/pull/110) — `giellaltmap` ET tag table; `pkg/lemmatizer-fi-et/lemmatizer.go` extended to dispatch `Lemmatize(ET, …)`; Step 5 wired for ET in `internal/store/dict.go`; generated factual table at `pkg/lemmatizer-fi-et/tables/et_min.json`.

**Net effect** (vs. 2026-05-06g, ET-only):

- **et-manual: lemma 77.8 → 88.9 (+11.1pp), grammar 0.0 → 16.7 (+16.7pp), coverage 91.7 → 100.0 (+8.3pp)** — single largest stage-attributable lift in the FST stack. Closes the long-standing ET gold-set coverage gap that 2026-04-28's first frozen baseline flagged.
- et-grammar: +1.1pp coverage; lemma/grammar/POS held. The remaining headroom (vs. estnltk's 98.1 lemma / 92.2 grammar at the 2026-05-05 ceiling) is on contextual-disambiguation cases the analyser doesn't disambiguate either.
- FI numbers carried unchanged from PR2 (PR3 was ET-only by design).

**Open issues this surfaced**:

- et-grammar grammar accuracy stuck at 2.0%. Same FEATS-consumer gap as FI; unblocks via the FEATS-migration follow-up.
- `pickBestVFSTAnalysis` in `internal/store/dict.go` is FI-named but is also chosen for ET tie-breaking. Cosmetic — not a behavior bug — but worth renaming when the FEATS migration touches the same code.

---

### 2026-05-06g — FST PR 2/4 ships: Giellalt FI HFST analyser

**Commit**: [`7d5cafd`][c-2026-05-06g] (PR [#108](https://github.com/sagarinbabel/finnestdb/pull/108))
**Detail**: [`baselines/2026-05-06-post-pr2-hfstol-fi.md`](baselines/2026-05-06-post-pr2-hfstol-fi.md)

Adds a pure-Go HFST optimised-lookup runtime (`pkg/lemmatizer-fi-et/hfstol/`) and plugs Giellalt's `lang-fin` FI analyser into Step 5 alongside Voikko's VFST. The two analysers run in priority order: Voikko VFST first (its case-disambiguation heuristics are stronger on common Finnish), Giellalt HFST as a fallback that fills the lookup gaps VFST misses.

**Headline numbers** (custom parser, FI only — PR2 didn't re-measure ET):

| Dataset (cases) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| fi-grammar (80) | 97.4 | 98.7 | 1.4 | 51.9 | 100.0 |
| fi-core (6) | 85.0 | 90.0 | 0.0 | 35.0 | **100.0** |
| fi-manual (v2, 4) | 88.9 | 100.0 | 0.0 | 55.6 | 100.0 |

**Changes since 2026-05-06f**:

- [#108](https://github.com/sagarinbabel/finnestdb/pull/108) — `pkg/lemmatizer-fi-et/hfstol/{transducer,analyze}.go` (HFST optimised-lookup format reader, pure Go); `pkg/lemmatizer-fi-et/giellaltmap/` (Giellalt tag → `Analysis`); `lemmatizer.go` extended to dispatch to both VFST and HFST tables for FI.

**Net effect** (vs. 2026-05-06f):

- **fi-core coverage: 95.7 → 100.0 (+4.3pp).** Pure recall lift — Giellalt HFST produces a lemma for surface forms VFST didn't cover. No accuracy movement on grammar/manual sets, which were already coverage-saturated.
- No FI lemma/POS change vs PR1 — when both VFST and HFST resolve a form, VFST wins per the analyser priority, so existing rankings are preserved.
- ET unchanged (PR2 is FI-only).

**Open issues this surfaced**:

- fi-grammar grammar accuracy plateaus at 1.4%. The HFST analyser produces UD-style FEATS but `internal/store/dict.go` only consumes lemma + POS today. Same FEATS-consumer gap that bites ET at PR3.
- fi-manual-v1 (the 22-case real-world Finnish set) wasn't re-measured at this stage; first measurement under the FST stack happens at PR4 (see 2026-05-06i).

---

### 2026-05-06f — FST PR 1/4 ships: Voikko VFST runtime (FI)

**Commit**: [`d9937c0`][c-2026-05-06f] (PR [#107](https://github.com/sagarinbabel/finnestdb/pull/107))
**Detail**: [`baselines/2026-05-06-post-pr1-vfst-fi.md`](baselines/2026-05-06-post-pr1-vfst-fi.md); pre-FST floor at [`baselines/2026-05-06-pre-fst-comparison-fi.md`](baselines/2026-05-06-pre-fst-comparison-fi.md), [`baselines/2026-05-06-pre-fst-comparison-et.md`](baselines/2026-05-06-pre-fst-comparison-et.md)

First step of the FST stack. Adds `pkg/lemmatizer-fi-et/vfst/` — a pure-Go port of libvoikko's `UnweightedTransducer` reading Voikko's `.vfst` binary directly — and `pkg/lemmatizer-fi-et/voikkomap/` to translate Voikko's FSTOUTPUT (`[Ln][Ica][Xp]talo[X]talo[Sn][Ny]`) into a structured `Analysis{Lemma, UPOS, GrammarLabel, Number, Tense, Mood, Person}`. Wires Step 5 into `internal/store/dict.go::BatchLookupForms` so unresolved tokens (or ambiguous multi-lemma resolutions) get a VFST analysis after the dict steps fail to nail a single answer.

**Pre-FST floor** (custom parser, the immediate predecessor — frozen on main as `2026-05-06-pre-fst-*`):

| Dataset (cases) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| fi-grammar (80) | 96.8 | 98.1 | 0.0 | 51.3 | 99.7 |
| fi-core (6) | 85.0 | 90.0 | 0.0 | 35.0 | 95.7 |
| fi-manual-v1 (22) | 81.4 | 85.7 | 0.0 | 62.9 | 91.2 |
| fi-manual (v2, 4) | 88.9 | 100.0 | 0.0 | 55.6 | 100.0 |
| et-grammar (50) | 88.6 | 96.2 | 2.0 | 42.9 | 98.9 |
| et-manual (4) | 77.8 | 77.8 | 0.0 | 11.1 | 91.7 |

**Headline numbers post-PR1** (custom parser, FI only — PR1 didn't measure ET):

| Dataset (cases) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| fi-grammar (80) | **97.4** | **98.7** | **1.4** | 51.9 | **100.0** |
| fi-core (6) | 85.0 | 90.0 | 0.0 | 35.0 | 95.7 |
| fi-manual (v2, 4) | 88.9 | 100.0 | 0.0 | 55.6 | 100.0 |

**Changes since 2026-05-06d** (= the pre-FST baseline parser; 2026-05-06b/c/d differ only in dictionary state, not parser code on this path):

- [#107](https://github.com/sagarinbabel/finnestdb/pull/107) — `pkg/lemmatizer-fi-et/{vfst,voikkomap,lemmatizer}` packages; `internal/store/dict.go` Step 5 added for FI; generated factual table `pkg/lemmatizer-fi-et/tables/fi_min.json` (derived offline from upstream Voikko `mor.vfst` via `cmd/genlemmatizertables`); [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) introduced (forbids upstream transducer blobs in git).

**Net effect** (vs. pre-FST baseline above):

- **fi-grammar: lemma 96.8 → 97.4 (+0.6pp), POS 98.1 → 98.7 (+0.6pp), grammar 0.0 → 1.4 (+1.4pp), coverage 99.7 → 100.0 (+0.3pp).** First non-zero grammar reading any FI dataset has produced — VFST emits Number/Tense/Mood/Person on surface analyses where the dict couldn't.
- fi-core unchanged at 95.7% coverage (the gap is on words VFST also misses; closes in PR2).
- fi-manual unchanged (the 4-case smoke set was already at 100% coverage and 88.9% lemma; no headroom for VFST to move it).
- ET not re-measured at this step (PR1 is FI-only).

**Open issues this surfaced**:

- VFST analyses sometimes return multiple readings; `pickBestVFSTAnalysis` picks one heuristically (lowercase-surface prefers lowercase lemma, demote PROPN on lowercase, FST priority order on tie). Edge cases pinned in `internal/store/dict_test.go::TestPickBestVFSTAnalysis_*`.
- VFST's Voikko-specific tag schema differs from Giellalt's HFST schema — addressed in PR2 by the parallel `voikkomap` / `giellaltmap` design.
- VFST covers ~95% of contemporary Finnish surface forms; the long tail (compounds, proper nouns, technical vocabulary) is what PR2's Giellalt HFST is designed to fill.

---

### 2026-05-06d — Phase 3 ships Kotus paradigm classes for FI

**Commit**: TBD (PR 3.2 head, follows [#92](https://github.com/sagarinbabel/finnestdb/pull/92) which landed the binary skeleton)

Phase 3 of [`docs/FINNISH_LEXICAL_PLAN.md`](FINNISH_LEXICAL_PLAN.md) is shipped. The Kotus Nykysuomen sanalista 2024 (CC BY 4.0, ~104k FI headwords) is now a tracked artifact under [`data/kotus/`](baselines/) and `cmd/importkotus` populates `paradigm_class` on the FI lemmas table. That's the join key the Phase 4 Voikko adapter needs.

PR 3.1 ([#92](https://github.com/sagarinbabel/finnestdb/pull/92)) landed the binary skeleton against an assumed XML schema — documented in the code as "refine in 3.2 against real data." When 3.2 fetched the real distribution, it turned out to be a **TSV, not XML**: header-prefixed `Hakusana\tHomonymia\tSanaluokka\tTaivutustiedot`. PR 3.2 replaced the parser entirely.

**Headline numbers** (custom parser): identical to 2026-05-06c. Phase 3 doesn't move accuracy or coverage — only metadata enrichment, no new form rows yet, no new lemmas resolvable through `BatchLookupForms`. Voikko (Phase 4) is what makes the metadata pay off by expanding each lemma's Kotus class into its surface paradigm.

**Real-data smoke (against `/tmp/finnestdb-baseline.db`, post-bugfix)**:

```
Done. processed=104743  paradigms_upgraded=46324  new_lemmas_inserted=4289
      no_known_class=436  no_paradigm=56485
```

- 104,743 TSV rows processed
- 46,324 paradigm-class upgrades on existing kaikki rows (scoped by current row's Sanaluokka)
- 4,289 new Kotus-only lemmas inserted at `source='kotus'`, priority 10 — includes homonyms like `kuurata`/NOUN + `kuurata`/VERB inserted from separate TSV rows
- 436 entries with no recognised Sanaluokka label (counted, not inserted)
- 56,485 entries (54%) have empty `Taivutustiedot` — typically compound headwords; rows inserted with `paradigm_class IS NULL`
- **Net: 48,740 FI lemmas now carry a populated `paradigm_class`** (~19% of total FI rows)

**Changes since 2026-05-06c**:

- [#92](https://github.com/sagarinbabel/finnestdb/pull/92) — `cmd/importkotus` binary, parser, and per-class fixture tests. Original assumed-schema XML parser.
- (this PR / 3.2) — TSV parser replacing the XML skeleton, real Kotus 2024 data committed as a tracked artifact under [`data/kotus/`](baselines/), `make import-kotus-fi` target, [`data/kotus/NOTICE.md`](baselines/) for CC BY 4.0 attribution. `make import-dict-fi-recommended` now chains kaikki + Kotus.

**Net effect**:

- **No accuracy/coverage movement** — as designed.
- **Voikko (Phase 4) is now unblocked.** The next phase is the Voikko generator spike (Phase 3.5), which proves the actual paradigm-generation entry point against the Kotus class numbers we just populated.
- **Test coverage held at 18 of 18** for `cmd/importkotus`. Tests rewritten for the TSV format; the XML-shape tests from 3.1 are gone since the file format isn't XML.

**Open issues this surfaced**:

- 320 entries (~0.3%) have Sanaluokka labels we don't recognise. Most are compound class strings like `"adverbi, postpositio, prepositio"` (one entry, multiple classes) or rarer terms. The `mapWordClass` table can be extended in a small follow-up. Won't break Phase 4.
- 56,485 entries with empty Taivutustiedot (Kotus-uncategorised compounds) get rows inserted with `paradigm_class IS NULL`. These are existing FI words that get headword presence but no Voikko-class join. If the Voikko adapter (Phase 4) is happy enough to skip them, fine; if it needs paradigm_class for every row, those compounds need their base lemma's class propagated — a question for Phase 3.5's spike.
- POS heuristic in 3.1 (number-range) has been replaced by direct Sanaluokka mapping. The 3.1 inflectionType-range fallback is gone — there's no situation where it's needed since real Kotus rows always have explicit `Sanaluokka`.

---

### 2026-05-06c — Phase 2 ships translations table end-to-end

**Commit**: [`615556e`][c-2026-05-06c] (PRs [#85](https://github.com/sagarinbabel/finnestdb/pull/85), [#86](https://github.com/sagarinbabel/finnestdb/pull/86), [#89](https://github.com/sagarinbabel/finnestdb/pull/89))

Phase 2 of [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md) is shipped across both languages. Each kaikki and Ekilex sense's English translation is now its own row in the `translations` table; the read path consults `translations` first, falling back to `lemmas.gloss` only when no row matches.

**Headline numbers** (custom parser): identical to 2026-05-06b. Phase 2 doesn't move accuracy or coverage — eval doesn't measure gloss text correctness. The change is structural, not behavioral.

**Changes since 2026-05-06b**:

- [#85](https://github.com/sagarinbabel/finnestdb/pull/85) — `cmd/importdict` writes per-sense translation rows from kaikki dumps. Three review iterations hardened the upsert + cleanup pattern: refresh-on-conflict, source-scoped wipe to remove orphaned senses, DELETE inside the transaction so a stream failure rolls back to the pre-import state.
- [#86](https://github.com/sagarinbabel/finnestdb/pull/86) — `internal/store/dict.go` `BatchLookupGlosses` queries `translations` JOINed to `lemmas` on `(lemma, pos, lang, source)`. The JOIN is the load-bearing design choice: it lets the query rank by `lemmas.source_priority` without a denormalized priority column, AND makes `-custom-glosses` overrides "just work" (the JOIN finds no `source='custom'` translation, falls through to `lemmas.gloss`).
- [#89](https://github.com/sagarinbabel/finnestdb/pull/89) — `cmd/importekilexdetails` extends #85's pattern to ET. All of #85's review-iteration lessons applied upfront so #89 went through review without a repeat round.

**Net effect**:

- **No accuracy/coverage movement**. As designed — Phase 2 was structural. Verified on `/tmp/finnestdb-baseline.db` (the post-#84 reference); both FI and ET return identical numbers via the lemmas.gloss fallback path that's still hit when translations rows aren't populated.
- **Translation rows now driving glosses for kaikki entries** when a fresh DB is built under the new code. For pre-existing DBs imported before #85 (the typical user state), the fallback path runs and behavior matches pre-Phase-2 exactly.
- **Multi-source translations supported** end-to-end. When kaikki and Ekilex both have a translation for the same `(lemma, pos)`, the read path picks the one whose `lemmas.source_priority` is higher — i.e., whichever source won the lemma upsert.
- **`-custom-glosses` contract preserved**. The CSV-driven override path writes only to `lemmas.gloss`. The new read path's JOIN finds no matching translation row, falls back to `lemmas.gloss`, custom override surfaces. Unchanged user-visible behavior.

**Open issues this surfaced**:

- `applyCustomGlosses` doesn't write to `translations`. Custom overrides still flow through `lemmas.gloss` only. Works correctly via fallback, but if/when we want custom overrides to participate in multi-translation surfaces, that path needs to mirror the kaikki write pattern.
- fi.wiktionary's Finnish-language definitions (`target_lang='FI'`) are still unused. The `definitions` table exists; no importer writes to it. Filed for a future PR after Phase 3/4 stabilize.
- Eval doesn't measure gloss correctness. Phase 2's main effect — richer multi-translation data available to clients — is invisible to the parser-eval harness. A future evolution entry for "Inspect UI shows multiple translations" would surface it qualitatively if/when that ships.

---

### 2026-05-06b — Source-aware lookup recovery

**Commit**: [`b327d4f`][c-2026-05-06b]
**Detail**: [`baselines/2026-05-06b-summary.md`](baselines/2026-05-06b-summary.md)

Same dictionary state as 2026-05-06; the only thing that changed is
parser code. This entry captures the fix for the regression that the
previous entry surfaced.

**Headline numbers** (custom parser):

| Dataset (cases) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| et-grammar-v1 (50) | **88.6** | **94.3** | 0.0 | 42.9 | 100.0 |
| et-manual-v1 (4) | 88.9 | 88.9 | 0.0 | 22.2 | 100.0 |
| fi-manual-v1 (22) | 81.4 | 85.7 | 0.0 | 62.9 | 91.2 |
| fi-grammar-v1 (80) | 96.8 | 98.1 | 0.0 | 51.3 | 99.7 |
| fi-core-v1 (6) | 85.0 | 90.0 | 0.0 | 35.0 | 95.7 |
| fi-manual-v2 (4) | 88.9 | 100.0 | 0.0 | 55.6 | 100.0 |

**Changes since 2026-05-06**:

- [`internal/store/dict.go`](../internal/store/dict.go) `BatchLookupForms` step 1 now ranks multi-lemma candidates instead of arbitrarily picking the first row. Ranking, higher-to-lower priority:
  1. Case-match: lowercase surface prefers lowercase lemma (place names lose to common nouns when surface is lowercase).
  2. POS sanity: lowercase surface demotes PROPN.
  3. Source priority (higher wins).
  4. Deterministic tiebreak: source asc, lemma asc, POS asc.
- [`cmd/importekilexdetails`](../cmd/importekilexdetails/main.go): inserts now tag rows with `source='ekilex'`, `source_priority=20`. `ensureSchema` now runs `EnsureMultiLemmaSchema` and `EnsureDictionarySourceColumns` so a fresh DB matches the server's shape.

**Net effect**:

- **ET regression closes.** et-grammar-v1 lemma recovered 80.0 → **88.6** (matches April baseline). POS recovered 83.8 → **94.3** (April was 97.1; the residual 2.8pp gap is on genuinely-ambiguous cases like `naeris` NOUN-vs-VERB, `keelt` different-lemma-form — not fixable by case/POS heuristics alone).
- **No FI regression.** All four FI datasets unchanged. The ranker fires equally on FI but FI doesn't currently have many multi-lemma rows. When Voikko (Phase 4) introduces them, the same heuristic prevents the same accuracy collapse ET would have seen.
- **Coverage stays at 100%.** The fix didn't trade coverage for accuracy.

**Open issues this surfaced**:

- Residual 2.8pp POS gap on et-grammar-v1 vs. April. The remaining cases need contextual disambiguation (POS tagging from a trigram model or a real morphological analyzer like Vabamorf). Out of scope for PR 0.5 — would be a follow-up if it becomes the largest blocker.
- Same heuristic should apply at deck-ingest time too (`BatchLookupAllForms` returns unranked candidates; deck creates one card per candidate — that's intentional, but the *order* of cards may matter for which becomes "primary"). Worth reviewing during the kaikki refactor (Phase 2).
- This is also the first measurement that demonstrates the convention working: PR #83 surfaced an issue → PR 0.5 fixed it → an entry here proves the recovery. Going forward, every parser-affecting PR should produce an entry of this shape.

---

### 2026-05-06 — Post-Ekilex baseline

**Commit**: [`46d8b77`][c-2026-05-06] (PR [#83](https://github.com/sagarinbabel/finnestdb/pull/83))
**Detail**: [`baselines/2026-05-06-fi-summary.md`](baselines/2026-05-06-fi-summary.md), [`baselines/2026-05-06-et-summary.md`](baselines/2026-05-06-et-summary.md)

**Dictionary state**:

| Lang | Forms | Lemmas | Sources |
|---|---:|---:|---|
| FI | 26,826,071 | 259,145 | kaikki.org Finnish |
| ET | 6,178,514 | 354,231 | kaikki.org + Ekilex bulk drop ([#78](https://github.com/sagarinbabel/finnestdb/pull/78)) |

**Headline numbers** (custom parser):

| Dataset (cases) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| fi-manual-v1 (22) | **81.4** | 85.7 | 0.0 | 62.9 | 91.2 |
| fi-grammar-v1 (80) | 96.8 | 98.1 | 0.0 | 51.3 | 99.7 |
| fi-core-v1 (6) | 85.0 | 90.0 | 0.0 | 35.0 | 95.7 |
| fi-manual-v2 (4) | 88.9 | 100.0 | 0.0 | 55.6 | 100.0 |
| et-grammar-v1 (50) | 80.0 | 83.8 | 0.0 | 42.9 | **100.0** |
| et-manual-v1 (4) | 88.9 | 88.9 | 0.0 | 22.2 | 100.0 |

**Changes since 2026-05-05** (or 2026-04-28 for FI):

- [#65](https://github.com/sagarinbabel/finnestdb/pull/65) — Estonian lemma fallback rules; trailing `*` treated as punctuation. Affects both languages' edge cases.
- [#67](https://github.com/sagarinbabel/finnestdb/pull/67) — Multi-source schema (`source`, `source_priority` columns on lemmas/forms). Schema-only; no runtime ranking yet.
- [#76](https://github.com/sagarinbabel/finnestdb/pull/76) — FI lexical Phase 1: `paradigm_class`, `feats`, `translations`, `definitions` schema. Empty until Phases 2–5.
- [#78](https://github.com/sagarinbabel/finnestdb/pull/78) — Multi-lemma `forms` PK + `cmd/importekilexdetails`. ET form count grew 27× (228k → 6.18M).

**Net effect**:

- **FI improved on real-world text.** `fi-manual-v1` custom jumped +8.5 lemma (72.9 → 81.4) and basic +11.4 (44.3 → 55.7). Most plausibly attributable to [#65](https://github.com/sagarinbabel/finnestdb/pull/65)'s tokenizer/parser-rule fixes carrying over to FI.
- **ET regressed on lemma/POS by 8–13pp on et-grammar-v1.** Coverage rose to 100% (Ekilex covers everything in the gold set), but `BatchLookupForms` is a single-row `QueryRow` that arbitrarily picks among multi-lemma candidates. Ekilex's proper-noun homonyms now win over common-noun gold answers (e.g. `linnas` → `Linna`/PROPN instead of `linn`/NOUN). 20 of 178 tokens regressed; 8 went the other way.
- **Grammar accuracy stays at 0%** on every FI parser and 0% on ET basic/custom. No source populates `feats` yet. Phase 4 (Voikko) is the lever for FI.

**Open issues this surfaced**:

- `cmd/importekilexdetails` does not set row-level `source` / `source_priority` (only `dict_metadata`). All ET rows have `source=''`, `source_priority=0`.
- `BatchLookupForms` does not rank by source priority or POS preference. Same regression will hit FI in Phase 4 (Voikko) unless fixed first.
- Recommended next PR: source-aware lookup + importer source tagging. Re-run this baseline; expect ET regression to close.

---

### 2026-05-05 — estnltk ceiling on Estonian

**Commit**: [`af111c2`][c-2026-05-05]
**Detail**: [`baselines/2026-05-05-et-grammar-estnltk.json`](baselines/2026-05-05-et-grammar-estnltk.json), [`baselines/2026-05-05-et-manual-estnltk.json`](baselines/2026-05-05-et-manual-estnltk.json)

First measurement with the `estnltk` external adapter (EstNLTK / Vabamorf) as a parser. **Sets the ceiling** that `custom` should approach for Estonian once full lexical enrichment lands, and is the analogue of what `custom` should reach for Finnish once Voikko (Phase 4) lands.

**Dictionary state**: kaikki.org only (FI ~12.2M forms / 145k lemmas, ET ~228k / 12.6k). No Ekilex yet.

**Headline numbers**:

| Dataset (cases) | Parser | Lemma | POS | Grammar | Full | Coverage |
|---|---|---:|---:|---:|---:|---:|
| et-grammar-v1 (50) | basic   | 88.6 | 97.1 | 0.0 | 43.8 | 92.9 |
| et-grammar-v1      | custom  | 88.6 | 97.1 | 2.0 | 43.8 | 94.6 |
| et-grammar-v1      | **estnltk** | **98.1** | **97.1** | **92.2** | **92.4** | **100.0** |
| et-manual-v1 (4)   | basic   | 77.8 | 88.9 | 0.0 | 22.2 | 75.0 |
| et-manual-v1       | custom  | 77.8 | 88.9 | 0.0 | 22.2 | 83.3 |
| et-manual-v1       | **estnltk** | **100.0** | **100.0** | **100.0** | **100.0** | **100.0** |

**Changes since 2026-04-28**:

- `et-grammar-v1` basic stayed flat at 88.6 lemma (vs. April), but `custom` recovered from 87.6 to 88.6 — the et-0032 `Rongisõit` compound-splitter bug flagged in the April summary appears to have been resolved by some PR between April 28 and May 5 (likely a side-effect of [#65](https://github.com/sagarinbabel/finnestdb/pull/65)'s ET fallback work or related tokenizer changes).
- The eval *infrastructure* changed (estnltk adapter wired up, [#66](https://github.com/sagarinbabel/finnestdb/pull/66)) more than the parser itself.

**Net effect**:

- **estnltk handily beats `custom` on Estonian on every metric.** +9.5pp lemma, +90.2pp grammar on et-grammar-v1; +22.2pp lemma, +100pp grammar on et-manual-v1.
- The 92.2% grammar accuracy on et-grammar-v1 is the closest thing we have to a "real morphological analyzer" measurement. It's what a properly-featured `custom` parser could approach if it had access to UD-style features per form.

**Open issues this surfaced**:

- `custom` has no path to grammar accuracy without populated `feats`. The pieces are present in the schema (since [#76](https://github.com/sagarinbabel/finnestdb/pull/76)) but no source writes them yet.
- This baseline was a one-off run during [#65](https://github.com/sagarinbabel/finnestdb/pull/65) iteration, not a deliberate freeze. The convention introduced by this evolution doc — that every parser-affecting PR re-baselines — would have caught the et-0032 regression-fix without us having to reverse-engineer when it happened.

---

### 2026-04-28 — First frozen baseline (B2 / C2)

**Commit**: [`bb744ba`][c-2026-04-28] (PR [#37](https://github.com/sagarinbabel/finnestdb/pull/37) for FI / PR [#42](https://github.com/sagarinbabel/finnestdb/pull/42) for ET)
**Detail**: [`baselines/2026-04-28-fi-summary.md`](baselines/2026-04-28-fi-summary.md), [`baselines/2026-04-28-et-summary.md`](baselines/2026-04-28-et-summary.md), [`baselines/2026-04-28-fi-3way-comparison.md`](baselines/2026-04-28-fi-3way-comparison.md)

The reference floor. First time the parser was measured against gold sets after the eval harness landed in [#29](https://github.com/sagarinbabel/finnestdb/pull/29).

**Dictionary state**:

| Lang | Forms | Lemmas | Sources |
|---|---:|---:|---|
| FI | 12,262,117 | 145,672 | kaikki.org Finnish |
| ET | 228,428 | 12,606 | kaikki.org Estonian |

**Headline numbers** (custom parser):

| Dataset (cases) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| fi-core-v1 (6) | 85.0 | 90.0 | 0.0 | 35.0 | 95.7 |
| fi-manual-v1 (22) | 72.9 | 85.7 | 0.0 | 57.1 | 93.8 |
| fi-grammar-v1 (80) | 96.8 | 98.1 | 0.0 | 51.3 | 99.7 |
| et-grammar-v1 (50) | 87.6 | 97.1 | 2.0 | 42.9 | 94.6 |

**Changes since prior**:

This was the first measurement event. Prior to [#29](https://github.com/sagarinbabel/finnestdb/pull/29) (eval harness, 2026-04-28) the parser was a stub with no gold-set comparison framework — no comparable numbers exist before this date.

**Net effect** (vs. an unmeasured "stub parser" prior):

- Established that `custom` beats `basic` by +28.6pp lemma on real-world FI text (`fi-manual-v1`). Compound + possessive enrichment proven valuable.
- Established that ET kaikki dump produces ~50× fewer forms than FI — flagged as a real coverage limitation that future ET work would need a richer source for. (Resolved on the form-count axis by [#78](https://github.com/sagarinbabel/finnestdb/pull/78)'s Ekilex bulk drop seven days later — though it introduced an accuracy regression, see 2026-05-06 entry.)
- Established that grammar accuracy is 0% across the board because dict coverage is so high that case-suffix enrichment never has to fire.

**Open issues this surfaced**:

- ET compound splitter buggy on `Rongisõit` (et-0032): producing a non-substring lemma. Quietly fixed between this baseline and 2026-05-05.
- ~~Per-case timing rounds to 0ms (eval-tooling limitation; needs nanosecond precision).~~ Resolved 2026-05-06 in PR #103: the eval timer now records `time.Now()` deltas as int64 nanoseconds end-to-end (`samples_ns` in the report JSON; `*_ms` summary fields are now sub-ms float64). Re-baselining the 2026-05-06 datasets surfaced ~40–50k words/s on `custom` for typical Finnish input — see [`baselines/2026-05-06-fi-summary.md#measured-throughput`](baselines/2026-05-06-fi-summary.md#measured-throughput).
- Grammar accuracy at 0% suggests case-suffix rules need to fire alongside dict, or dict needs to ship grammar labels itself.

## Reading this doc

- **Top-down for the latest state.** Most recent entry has the freshest numbers.
- **Trend table for trajectory.** Eyeball whether grammar accuracy has moved off zero or whether ET coverage stayed at 100% after the priority-resolution fix lands.
- **"Open issues this surfaced"** is the action queue. Items here should map to PR titles within a release cycle or be explicitly deferred.

## Reading the data files

Each entry links to one or more JSON reports under [`baselines/`](baselines/) and to summary markdowns of the same date. The JSON reports are the raw output of `cmd/parsertest` (see [`baselines/README.md`](baselines/README.md) for the field schema). The summary markdowns are the human-friendly per-measurement narrative; this doc is the cross-measurement narrative threading them together.
