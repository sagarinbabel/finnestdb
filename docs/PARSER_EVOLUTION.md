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

Two FI lemma columns are tracked: `fi-manual-v1` (curated, 22 cases) for
continuity with prior baselines, and `ud-fi-tdt-test` (UD, 1,554 cases) for
real-world reading. The UD column lights up only on baselines that measured it
(2026-05-07j onward); earlier baselines show "—" because UD wasn't part of the
committed run.

| Date | Commit | FI fi-manual-v1 lemma | FI ud-tdt lemma | ET et-grammar-v1 lemma | FI grammar | ET grammar | ET coverage |
|---|---|---:|---:|---:|---:|---:|---:|
| 2026-05-12a-T1526Z (2026-05-12 PR cascade #183/#185/#187/#188/#189/#191/#193/#195; FI non-finite paradigm coverage in FST + ET +Pers/+Imprs→Voice + ET Inf/Sup/Ger mapping; **UD test sets all up +0.3-1.2pt**, ET grammar Case 78.4→**82.4**) | [`a5a4808`][c-2026-05-12a-T1526Z] | 81.4 | **61.4** | **91.4** | 98.6 | 82.4 | **100.0** |
| 2026-05-07k-T1118Z (FEATS migration + Ekilex bulk drop + FI kaikki backfill; **first measured FEATS lift end-to-end**: FI grammar 59.5→**98.6**, ET grammar 19.6→**78.4**, UD-tdt grammar 22.2→**83.2**) | [`ffd7584`][c-2026-05-07k-T1118Z] | 81.4 | 60.2 | 86.7 | **98.6** | **78.4** | **100.0** |
| 2026-05-07k-feats-rich (PR #139; FEATS-rich gold + dict + adapters end-to-end; **first baseline with non-empty FEATS-attribute table**, but DB had no FEATS yet) | (PR #139) | 81.4 | — | 88.6 | 59.5 | 19.6 | 98.9 |
| 2026-05-07k-T0944Z (post-FEATS re-measure; FST disabled — no production tables; same DB as j) | [`317ab1b`][c-2026-05-07k] | 81.4 | _superseded_ | 88.6 | 59.5 | 19.6 | 98.9 |
| 2026-05-07j (post-fst re-measure on main; case-suffix label stopgap + smoke FST tables; **first UD-at-scale baseline**) | [`42e95d9`][c-2026-05-07j] | 81.4 | **60.2** | 88.6 | **59.5** | **19.6** | 98.9 |
| 2026-05-06i (FST PR 4/4 ships, full suite re-measured) | [`91fecbf`][c-2026-05-06i] | **82.9** | — | 88.6 | 1.4 | 2.0 | **100.0** |
| 2026-05-06h (FST PR 3/4: Giellalt ET) | [`5944733`][c-2026-05-06h] | — | — | 88.6 | — | **2.0** | **100.0** |
| 2026-05-06g (FST PR 2/4: Giellalt FI HFST) | [`7d5cafd`][c-2026-05-06g] | — | — | — | 1.4 | — | — |
| 2026-05-06f (FST PR 1/4: Voikko VFST) | [`d9937c0`][c-2026-05-06f] | — | — | — | **1.4** | — | — |
| 2026-05-06d (Phase 3 ships) | (PR 3.2 head) | 81.4 | — | 88.6 | 0.0 | 0.0 | 100.0 |
| 2026-05-06c (Phase 2 ships) | [`615556e`][c-2026-05-06c] | 81.4 | — | 88.6 | 0.0 | 0.0 | 100.0 |
| 2026-05-06b (post-priority-fix) | [`b327d4f`][c-2026-05-06b] | **81.4** | — | **88.6** | 0.0 | 0.0 | **100.0** |
| 2026-05-06 | [`46d8b77`][c-2026-05-06] | 81.4 | — | 80.0 | 0.0 | 0.0 | 100.0 |
| 2026-05-05 | [`af111c2`][c-2026-05-05] | — | — | 88.6 | — | 2.0 | 94.6 |
| 2026-05-05 (estnltk ceiling) | [`af111c2`][c-2026-05-05] | — | — | **98.1** | — | **92.2** | 100.0 |
| 2026-04-28 | [`bb744ba`][c-2026-04-28] | 72.9 | — | 87.6 | 0.0 | 2.0 | 94.6 |

[c-2026-05-12a-T1526Z]: https://github.com/sagarinbabel/finnestdb/commit/a5a4808
[c-2026-05-07k-T1118Z]: https://github.com/sagarinbabel/finnestdb/commit/ffd7584
[c-2026-05-07k]: https://github.com/sagarinbabel/finnestdb/commit/317ab1b
[c-2026-05-07j]: https://github.com/sagarinbabel/finnestdb/commit/42e95d9
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

### 2026-05-12c — Source-backed ET learner cleanup

**Parser stamp**: `2026.05.12c`
**Scope**: `internal/store/`, `cmd/importekilexdetails/`,
`pkg/lemmatizer-fi-et/lexadverbs`

Follow-up to the Little Prince learner-row cleanup after a manual
Sõnaveeb/Ekilex audit of high-frequency Estonian rows. The change
does not add a new source or a probabilistic disambiguator; it tightens
the deterministic dictionary path so source-backed but bad learner
primaries stop leaking into parse output.

**What changed:**

1. Special-capitalized dictionary lemmas now require exact bare-surface
   matches. The pronoun `ma` and sentence-initial `Ma` no longer match
   abbreviation entries like `mA` or `MA`; exact `mA` and `MA` still
   resolve to their dictionary rows.

2. Runtime FEATS sanitization clears nominal case-only labels from
   invariant closed-class exact rows (`ADV`, `ADP`, `CCONJ`, `SCONJ`,
   `INTJ`, `PART`, `X`) and normalizes exact special-capitalized
   dictionary-form rows to nominative display. This protects existing
   DBs built with stale Ekilex form duplicates.

3. Exact ET verb dictionary forms such as `olema` no longer expose the
   Ekilex `Sup/Ill` morphology as a learner-facing case label; they
   display as `VerbForm=Inf`.

4. The Ekilex details importer now treats `ID` form rows as invariant
   and keeps their FEATS empty. For duplicate bare noun forms, `SgN`
   can overwrite earlier same-key case duplicates with
   `Case=Nom|Number=Sing`, preventing future imports from recreating
   `mA`/`MA` illative or genitive display.

5. ET lexical overlay adds deterministic high-frequency corrections
   for `ei/ADV`, `ma/PRON`, and `sina/PRON`, avoiding nominal or
   ET-only source-language fallback rows in custom mode.

6. Known ET source-language-only trap alternatives such as `kui/NOUN`
   are filtered by exact `(surface, lemma, POS)` before ranking and
   homonym expansion, so stale nominal FEATS cannot beat the useful
   `kui/ADV` or `kui/CCONJ` readings.

7. Learner gloss overrides now cover `ei/ADV`, `et/CCONJ`,
   `kui/CCONJ`, and `olema/VERB` alongside the existing `see/PRON`
   and `väike/ADJ` overrides. These are source-audited presentation
   choices, not invented Sõnaveeb/Ekilex translations.

**Verification**:

- `go test ./internal/store`
- `go test ./cmd/importekilexdetails`
- `go test ./internal/api ./internal/parsecore ./pkg/lemmatizer-fi-et/lexadverbs`
- Local API smoke on `olema see väike ma mA MA ei et kui sina` verified
  `olema -> be`, `see -> this; that`, `väike -> small; little`,
  `ma -> I`, exact-only `mA`/`MA` abbreviation matches, `ei -> no; not`,
  `et -> that`, `kui` with no bogus case label, and
  `sina -> you`.

**Design choices and provenance**: see Decision 22 in
[`DECISIONS.md`](DECISIONS.md).

### 2026-05-12a-T1526Z — FI non-finite paradigm coverage + ET FST FEATS mappings (PRs #188/#189/#191)

**PRs**: [#188](https://github.com/sagarinbabel/finnestdb/pull/188), [#189](https://github.com/sagarinbabel/finnestdb/pull/189), [#191](https://github.com/sagarinbabel/finnestdb/pull/191) (also includes follow-ons [#193](https://github.com/sagarinbabel/finnestdb/pull/193) and [#195](https://github.com/sagarinbabel/finnestdb/pull/195))
**Commit measured**: [`a5a4808`][c-2026-05-12a-T1526Z] (= `main` head; merge of PR #195)
**Run started**: 2026-05-12T15:26Z (UTC, FI); 15:34Z (UTC, ET — same baseline)
**Detail**: [`baselines/2026-05-12a-T1526Z-fi.md`](baselines/2026-05-12a-T1526Z-fi.md), [`baselines/2026-05-12a-T1526Z-et.md`](baselines/2026-05-12a-T1526Z-et.md)
**Parser version stamp**: `2026.05.12a` (`parsecore.ParserVersion` — unchanged; this baseline is the second freeze on `2026.05.12a`, after `2026-05-12a` which only measured analyzer-traps gold)

First end-to-end baseline that captures the 2026-05-12 PR cascade. Two FST tables were regenerated this session:

1. **FI FST table** via `make gen-lemmatizer-tables-fi VFST_PATH=…` — wordlist grew from ~195k (lemmas-only) to 482,835 surfaces via the new `cmd/genlemmatizerwordlist` from PR #188 (lemmas + 180,341 non-finite dict surfaces + 114,180 synthesised A-inf-long candidates). `fi_min.json` 79 MB → 255 MB. Captures PR #188's non-finite paradigm coverage.
2. **ET FST table** via `cmd/genlemmatizertables -lang et -hfstol …` against Giellalt lang-est analyser (`md5 ce93843c…`). Wordlist reused the 138,237 keys from the prior 2026-05-07 ET table to hold coverage constant; new mappings from #189/#191 now apply at table-gen time. Captures PR #189's Sup/Ger/Inf corrections and PR #191's +Pers/+Imprs → Voice tagging.

**Dictionary state at measurement.** Same FEATS-rich DB as 2026-05-07k. **No Ekilex reimport was performed this session**, so PR #189's importer fix is **not yet reflected in the dict** (e.g. `õppida → õppima` still has stale `VerbForm=Sup` instead of `Inf`). A follow-up baseline after `make reduce-ekilex` + `make import-ekilex-details-et` will measure the dict-side half of #189.

**Headline numbers** (custom parser):

| Dataset (cases) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| _Curated FI sets:_ | | | | | |
| fi-analyzer-traps (20, new) | **100.0** | **100.0** | **100.0** | **100.0** | 98.8 |
| fi-grammar (80) | 96.8 | 98.1 | **98.6** | 50.6 | 99.7 |
| fi-core (6) | 85.0 | 90.0 | **100.0** | 45.0 | 95.7 |
| fi-manual-v1 (22) | 81.4 | 85.7 | **60.0** | 32.9 | 90.7 |
| fi-manual-v2 (4) | 88.9 | 100.0 | **100.0** | 44.4 | 100.0 |
| _UD FI test sets:_ | | | | | |
| ud-fi-ftb-test (1867) | 71.4 | 67.4 | **84.0** | 28.6 | 93.1 |
| ud-fi-ood-test (2106) | 63.4 | 66.5 | **80.2** | 21.6 | 85.6 |
| ud-fi-pud-test (1000) | 60.9 | 66.8 | **78.9** | 21.1 | 85.9 |
| ud-fi-tdt-test (1554) | 61.4 | 68.8 | **83.6** | 21.7 | 90.0 |
| _Curated ET sets:_ | | | | | |
| et-analyzer-traps (11, new) | **100.0** | **100.0** | 0.0 | **100.0** | 100.0 |
| et-grammar (50) | **91.4** | **93.3** | **82.4** | 80.0 | 100.0 |
| et-manual (4) | 88.9 | 88.9 | **83.3** | 66.7 | 100.0 |

**Net effect vs `2026-05-07k-T1118Z`** (custom parser, curated + UD sets):

| Dataset | Δ Lemma | Δ POS | Δ Grammar | Δ Full |
|---|---:|---:|---:|---:|
| fi-core / fi-grammar / fi-manual-v1 / fi-manual-v2 | flat | flat | flat | flat |
| ud-fi-ftb-test | +0.0 | **+1.0** | **+0.4** | **+0.3** |
| ud-fi-ood-test | **+0.9** | **+0.9** | **+0.6** | **+0.4** |
| ud-fi-pud-test | **+0.9** | **+0.8** | **+0.4** | **+0.3** |
| ud-fi-tdt-test | **+1.2** | **+1.0** | **+0.4** | **+0.4** |
| et-grammar | **+4.7** | **+1.9** | **+4.0** | **+1.0** |
| et-manual | **−11.1** | **−11.1** | +0.0 | **−11.1** |

**Reading.**

- **FI curated sets at ceiling** — already near-100% on lemma/POS/grammar; PR #188's non-finite coverage doesn't move them. fi-analyzer-traps reaches 100% (was 95.2% in #183's mini-baseline; PR #188's FST regen fixes the documented `tarjoamaan → tarjoama` residual).
- **FI UD sets: small consistent positive Δ** across all four, on every metric. This is where PR #188's non-finite coverage shows up — `mennäkseen → mennä`, `tarjoamaan → tarjota`, etc. are naturally-distributed in UD test corpora but absent from curated grammar sets.
- **ET grammar (50 cases / 105 scored tokens, the statistically meaningful set) lifts on every metric** — Case 78.4 → 82.4 (+4.0pt), lemma 86.7 → 91.4 (+4.7pt). Improvements appear to come from PR #185/#187 dict-side ranking and lexadverbs overlay, not (yet) from PR #189 FEATS — that needs an Ekilex reimport.
- **ET manual regresses by one verb token** (`peatus → peatu/ADJ` instead of `peatuma/VERB`). With only 9 scored tokens, one miss = 11% drop. Reproduces with both stale and fresh ET FST tables → root cause is dict-side, likely PR #187's source-priority-first ranking. See [`baselines/2026-05-12a-T1526Z-et.md`](baselines/2026-05-12a-T1526Z-et.md) for the trace.
- **Per-FEATS-attribute table on et-grammar** shows Case +4.0pt (real fix). The one-token drops on Mood/Tense/Voice and the net Person/VerbForm drops come from `naeris` changing from `naerma/VERB` to `naeris/NOUN`; existing `lugesid`/`läksid` Person misses and the pending `õppida` VerbForm miss remain unchanged.

**Open issues this surfaced** (carried to follow-ups, not blocking this freeze):

1. **`peatus → peatu/ADJ` regression** (et-manual) — kind of dictionary-entry attachment that matters for learners. Investigate PR #187 ranking interaction.
2. **Verb/noun/PROPN homograph picks** on ET — `joon` ("I drink" vs "line"), `naeris` ("laughed" vs "turnip"), and `Eestis`. FST table emits both verb/noun readings for the verb cases; runtime tie-break is now picking NOUN where 2026-05-07k picked VERB/PROPN. `õuna` remains the unchanged `õud/NOUN` lemma miss from 2026-05-07k.
3. **Ekilex reimport pending** — PR #189's dict-side fix doesn't show up here; expected to fix `õppida` and friends. Run `make reduce-ekilex` + `make import-ekilex-details-et` and re-freeze.

### 2026-05-12a — Little Prince learner-row cleanup

**Parser stamp**: `2026.05.12a`
**Scope**: `internal/parsecore/`, `internal/api/`, `internal/store/`

Fixed a gap between high token coverage and learner-visible row
quality on the Estonian Little Prince browser parse. The parser had
resolved nearly every token, but aggregate rows could still display
misleading grammar labels and low-value dictionary alternatives.

**What changed:**

1. Aggregate grammar labels now survive only when every attached form
   has one normalized form and learner-facing POS is appropriate for
   row-level morphology. This suppresses misleading labels such as
   `olema` showing an illative badge.

2. High-impact Estonian learner gloss overrides correct known bad
   primaries for `see/PRON` and `väike/ADJ` while preserving custom
   gloss rows.

3. Inspect expansion and low-value alternative filtering now avoid
   copying grammar onto mixed-form expansions, suppress `[ET]`
   fallback gloss alternatives when English glosses are available,
   demote `X` alternatives, and remove uppercase acronym homonyms
   such as `TA/NOUN` when a useful lowercase analysis exists.

4. Deck details and review cards post-process glosses through the
   same lookup path as parse results so the learner surfaces stay in
   sync.

**Verification**:

- `go test ./...`
- Little Prince ET API smoke on `localdata/et-corpus/epub/per-book/lilprince.txt`:
  `11364` resolved tokens, `185` unresolved tokens, `3660` punctuation
  tokens. Targeted bad rows (`TA/NOUN`, `Ta/X`, `ei INTJ/NOUN/X`) no
  longer appeared; `olema`, `see`, and `väike` no longer carried the
  reported bad learner display.

### 2026-05-12a — Analyser-quality, alternative filtering, and ranker fixes (PRs #183/#185/#187)

**PRs**: [#183](https://github.com/sagarinbabel/finnestdb/pull/183), [#185](https://github.com/sagarinbabel/finnestdb/pull/185), [#187](https://github.com/sagarinbabel/finnestdb/pull/187)
**Scope**: `pkg/lemmatizer-fi-et/{lexadverbs,udfeats}`, `internal/store/dict.go`, `internal/api/handlers.go`, `cmd/importdict/`, `testdata/parser-eval/{fi,et}/gold/`
**Parser version stamp**: `2026.05.12a` (`parsecore.ParserVersion`)

Pulled five analyser-quality fixes back into finnestdb from yle_subs
— the downstream Anki-deck builder that consumes `wordlist.tsv` /
`sentences_user_friendly.tsv`. Each fix was a real-world bug yle_subs
had already patched at the deck-builder layer with a manual override;
moving the rule into the parser/dict closes the leak for every
consumer.

**What changed:**

1. **`pkg/lemmatizer-fi-et/lexadverbs`** — new package with FI + ET
   overlay tables. When a surface like `tuskin`, `varsin`, `peale`,
   `välja` is looked up in custom mode, the curated analysis
   short-circuits at the top of `BatchLookupForms` (new Step 0)
   before any dict or FST work. Source tag `lex-overlay`. The
   overlay is custom-mode-only so basic baselines stay stable.

2. **`udfeats.NormalizeMaInfinitive` + `IsMaInfinitiveSurface`** —
   new exported helpers in
   `pkg/lemmatizer-fi-et/udfeats/udfeats.go`. Rewrites FEATS on
   Finnish verb surfaces ending in -maan/-mään/-massa/-mässä/
   -masta/-mästä/-malla/-mällä/-matta/-mättä with a matching
   `Case=` attribute: strips spurious `Person=3|Number=Sing` and
   asserts `Case=X|InfForm=Ma|VerbForm=Inf|Voice=Act`. `Voice=Act`
   is added only when no Voice attribute is present, preserving
   explicit `Voice=Pass`.

3. **MA-infinitive ranking bias** in
   `pickBestResolutionCandidate`. Demotes any candidate whose
   lemma ends in `-ma`/`-mä` on a MA-infinitive surface regardless
   of POS (kaikki occasionally tags derived nouns as VERB);
   promotes VERB candidates with `InfForm=Ma`. Paired with the
   normaliser above so dict-supplied feats get rewritten via
   `formResolutionFromCandidate` before ranking sees them.

4. **Bad-lemma blocklist** (two tiers) in
   `lookupFormCandidates`:
   - `alwaysBadDictLemmasFI` — never-legitimate fragments
     (`as`, `taa`, `ku`, `sisä-`, `ylä-`, `poli`). Filtered
     regardless of surface.
   - `badSurfaceLemmaFI` — (surface, lemma) pairs that are bad
     for the trap surface but legitimate elsewhere
     (`(varsin, varsi)`, `(vuotta, vuo)`, etc.). Bare-lemma
     lookups like `varsi → varsi/NOUN` are preserved.

5. **Structural-gloss filter at kaikki ingest** —
   `cmd/importdict/structural.go` rejects Wiktionary "form-of"
   restatements (`inflection of`, `partitive singular of X`,
   `past active participle of`, `alternative form of`, etc.) as
   the primary gloss. Two filter points in
   `cmd/importdict/main.go`: `lemmas.gloss` cache picks the first
   meaning gloss across senses; the `translations` table skips
   structural rows entirely so they cannot displace real glosses
   under `BatchLookupGlosses`' `ORDER BY sense_idx ASC`. Pattern
   uses `\pL` (Unicode letter class) so non-ASCII headwords
   (`ääni`, `õun`, `ümar`) match correctly.

6. **`BatchLookupSenses` multi-sense API** in `internal/store/
   dict.go` — sibling to `BatchLookupGlosses` returning the full
   ranked sense list `[]Sense{Text, SenseIdx, Source,
   SourcePriority}`. Same ranking order so the first element
   matches `BatchLookupGlosses`. Unblocks downstream
   contextual-gloss work on polysemous lemmas.

7. **Analyser-traps gold fixtures** —
   `testdata/parser-eval/fi/gold/fi-analyzer-traps-v1.json`
   (20 cases) and `testdata/parser-eval/et/gold/
   et-analyzer-traps-v1.json` (11 cases). Each entry is a known
   yle_subs bug report. Auto-picked up by
   `make compare-parsers{,-et}` via the `*.json` glob.

8. **Deck/parse low-value alternative suppression** from PR #185 —
   `internal/api/handlers.go` filters `BatchLookupAllForms` output
   when a surface has at least one non-empty lexical-base alternative:
   empty-gloss candidates and Wiktionary form-of alternatives are
   suppressed before parse-overview and deck-ingest expansion. Gap
   surfaces where every candidate is empty-gloss are preserved.

9. **Source-priority-first dict/FST ranking** from PR #187 —
   `pickBestResolutionCandidate` now lets dictionary `source_priority`
   outrank generic FST support and morphology-density tie-breaks after
   the case/POS sanity checks and the narrow weak-legacy FST escape
   hatch. Higher-authority Ekilex rows therefore beat lower-priority
   kaikki rows once the known regression guards tie.

**Measurement** (custom parser, against the new #183 gold fixtures):

| Dataset | Lemma | POS | Full |
|---|---:|---:|---:|
| fi-analyzer-traps-v1 | 95.2 | 100.0 | 95.2 |
| et-analyzer-traps-v1 | 100.0 | 100.0 | 100.0 |

The one documented FI residual is `tarjoamaan → tarjoama` instead
of `tarjota`: production FST tables don't ship MA-infinitive
surface entries at all (verified via direct lookup), so the
dict-only candidate is the only one available and runtime lemma
reconstruction would need a Finnish morphological generator.
POS/FEATS/gloss are all correct; only the verb-lemma is missing.
Closing this fully needs FST table regeneration with MA-infinitive
inflected forms included — separate scope.

**Design choices and provenance**: see Decisions 19, 20, and 21 in
[`DECISIONS.md`](DECISIONS.md). The yle_subs source files are
referenced from code comments at every fix point so future audits
can trace each rule back to the learner-visible bug it patched.

---

### 2026-05-07 — Voikko `[P4]` Voice + participle field cleanup (PR #158)

**PR**: [#158](https://github.com/sagarinbabel/finnestdb/pull/158)
**Scope**: `pkg/lemmatizer-fi-et/voikkomap` only

Two surgical fixes on top of the rich FEATS extraction shipped in PRs
[#154](https://github.com/sagarinbabel/finnestdb/pull/154) and
[#155](https://github.com/sagarinbabel/finnestdb/pull/155). Closes the
Voice accuracy gap flagged in the parser audit (FI custom 5.3% vs
omorfi 89.7% on fi-ftb).

**What changed:**

1. **`[P4]` → `Voice=Pass` (no `Person=4` leak).** Finnish passive is
   grammatically "4th person" in Voikko's FST, but UD `Person` is
   1/2/3. `[P4]` now sets `Voice=Pass` only; `[P1-P3]` set `Voice=Act`
   alongside `Person`, so active finite verbs stop composing FEATS
   without Voice.
2. **`applyParticiple` clears finite-only fields.** When `[R*]` wins,
   `Mood`, `Tense`, and `Person` are reset so a participle never
   composes contradictory FEATS like `Tense=Past|VerbForm=Part` —
   Finnish UD encodes the past/present participle distinction in
   `PartForm=`, not `Tense=`.

**What did NOT change** (already on main before this PR):

- The 7-param `udfeats.Compose` and `udfeats.ComposeMap` signatures
  shipped in PR #155 / #154.
- The `Voice`, `VerbForm`, `PartForm`, `InfForm`, `Degree`, `PronType`,
  `PersonPsor`, `NumberPsor`, `Clitic`, `NumType`, `Connegative`,
  `AdpType` fields on `voikkomap.Analysis` — added in PR #154.
- `applyParticiple` itself, including `Voice=Pass` on TU/TAVA
  passive participles — added in PR #154.
- Giellalt's `Act`/`Pass`/`Inf*`/`PrsPrc`/`PrfPrc` extraction —
  added in PR #155.

**`[E*]` finding (no code change):** Investigated as a possible voice
signal and found to encode connegative status (`Ef`=false, `Et`=true,
`Eb`=both), confirmed from libvoikko's
`FinnishVfstAnalyzer.cpp::parseBasicAttributes`. Not projected to UD —
the runtime gets `Connegative=Yes` from the orthogonal `[Cn]` tag
(handled in `applyComparison`).

**Expected eval impact** (pending table regen + re-measurement):

- Voice accuracy on fi-ftb should jump from 5.3% toward omorfi's 89.7%.
  Every active finite verb in the regenerated FI table will carry
  `Voice=Act`; passive forms will carry `Voice=Pass` without spurious
  `Person=4`.
- No lemma/POS/grammar regressions expected — those fields are
  untouched.

**Measurement deferred**: exact numbers will be added as a dated
sub-entry once production tables are regenerated and
`make compare-parsers` is re-run.

---

### 2026-05-07k-T1118Z — FEATS migration measured end-to-end (first DB-with-FEATS run)

**Commit measured**: [`ffd7584`][c-2026-05-07k-T1118Z] (= `main` head; merge of PR [#139](https://github.com/sagarinbabel/finnestdb/pull/139))
**Run started**: 2026-05-07T11:18Z (UTC)
**Detail**: [`baselines/2026-05-07k-T1118Z-fi.md`](baselines/2026-05-07k-T1118Z-fi.md), [`baselines/2026-05-07k-T1118Z-et.md`](baselines/2026-05-07k-T1118Z-et.md)
**Parser version stamp**: `2026.05.07k` (`parsecore.ParserVersion`)

First baseline that captures the FEATS-data-thread migration end-to-end on a DB with FEATS populated. The §2026-05-07k-feats-rich entry below shipped the wiring; this entry measures it. Two prerequisite imports were run this session before measurement:

1. **`make reduce-ekilex` + `make import-ekilex-details-et`** — loaded the 6.18M ET form rows with `morph_code → FEATS` projection. ET FEATS coverage in DB went from 0% → **96.0%**. Pipeline: cached raw Ekilex API responses (`localdata/ekilex/details/raw/`, ~1 GB, populated by an earlier `make fetch-ekilex` scrape) → `make reduce-ekilex` (~2 min) → `cmd/importekilexdetails` (~30 sec). 178k unique (lemma, pos) entries; 5.96M form rows touched.
2. **`cmd/importdict -backfill-feats`** — projected Wiktionary tag arrays into UD FEATS for 26.6M FI form rows in-place. FI FEATS coverage in DB went from 0% → **99.3%**. Pipeline: download `kaikki.org-dictionary-Finnish.jsonl.gz` (248 MB compressed, ~22 sec on a fast connection) → `cmd/importdict -lang fi -file ...gz -backfill-feats -source-key kaikki ...` (~2 min for the 26.6M `WHERE feats IS NULL` updates).

**Headline numbers** (custom parser):

| Dataset (cases / tokens) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| _Curated FI sets:_ | | | | | |
| fi-grammar (80 / 156) | 96.8 | 98.1 | **98.6** | 50.6 | 99.7 |
| fi-core (6 / 23) | 85.0 | 90.0 | **100.0** | 45.0 | 95.7 |
| fi-manual-v1 (22 / 187) | 81.4 | 85.7 | **60.0** | 32.9 | 91.2 |
| fi-manual-v2 (4 / 12) | 88.9 | 100.0 | **100.0** | 44.4 | 100.0 |
| _UD FI test sets:_ | | | | | |
| ud-fi-ftb-test (1867 / 13,973) | 71.4 | 66.4 | **83.6** | 28.3 | 92.5 |
| ud-fi-ood-test (2106 / 16,151) | 62.5 | 65.6 | **79.6** | 21.2 | 85.0 |
| ud-fi-pud-test (1000 / 13,474) | 60.0 | 66.0 | **78.5** | 20.8 | 85.5 |
| ud-fi-tdt-test (1554 / 17,951) | 60.2 | 67.8 | **83.2** | 21.3 | 89.6 |
| _Curated ET sets:_ | | | | | |
| et-grammar (50 / 178) | 86.7 | 91.4 | **78.4** | 79.0 | 100.0 |
| et-manual (4 / 12) | 100.0 | 100.0 | **83.3** | 77.8 | 100.0 |

**Net effect vs `2026-05-07j`** (custom parser; both DB-state-of-art and FEATS migration accumulated between the two):

- **FI grammar accuracy lifts: +39 to +70pp across every FI dataset.** kaikki tag arrays (`["illative","singular"]`) now project to UD FEATS at import time via `cmd/importdict/feats.go::kaikkiTagsToFeats`. Custom matches omorfi exactly on `fi-core` (100%) and `fi-manual-v1` (60%); within 1.4pp of omorfi on `fi-grammar` (98.6 vs 100.0). UD-FI grammar accuracy lands in the 78.5–83.6% band.
- **ET grammar accuracy lifts: +59 to +83pp.** `et-grammar` 19.6 → 78.4 (LEARNINGS.md projected ~95%; the remaining 16pp gap to estnltk's 94.1% is on contextual-disambiguation cases). `et-manual` 0.0 → 83.3.
- **ET lemma jumped on et-manual: 77.8 → 100.0** (+22.2pp). The bulk Ekilex drop's long-tail form coverage closed the cases the kaikki+public-headwords slice was missing.
- **Lemma/POS slight regressions on et-grammar: 88.6 → 86.7 lemma (−1.9pp), 96.2 → 91.4 POS (−4.8pp).** Ekilex bulk introduces multi-lemma homonyms (e.g. PROPN/NOUN tied surfaces) that the dict ranker mishandles at bulk scale. Same kind of issue [`b327d4f`](https://github.com/sagarinbabel/finnestdb/commit/b327d4f) (PR 0.5) fixed for the public-headwords slice; recurs at bulk scale. **Action**: ranker tweak follow-up to recover the lost pp without regressing grammar.
- **Full% drops vs j across the board (−5 to −31pp on FI; ET +28 to +67pp despite the shift).** `Match.Full` is now FEATS-strict (PR [#130](https://github.com/sagarinbabel/finnestdb/pull/130)) — every gold-supplied FEATS attribute must match. FI Full% reflects the mismatch on Number/Tense/Mood/Person attributes between dict-projected FEATS and gold FEATS. The remaining gap to omorfi/estnltk Full% is the per-attribute disambiguation that contextual analysis does and the dict-only path doesn't.

**Open issues this surfaced**:

- **Per-FEATS-attribute table now non-empty** — but the FI side wasn't measured against UD because UD gold's FEATS shape needs alignment with what `kaikkiTagsToFeats` produces (e.g. PronType, InfForm/PartForm). [`baselines/2026-05-07-feats-rich-fi.md`](baselines/2026-05-07-feats-rich-fi.md) holds the per-attribute gold reference. Next step: stratify the per-attribute table by analyzer family to see where custom's tag-projected FEATS disagree with gold FEATS systematically.
- **Ranker regression on et-grammar at bulk-Ekilex scale.** Loss of 1.9pp lemma / 4.8pp POS / 4.0pp grammar on et-grammar suggests the source-priority + case-match heuristics in `pickBestFormCandidate` need revalidation against the Ekilex bulk shape. Filed as a follow-up; expect a PR that revisits the ranker tiebreak rules and re-baselines.
- **The "FI FEATS muted" issue from §2026-05-07k-feats-rich is resolved.** That entry's "live DB lacks FEATS — until the user runs `cmd/importdict -lang fi -backfill-feats`..." was the gating step. Done in this session.
- **FST runtime still off** — `localdata/lemmatizer-fi-et/tables/` is empty here, so the FST step doesn't fire. The FEATS lift in this baseline comes from the dict-import-time projection, not the FST runtime. Maintainer-local table generation would unlock the FST contribution on top of the existing lift.

**Convention note**: the `THHMMZ` suffix means UTC run start time. This is the second `2026-05-07k` baseline of the day (after `T0944Z`, which has been superseded by this run since FI/ET FEATS were not yet loaded then). Multiple baselines on the same parser-behavior version are distinguished by run timestamp. See [`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md).

---

### 2026-05-07k-feats-rich — FEATS-rich gold + dict + adapters end-to-end (first non-empty FEATS-attribute table)

**Commit**: this PR
**Detail**: [`baselines/2026-05-07-feats-rich.md`](baselines/2026-05-07-feats-rich.md), [`baselines/2026-05-07-feats-rich-fi.md`](baselines/2026-05-07-feats-rich-fi.md), [`baselines/2026-05-07-feats-rich-et.md`](baselines/2026-05-07-feats-rich-et.md)
**Parser version stamp**: `2026.05.07k`

Closes the data-side gap that left the FEATS-attribute eval table empty on every prior baseline. The eval framework's per-attribute scorer ([`internal/eval/eval.go:380-407`](../internal/eval/eval.go)) was complete since [#130](https://github.com/sagarinbabel/finnestdb/pull/130), but it only fires when gold tokens carry `feats`. Until this entry, no committed gold set did.

This entry lands six independently-shippable changes that together make every linguistic surface in the project speak the same UD FEATS vocabulary:

1. **`cmd/importdict/feats.go`** — `kaikkiTagsToFeats` projects Wiktionary tag arrays (`["illative","singular"]` etc.) into UD FEATS strings on every form row imported from kaikki.org. Coverage: Case (15 entries), Number, Person, Tense, Mood, Voice, VerbForm, Degree (Adj), Reflex (lexical-static for `itse`/`enese`/`enda`), PronType. Wired through `upsertFormSQL` and a new `-backfill-feats` mode that updates `WHERE feats IS NULL` rows in place against an existing DB.
2. **`internal/store/dict.go::featsFromCaseLabel`** — when the case-suffix-strip path is the only signal that resolves a form, it now projects `Case=` into FEATS via `udfeats.LegacyLabelToUDCase`. Number/Tense/etc. stay empty because the suffix alone can't disambiguate them.
3. **`pkg/lemmatizer-fi-et/udfeats/`** — new shared package that holds the canonical `LegacyLabelToUDCase` / `UDCaseToLegacyLabel` maps and the `Compose(grammarLabel, number, tense, mood, person)` function. Both `voikkomap.Parse` and `giellaltmap.Parse` now compose `Analysis.Feats` at parse time, so generated FST tables are self-describing on disk; `dict.go::featsFromFSTAnalysis` prefers the persisted field and falls back to recomposing for legacy table files (backwards-compatible).
4. **`scripts/_vabamorf_feats.py`** — Vabamorf form code → UD FEATS mapper, parallel to `cmd/importekilexdetails/feats.go::ekilexMorphToFeats` but for the analyzer's own form codes (`b`, `sin`, `ks`, `tud`, `mas`, …). The EstNLTK adapter at `scripts/estnltk_adapter_example.py` now emits the same UD FEATS shape Omorfi already did, replacing the earlier `{vabamorf_form: <code>}` placeholder.
5. **`cmd/enrichgoldfeats/`** — new tool that seeds UD FEATS into the 6 manual gold sets by running the language-appropriate adapter on each case's text, deterministically anchoring `Case=` to the gold's existing `grammar_label`, and writing back the gold JSON plus a `.diff.md` audit log flagging tokens that need a manual look (OOV compounds, case disagreements). Applied across `fi-core-v1`, `fi-grammar-v1`, `fi-manual-v1`, `fi-manual-v2`, `et-grammar-v1`, `et-manual-v1` (~370 tokens).
6. **Smoke FST tables (`testdata/lemmatizer/{fi,et}_min.json`)** — regenerated to include the new `Feats` field so the runtime composer's "prefer persisted" fast path is exercised by tests.

**Headline numbers** (custom parser, lemma/POS unchanged from `2026-05-07j` because the parser's resolution behavior didn't change here — the eval just has more to compare against):

| Dataset | Cases / tokens | Lemma | POS | Grammar | Full | Coverage |
|---|---|---:|---:|---:|---:|---:|
| fi-core | 6 / 23 | 85.0 | 90.0 | 30.0 | 0.0 | 95.7 |
| fi-grammar | 80 / 156 | 96.8 | 98.1 | 59.5 | 0.0 | 99.7 |
| fi-manual-v1 | 22 / 187 | 81.4 | 85.7 | 6.7 | 0.0 | 91.2 |
| fi-manual-v2 | 4 / 12 | 88.9 | 100.0 | 33.3 | 11.1 | 100.0 |
| et-grammar | 50 / 178 | 88.6 | 96.2 | 19.6 | 0.0 | 98.9 |
| et-manual | 4 / 12 | 77.8 | 77.8 | 0.0 | 0.0 | 91.7 |

The `Full` column drops to ~0 across the board because it's now correctly gated on per-attribute FEATS equality (per [#130](https://github.com/sagarinbabel/finnestdb/pull/130)) and the live DB this measurement runs against has no FEATS yet — see Open issues below. The pre-`2026-05-07j` `Full` column was implicitly ignoring FEATS, so it overstated correctness on whatever non-trivial FEATS the gold set had. This is the first baseline where `Full` is honest about FEATS.

**FEATS-attribute table — the new metric** (per dataset; full table in [`baselines/2026-05-07-feats-rich-fi.md`](baselines/2026-05-07-feats-rich-fi.md) / `-et.md`):

| Dataset | FEATS attributes covered | omorfi/estnltk eligible | omorfi/estnltk correct |
|---|---|---:|---:|
| fi-core | 13 (Case, Degree, Mood, Number, Number[psor], Person, Person[psor], PronType, Style, Tense, VerbForm, Voice + 1 more) | 56 | 56 (100.0%) |
| fi-grammar | 13 (above + InfForm, PartForm) | 449 | 448 (99.8%) |
| fi-manual-v1 | 8 (Case, Mood, Number, Number[psor], Person, Person[psor], Tense, VerbForm, Voice) | 26 | 26 (100.0%) |
| fi-manual-v2 | 8 | 26 | 26 (100.0%) |
| et-grammar | 7 (Case, Mood, Number, Person, Tense, VerbForm, Voice) | 273 | 270 (98.9%) |
| et-manual | 7 | 20 | 20 (100.0%) |

External analyzers score ≥99% on every dataset because the gold seeds were drawn from those analyzers' own output (with gold's existing `grammar_label` deterministically anchoring Case=). `basic` and `custom` score 0% on every attribute because the live DB has `forms.feats IS NULL` for all 27.2M rows — see "Open issues this surfaced" below.

**Pipeline diagram (new state):**

```
[kaikki JSONL]    --tags-> [kaikkiTagsToFeats] -+
[Ekilex morph]    --code-> [ekilexMorphToFeats] +-> forms.feats (SQLite)
                                                |
[Voikko VFST]     --tags-> [voikkomap.Parse]    +-> Analysis.Feats
[Giellalt HFST]   --tags-> [giellaltmap.Parse]  +     (via udfeats.Compose)
                                                |
[case-suffix]     --label-> [featsFromCaseLabel]+-> custom parser Token.Feats
                                                |
[omorfi adapter]  --ufeats->                    |
[estnltk adapter] --vabamorf_form_to_feats->    |
                                                v
                                         [eval per-FEATS-attr table]
```

**Open issues this surfaced**:

- **Live DB lacks FEATS**: the production DB used for this measurement was last imported on 2026-05-05, before any of the FEATS mappers landed. Until the user runs `go run ./cmd/importdict -lang fi -reimport -db finnestdb.db -file localdata/kaikki.org-fi.jsonl.gz` (or the matching `-backfill-feats` mode), the `custom` parser has no FEATS to emit and scores 0% on every FEATS attribute. The kaikki JSONL files aren't on the maintainer's machine right now — re-importing requires re-fetching from kaikki.org. Capturing this as a follow-up rather than blocking the entry, because the wiring is unit-tested ([`cmd/importdict/feats_test.go`](../cmd/importdict/feats_test.go), [`internal/store/dict_test.go::TestCaseSuffixStrip_Inessive`](../internal/store/dict_test.go)) and the omorfi/estnltk reference rows on this baseline prove the eval framework is correct.
- **Vabamorf adapter polarity**: estnltk's morph_analysis layer emits Polarity=Neg only when sentence-level context (`ei` particle) is visible. The new `_vabamorf_feats.py` mapper threads it through, but only ~3 of the 6 gold sets exercise polarity — coverage is honest about which tokens it appears on.
- **enrichgoldfeats diff reports** flag 16 tokens in `fi-manual-v1` and 1 in `fi-grammar-v1` where omorfi has no analysis (mostly real-world news compounds like `Pokemon-yhteistyö`, `peruutusvakuutuksia`). The case-suffix-strip fallback covered most of them; nominative singular compounds where the surface has no inflectional marker stay empty in FEATS by design.
- **The fi-manual-v1/v2 slug collision** in `parser-comparison.sh` was worked around in this baseline by re-running v1 explicitly via `cmd/parsertest -out`. Resolved 2026-05-07: both comparison scripts now slug from the input file basename instead of `dataset.name`.
- **Resolves the "curated gold doesn't have FEATS" open issue** flagged in §2026-05-07k-T0944Z below — that entry's "small JSON-edit job, gated on whether the curated sets need to keep tracking the analyzer ceiling closely" was decided yes, and is now done.

---

### 2026-05-07k-T0944Z — Post-FEATS re-measure (FST disabled, dict + case-suffix path)

**Commit measured**: [`317ab1b`][c-2026-05-07k] (= `main` head; merge of PR [#133](https://github.com/sagarinbabel/finnestdb/pull/133) which committed the j baseline)
**Run started**: 2026-05-07T09:44Z (UTC)
**Detail**: [`baselines/2026-05-07k-T0944Z-fi.md`](baselines/2026-05-07k-T0944Z-fi.md), [`baselines/2026-05-07k-T0944Z-et.md`](baselines/2026-05-07k-T0944Z-et.md), [`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md)
**Parser version stamp**: `2026.05.07k` (`parsecore.ParserVersion`)

Re-runs the same 8 FI + 2 ET datasets used in §2026-05-07j on `main` after the FEATS-migration stack landed: PR [#127](https://github.com/sagarinbabel/finnestdb/pull/127) (FST step promoted to parallel scorer in dict step 1), [#128](https://github.com/sagarinbabel/finnestdb/pull/128) (lemmatizer tables migrated to disk-loaded `localdata/lemmatizer-fi-et/tables/` with `testdata/lemmatizer/` fixtures), [#129](https://github.com/sagarinbabel/finnestdb/pull/129) (full FST candidate merge), [#130](https://github.com/sagarinbabel/finnestdb/pull/130) (FEATS migration — gate `Match.Full` on FEATS, attach FEATS-only custom morphology, thread per-attribute FEATS through eval and report), [#131](https://github.com/sagarinbabel/finnestdb/pull/131) (data consolidation under `localdata/`).

**Two simultaneous reasons curated headline numbers don't move vs j**:

1. **Eval semantics (PR #130) — gold-driven.** `Match.Full` now requires per-attribute FEATS matching when the gold supplies a FEATS string. Curated FI/ET gold sets don't carry FEATS in annotations, so `Match.Full` falls back to pre-FEATS semantics (lemma & POS & grammar_label). UD test sets DO carry full UD FEATS, so Full% on UD reflects stricter matching.
2. **FST runtime (PR #128) — disabled on this measurement.** The runtime now disk-loads tables from `localdata/lemmatizer-fi-et/tables/`. That directory is empty on this machine (only `testdata/lemmatizer/` holds 12-form smoke fixtures used by tests, not by the production lookup). The runtime treats the missing tables as `ErrNoTables` → "FST disabled, fall through to dict + case-suffix path". So this baseline measures the **dict + case-suffix stopgap with FST off** — same effective parser as the j baseline (which also had FST off because j's smoke fixtures only covered 12 forms).

**Headline numbers** (custom parser):

| Dataset (cases / tokens) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| _Curated FI sets:_ | | | | | |
| fi-grammar (80 / 156) | 96.8 | 98.1 | 59.5 | 79.5 | 99.7 |
| fi-core (6 / 23) | 85.0 | 90.0 | 30.0 | 50.0 | 95.7 |
| fi-manual-v1 (22 / 187) | 81.4 | 85.7 | 6.7 | 64.3 | 91.2 |
| fi-manual-v2 (4 / 12) | 88.9 | 100.0 | 33.3 | 66.7 | 100.0 |
| _UD FI test sets (real-world):_ | | | | | |
| ud-fi-ftb-test (1867 / 13,973) | _PENDING_ | _PENDING_ | _PENDING_ | _PENDING_ | _PENDING_ |
| ud-fi-ood-test (2106 / 16,151) | _PENDING_ | _PENDING_ | _PENDING_ | _PENDING_ | _PENDING_ |
| ud-fi-pud-test (1000 / 13,474) | _PENDING_ | _PENDING_ | _PENDING_ | _PENDING_ | _PENDING_ |
| ud-fi-tdt-test (1554 / 17,951) | _PENDING_ | _PENDING_ | _PENDING_ | _PENDING_ | _PENDING_ |
| _Curated ET sets:_ | | | | | |
| et-grammar (50 / 178) | 88.6 | 96.2 | 19.6 | 51.4 | 98.9 |
| et-manual (4 / 12) | 77.8 | 77.8 | 0.0 | 11.1 | 91.7 |

**Net effect vs 2026-05-07j** (custom parser):

- **Curated FI/ET: zero movement.** Full%, Lemma, POS, Grammar, Coverage all identical to j on all six curated datasets. The FEATS migration's `Match.Full` change doesn't fire when gold has no FEATS, and FST is off in both j and k.
- **UD FI: Full% drops** because UD gold supplies full FEATS and parser supplies none (FST off; no Ekilex bulk drop). Lemma/POS/Grammar/Coverage hold (same dict + case-suffix path). _Filled in once UD pass completes._

**Convention note**: this is the first baseline tagged with a wall-clock timestamp suffix (`-T0944Z`). The letter (`k`) is the within-stack ordinal that follows `j`; the `THHMMZ` suffix lets multiple baselines on the same parser-behavior-version stay distinct in `docs/baselines/` and on disk. See [`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md) for the full convention.

**Open issues this surfaced**:

- **FST production tables not on this machine.** `localdata/lemmatizer-fi-et/tables/` is empty here, so the FST step is off across the entire baseline. To make the FST contribution measurable we either (a) regenerate locally via `make gen-lemmatizer-tables-fi VFST_PATH=…` against an upstream Voikko `mor.vfst`, or (b) ship a deterministic regeneration recipe that's runnable from a fresh clone. Tracked since §2026-05-07j; still open.
- **Ekilex bulk drop missing on this DB.** ET FEATS via Ekilex `morph_code` (commit [`2febc31`](https://github.com/sagarinbabel/finnestdb/commit/2febc31)) requires the ~6.18M form rows from `make import-ekilex-details-et`. This DB is at the kaikki + Ekilex public-headwords state (392k forms). The projected ~95% ET grammar lift documented in [`LEARNINGS.md`](LEARNINGS.md) needs that import replayed before it's measurable.
- **Curated gold doesn't have FEATS.** The FEATS migration's main accuracy mechanism only fires on FEATS-bearing gold. Curated FI/ET sets pre-date FEATS-aware annotation, so the migration's impact is invisible on them. Backfilling FEATS into the curated gold would let us see per-attribute FEATS scoring on those small high-signal sets too — a small JSON-edit job, gated on whether the curated sets need to keep tracking the analyzer ceiling closely. (**Resolved** by §2026-05-07k-feats-rich above.)

---

### 2026-05-07j — Post-FST re-measure (case-suffix grammar-label stopgap + smoke FST tables) — pre-FEATS-migration baseline

**Commit measured**: [`42e95d9`][c-2026-05-07j] (= `main` head at 2026-05-07T11:22Z, the moment this measurement was taken; subsequent commits landed during/after the run — see "Post-baseline movement" below)
**Detail**: [`baselines/2026-05-07-post-fst-fi.md`](baselines/2026-05-07-post-fst-fi.md), [`baselines/2026-05-07-post-fst-et.md`](baselines/2026-05-07-post-fst-et.md), [`PARSER_EVAL_METHODOLOGY.md`](PARSER_EVAL_METHODOLOGY.md)
**Parser version stamp**: `2026.05.07j` (`parsecore.ParserVersion`)

Re-runs the full FI + ET eval suite on `main` after PR [#109](https://github.com/sagarinbabel/finnestdb/pull/109) (case-suffix grammar-label stopgap) and the artifact-policy migration ([`8d75dbf`](https://github.com/sagarinbabel/finnestdb/commit/8d75dbf), PR [#125](https://github.com/sagarinbabel/finnestdb/pull/125)) landed on top of the FST stack. The 2026-05-06i FINAL snapshots restored in [#124](https://github.com/sagarinbabel/finnestdb/pull/124) were taken on the maintainer's PR4 branch where the embedded `fi_min.json` / `et_min.json` were locally regenerated against full Voikko / Giellalt analysers; this re-measure captures the state a fresh clone of `main` actually exhibits, where those tables are the 12-form smoke fixtures the artifact policy intends.

**Post-baseline movement (informational; not measured here):** between this measurement and the PR that lands this entry, five PRs merged to `main` ([#127](https://github.com/sagarinbabel/finnestdb/pull/127) FST step promotion → parallel scorer in dict step 1; [#128](https://github.com/sagarinbabel/finnestdb/pull/128) lemmatizer tables → `testdata/lemmatizer/` + `localdata/lemmatizer-fi-et/tables/`; [#129](https://github.com/sagarinbabel/finnestdb/pull/129) full FST candidate merge; [#130](https://github.com/sagarinbabel/finnestdb/pull/130) FEATS migration — gate `Match.Full` on FEATS, attach FEATS-only custom morphology, thread per-attribute FEATS through eval and report; [#131](https://github.com/sagarinbabel/finnestdb/pull/131) data consolidation under `localdata/`). The numbers below therefore freeze the **pre-FEATS-migration / pre-FST-step-promotion** state — useful as the "before" reference for the FEATS migration's measured impact. A follow-up `2026-05-07k` entry should re-run these datasets on the post-FEATS code path.

**Headline numbers** (custom parser):

| Dataset (cases / tokens) | Lemma | POS | Grammar | Full | Coverage |
|---|---:|---:|---:|---:|---:|
| _Curated FI sets:_ | | | | | |
| fi-grammar (80 / 156) | 96.8 | 98.1 | **59.5** | **79.5** | 99.7 |
| fi-core (6 / 23) | 85.0 | 90.0 | **30.0** | **50.0** | 95.7 |
| fi-manual-v1 (22 / 187) | 81.4 | 85.7 | 6.7 | 64.3 | 91.2 |
| fi-manual-v2 (4 / 12) | 88.9 | 100.0 | **33.3** | 66.7 | 100.0 |
| _UD FI test sets (real-world):_ | | | | | |
| ud-fi-ftb-test (1867 / 13,973) | 71.4 | 66.4 | 22.4 | 41.1 | 92.5 |
| ud-fi-ood-test (2106 / 16,151) | 62.5 | 65.6 | 23.0 | 39.3 | 85.0 |
| ud-fi-pud-test (1000 / 13,474) | 60.0 | 66.0 | 19.7 | 34.7 | 85.5 |
| ud-fi-tdt-test (1554 / 17,951) | 60.2 | 67.8 | 22.2 | 36.3 | 89.6 |
| _Curated ET sets:_ | | | | | |
| et-grammar (50 / 178) | 88.6 | 96.2 | **19.6** | **51.4** | 98.9 |
| et-manual (4 / 12) | 77.8 | 77.8 | 0.0 | 11.1 | 91.7 |

**Total evaluated:** 8 FI datasets / 61,927 FI tokens; 2 ET datasets / 190 ET tokens. **First baseline that exercises the parser at scale on real-world text** — the prior FINAL baseline only measured 378 FI tokens across 4 curated sets. The curated-vs-UD lemma gap (97% → 60%) is the LEARNINGS finding ([§2026-05-07](LEARNINGS.md)) reproduced and now committed under continuous measurement.

External-analyzer reference columns (omorfi for FI, estnltk/Vabamorf for ET) recorded in the JSON reports per the comparison-script default policy — see [`baselines/2026-05-07-post-fst-fi.md`](baselines/2026-05-07-post-fst-fi.md) for the full side-by-side gap table. omorfi 0.9.12 now ships with `pyhfst` (pure Python) rather than the C `hfst` library, so `make setup-nlp` works on macOS arm64 without HFST C builds; this re-measure used the bundled adapter against `~/.cache/omorfi/omorfi.analyse.hfst` (25 MB tarball).

**Headline gap to omorfi (FI custom vs. omorfi, percentage-point lag — positive = custom behind):**

| Dataset | Δ Lemma | Δ POS | Δ Grammar | Δ Full | Δ Coverage |
|---|---:|---:|---:|---:|---:|
| fi-grammar (curated) | −1.3 | −1.9 | +40.5 | +42.3 | +0.3 |
| ud-fi-ftb-test (UD) | **+11.9** | **+8.6** | **+60.6** | **+28.8** | +1.9 |
| ud-fi-ood-test (UD) | +6.0 | +5.1 | +54.5 | +22.3 | +0.6 |
| ud-fi-pud-test (UD) | +8.1 | +6.0 | +57.3 | +27.3 | +0.1 |
| ud-fi-tdt-test (UD) | +9.0 | +7.8 | +58.7 | +27.0 | +0.3 |

**Changes since 2026-05-06i** (commits reachable from HEAD that affect the eval; in chronological order):

- [`2febc31`](https://github.com/sagarinbabel/finnestdb/commit/2febc31) — `ET FEATS via Ekilex morph_code + LEARNINGS doc`. ET form rows now carry FEATS derived from Ekilex `morph_code` (SgN, SgG, PlAd, IndPrSg1, …). Effect on this re-measure is muted because the DB used here lacks the bulk Ekilex import (see open issues); the latent mechanism still lands when that import runs.
- [`da37ae9`](https://github.com/sagarinbabel/finnestdb/commit/da37ae9) — `Eval parity + grammar-label stopgap + suffix-table freeze (#109)`. **Largest single mechanism behind the grammar-accuracy lift in this entry.** After dict Step 1 resolves a (lemma, POS), the case-suffix matcher runs additively to attach a `GrammarLabel`, only when the suffix-strip independently arrives at the same lemma. New helper `attachCaseLabelIfStemMatches` in `internal/store/dict.go`. Bilingual: applies for FI and ET via `parserules.{Finnish,Estonian}CaseSuffixes`. The same PR also froze the suffix table — see DECISIONS.md Decision 5 — because new morphology work goes through `pkg/lemmatizer-fi-et/` (FST), not by extending the suffix list further.
- [`80e5946`](https://github.com/sagarinbabel/finnestdb/commit/80e5946) — `dict: rank VFST analyses against original surface case`. Hardens VFST disambiguation so PROPN homonyms don't beat common-noun lemmas on lowercase surfaces inside the Step 5 FST path. Effect on this re-measure is small because the smoke `fi_min.json` covers ~12 forms; the mechanism is what stops PROPN/NOUN homonyms from regressing once the local table is regenerated against full Voikko `mor.vfst`.
- [`8d75dbf`](https://github.com/sagarinbabel/finnestdb/commit/8d75dbf) — `data: migrate third-party artifacts to localdata/ + tighten artifact policy` (PR [#125](https://github.com/sagarinbabel/finnestdb/pull/125)). Codifies what PR4 already documented: upstream transducer blobs and any `localdata/` corpora artifacts are out-of-tree; the runtime-embedded `pkg/lemmatizer-fi-et/tables/{fi,et}_min.json` files stay smoke fixtures, regenerated locally via `make gen-lemmatizer-tables-fi VFST_PATH=…`.
- [`8b4fd9f`](https://github.com/sagarinbabel/finnestdb/commit/8b4fd9f) / [`6cf0610`](https://github.com/sagarinbabel/finnestdb/commit/6cf0610) — `docs: restore staged FST eval snapshots + per-stage attribution entries` and follow-up bootstrap-path fix (PR [#124](https://github.com/sagarinbabel/finnestdb/pull/124)). Restored the FINAL snapshot files into `docs/baselines/` and the per-stage entries 2026-05-06f/g/h/i in this evolution doc.

**Net effect vs 2026-05-06i** (custom parser):

- **FI grammar accuracy lifts (driven by [`da37ae9`](https://github.com/sagarinbabel/finnestdb/commit/da37ae9)'s case-suffix label stopgap; reproducible from public code):**
  - `fi-grammar`: 1.4 → **59.5** (+58.1pp), full 51.9 → **79.5** (+27.6pp)
  - `fi-core`: 0.0 → **30.0** (+30.0pp), full 35.0 → **50.0** (+15.0pp)
  - `fi-manual-v2`: 0.0 → **33.3** (+33.3pp), full 55.6 → 66.7 (+11.1pp)
- **ET grammar accuracy lift (same mechanism, ET wired):**
  - `et-grammar`: 2.0 → **19.6** (+17.6pp), full 42.9 → **51.4** (+8.5pp)
- **Lemma/coverage drops on FST-driven datasets (smoke tables vs. maintainer-local full tables):**
  - `fi-manual-v1`: lemma 82.9 → 81.4 (−1.5pp), coverage 96.9 → 91.2 (−5.7pp)
  - `fi-core`: coverage 100.0 → 95.7 (−4.3pp)
  - `et-manual`: lemma 88.9 → 77.8 (−11.1pp), grammar 16.7 → 0.0 (−16.7pp), coverage 100.0 → 91.7 (−8.3pp)
  - `et-grammar`: coverage 100.0 → 98.9 (−1.1pp)
  - These drops are not parser-code regressions — they reflect that on a fresh clone the embedded FST tables cover ~12 forms by design. To reproduce 2026-05-06i numbers on `main`, regenerate `fi_min.json` / `et_min.json` locally against full upstream analysers per the [artifact policy](ARTIFACT_POLICY.md).
- **Lemma/POS holds on dict-saturated datasets** where the case-suffix stopgap adds grammar labels but doesn't change the resolved lemma: `fi-grammar` (lemma 96.8 unchanged from PR 0.5 dict-only), `fi-core` lemma 85.0 unchanged, `fi-manual-v2` 88.9 unchanged, `et-grammar` 88.6 unchanged.
- **Note**: `fi-manual-v1` grammar dropped 13.3 → 6.7. The stopgap fires only when both the case-suffix matcher resolves the surface AND it agrees on the lemma; on real-world Finnish (v1, 22 cases) the suffix matcher was less reliable than what the maintainer-local full FST table emitted at 2026-05-06i. The FST runtime is the long-term answer; the stopgap is explicitly transitional (suffix table is frozen — see DECISIONS.md Decision 5).

**Open issues this surfaced**:

- The 2026-05-06i FINAL snapshots are non-reproducible from a fresh clone — they require the maintainer's local `fi_min.json` / `et_min.json` regeneration via `make gen-lemmatizer-tables-fi VFST_PATH=…` against full upstream Voikko `mor.vfst` / Giellalt `analyser-gt-desc.hfstol` (paths kept under `localdata/`, gitignored). This re-measure makes the public-repo state legible. A follow-up could either (a) document a deterministic local-table regeneration recipe (exact upstream analyser version + wordlist) so anyone can land the FINAL numbers on their own machine, or (b) ship the runtime so it can `mmap` an out-of-tree `mor.vfst` / `analyser-gt-desc.hfstol` at startup — the latter would close the public/private numbers gap entirely without changing the artifact policy.
- ~~The `parser-comparison.sh` script slugifies dataset names from the JSON `dataset.name` field, so `fi-manual-v1.json` and `fi-manual-v2.json` (both with `dataset.name == "fi-manual"`) overwrite each other under `${RUN_TS}-fi-manual.json`.~~ Resolved 2026-05-07: both `scripts/parser-comparison.sh` and `scripts/parser-comparison-et.sh` now slug from the input file basename, so v1/v2 land in distinct files.
- The DB used for this measurement does not contain the Ekilex bulk drop ([#78](https://github.com/sagarinbabel/finnestdb/pull/78)) — only kaikki + the Ekilex public-headwords snapshot. ET counts: 392,863 forms / 186,798 lemmas. The 2026-05-06i FINAL was measured against a DB with ~6.18M ET forms. The grammar-accuracy effect of [`2febc31`](https://github.com/sagarinbabel/finnestdb/commit/2febc31) (ET FEATS via Ekilex `morph_code`) is therefore muted in this baseline; for the full ET FEATS lift the bulk import has to be replayed (`make fetch-ekilex && make reduce-ekilex && make import-ekilex-details-et`).
- ~~The `omorfi` adapter dispatch did not auto-discover the repo-local NLP venv.~~ Resolved 2026-05-07: `runExternalOmorfi` now lives in `internal/evalparsers` and discovers `.venv/bin/python`; `make setup-nlp` creates the shared repo-local venv because system Python on macOS Homebrew enforces PEP 668, so the prior system-pip path failed there.

---

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

- Per-dataset numbers held identical to PR3 head on every dataset PR3 measured. Cleanup commits between PR3 head and PR4 merge did not move metrics — verified file-by-file against `2026-05-06-post-pr3-*.json.gz` and `2026-05-06-post-pr2-hfstol-fi-*.json.gz`.
- **First FST-stack measurement of `fi-manual-v1` (22 cases): lemma 81.4 → 82.9 (+1.4pp), grammar 0.0 → 13.3 (+13.3pp), coverage 91.2 → 96.9 (+5.7pp).** This is the FST stack's largest accuracy lift on a non-trivial Finnish set — the v2 (4-case) set used at PR1/PR2 didn't have headroom.

**Open issues this surfaced**:

- Grammar accuracy still low across all sets (1.4–16.7%). The FST analysers produce UD FEATS but `internal/store/dict.go` doesn't yet consume Number/Tense/Mood/Person from FST output beyond the Voikko grammar-label heuristic. Tracked as the FEATS-migration follow-up (was [#118](https://github.com/sagarinbabel/finnestdb/pull/118), to be cherry-picked onto the cleaned base).
- Tracked tables `pkg/lemmatizer-fi-et/tables/{fi_min.json,fi_wordlist.txt,et_min.json}` are derived data and should move under `localdata/` per the new artifact policy. Pending the artifact-policy cleanup PR. _(Resolved: tables migrated to `localdata/lemmatizer-fi-et/tables/` (gitignored); test fixtures relocated to `testdata/lemmatizer/`; seed wordlist moved to `cmd/genlemmatizertables/wordlists/fi_smoke.txt`. Runtime now disk-loads via `lemmatizer.New()` / `NewFromDir`.)_

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

Phase 3 of [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md) is shipped. The Kotus Nykysuomen sanalista 2024 (CC BY 4.0, ~104k FI headwords) lives under `localdata/kotus/` (gitignored per `docs/ARTIFACT_POLICY.md` after the 2026-05-07 single-folder bootstrap migration) and `cmd/importkotus` populates `paradigm_class` on the FI lemmas table. That's the join key the Phase 4 Voikko adapter needs.

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
**Detail**: [`baselines/2026-05-05-et-grammar-estnltk.json.gz`](baselines/2026-05-05-et-grammar-estnltk.json.gz), [`baselines/2026-05-05-et-manual-estnltk.json.gz`](baselines/2026-05-05-et-manual-estnltk.json.gz)

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
