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
| 2026-05-06 | [`46d8b77`][c-2026-05-06] | **81.4** | **80.0** | 0.0 | 0.0 | **100.0** |
| 2026-05-05 | [`af111c2`][c-2026-05-05] | — | 88.6 | — | 2.0 | 94.6 |
| 2026-05-05 (estnltk ceiling) | [`af111c2`][c-2026-05-05] | — | **98.1** | — | **92.2** | 100.0 |
| 2026-04-28 | [`bb744ba`][c-2026-04-28] | 72.9 | 87.6 | 0.0 | 2.0 | 94.6 |

[c-2026-05-06]: https://github.com/sagarinbabel/finnestdb/commit/46d8b77
[c-2026-05-05]: https://github.com/sagarinbabel/finnestdb/commit/af111c2
[c-2026-04-28]: https://github.com/sagarinbabel/finnestdb/commit/bb744ba

## Entries

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
- Per-case timing rounds to 0ms (eval-tooling limitation; needs nanosecond precision).
- Grammar accuracy at 0% suggests case-suffix rules need to fire alongside dict, or dict needs to ship grammar labels itself.

## Reading this doc

- **Top-down for the latest state.** Most recent entry has the freshest numbers.
- **Trend table for trajectory.** Eyeball whether grammar accuracy has moved off zero or whether ET coverage stayed at 100% after the priority-resolution fix lands.
- **"Open issues this surfaced"** is the action queue. Items here should map to PR titles within a release cycle or be explicitly deferred.

## Reading the data files

Each entry links to one or more JSON reports under [`baselines/`](baselines/) and to summary markdowns of the same date. The JSON reports are the raw output of `cmd/parsertest` (see [`baselines/README.md`](baselines/README.md) for the field schema). The summary markdowns are the human-friendly per-measurement narrative; this doc is the cross-measurement narrative threading them together.
