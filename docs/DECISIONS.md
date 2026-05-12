# Decisions Log

_Reverse-chronological — newest decisions at the top. See [CHANGELOG.md](CHANGELOG.md) for revisions._

> **Note:** DECISIONS.md and CHANGELOG.md overlap by design — CHANGELOG records
> *what* changed; DECISIONS records *why* we chose to change it that way.
> Where a decision and a changelog entry describe the same event, both files
> cross-link.
>
> The consumer-alpha execution plan in [`../TODO.md`](../TODO.md) and the
> product framing in [`FEATURES.md`](FEATURES.md) take precedence over older
> decisions recorded here where they conflict. Older entries remain for
> historical context.
>
> The project roadmap formerly tracked in this file (Tracks A/B/C) has moved
> to TODO.md's "What's in main" / "What's not in main yet" sections. The
> Track A/B/C breakdown is preserved at the bottom of this file as historical
> record only.

This document tracks key architectural decisions and the reasoning behind
them. It serves as a journal of how the project evolves and why we made the
choices we did.

---

## Product Vision

FinEstDB is a **JPDB clone for Finnish and Estonian**. The core user flow:

1. **Paste text** — User submits Finnish (or Estonian) text
2. **Parse perfectly** — System extracts lemmas, POS, definitions
3. **Review word list** — User sees all unique words with meanings
4. **Mark known/unknown** — User indicates which words they already know
5. **Create deck** — User saves unknown words as a study deck
6. **SRS study** — Spaced repetition moves words from "learning" to "known"
7. **Loop** — Next time user pastes text, known words are dimmed, focus is on new vocabulary

The value proposition: **pre-mine vocabulary before reading** so the reading experience
is more enjoyable and comprehension is higher.

---

## Decision 19: Filter low-value dict alternatives in deck/parse expansion

**Date:** 2026-05-12

### Context

`BatchLookupAllForms` returns every `(lemma, pos)` candidate the dictionary
has for a surface form. Wiktionary-imported form-of rows (e.g.
`olen → "first-person singular present indicative of olema"`) live alongside
the base lemma (`olema → "be"`) and, until PR #185, both produced their own
card or word-list entry during deck/parse expansion. Some surfaces also had
candidates with empty glosses — `liiga/X` next to `liiga/ADV → "too"` — which
similarly bloated the deck with rows the learner can't act on.

### Decision

When a surface has multiple dict candidates and at least one has a non-empty
gloss, suppress:

1. candidates with empty glosses, and
2. Wiktionary form-of alternatives, when a lexical-base alternative exists for
   the same surface.

Form-of detection is structural, not substring-based:

- `candidate.Lemma == form` (case-insensitive, trimmed) — Wiktionary form-of
  rows are imported with the surface form as their own lemma.
- Gloss contains no `;` or `,` — form-of glosses are single-clause.
- Gloss parses as `<allowed morphology terms> of <single-word target>` after
  normalizing `-` and `/` to spaces. The allowed vocabulary covers
  case names, person/number, tense/mood/voice, infinitive/participle/gerund,
  comparative/superlative degree, connegative and potential moods, and the
  bare `form` / `inflection` markers.

When no lexical alternative exists for a surface, all candidates are preserved
— genuine unresolved / gap cases still surface to the learner.

### Reasoning

A v1 marker-substring heuristic produced false positives on common lexical
glosses whose body text happens to mention grammatical terms:
`vana/ADJ "old; ancient; ...; out of order; ...; past; ..."`,
`oma/ADJ "(my/...) own; ...; one of a kind; ...; singular; ..."`,
`mennä/VERB "to go [with illative of third infinitive ...]"`. The structural
signals are language-independent and robust: `form == lemma` identifies
Wiktionary form-of-as-lemma rows directly, and the `;`/`,` rejection rules
out multi-sense lexical glosses without inspecting their body.

The filter operates on `BatchLookupAllForms` output before deck-ingest and
before the parse-overview word list, so the unique-lemma count of an import
overview still matches the count of the deck the user would save.

### Source

PR [#185](https://github.com/sagarinbabel/finnestdb/pull/185).

**See also:** [CHANGELOG.md §2026-05-12 — Deck/parse low-value dict-alternative
filter (PR #185)](CHANGELOG.md).

---

## Decision 18: IMPLEMENTATION.md split

**Date:** 2026-05-07

### Context

`docs/IMPLEMENTATION.md` overlapped substantially with
[`ARCHITECTURE.md`](../ARCHITECTURE.md) and was hard to keep in sync. Updates
landed in one file but not the other; readers couldn't tell which doc was
canonical for a given topic.

### Decision

`docs/IMPLEMENTATION.md` becomes a redirect stub. Unique content moved to
its canonical home:

- "Suggest fix" UX → new
  [`docs/PARSER_FEEDBACK_LOOP.md`](PARSER_FEEDBACK_LOOP.md).
- Build/tooling → [`README.md`](../README.md) "Build & Start" section.
- Current limitations → [`README.md`](../README.md) "Known Limitations"
  section.

### Reasoning

Minimize cross-doc drift. One canonical home per topic, with stubs that
redirect rather than mirror.

### Source

PR #135.

**See also:** [CHANGELOG.md §2026-05-07 — Runtime docs parity pass](CHANGELOG.md).

---

## Decision 17: ESTONIAN_LEXICAL_PLAN consolidation

**Date:** 2026-05-07

### Context

We had two lexical-layer plan documents — `docs/LEXICAL_PLAN.md` (FI) and
`docs/ESTONIAN_LEXICAL_PLAN.md` (ET) — and they had drifted independently.
The ET plan still recommended a smoke import path (`make
import-dict-et-ekilex`) that the bulk Ekilex pipeline had superseded.

### Decision

Consolidate into one [`docs/LEXICAL_PLAN.md`](LEXICAL_PLAN.md). ET-specific
source choices and the EstNLTK adapter contract now live in a section
("Estonian-specific source choices and adapter contract") inside that file.
`docs/ESTONIAN_LEXICAL_PLAN.md` is deleted.

### Reasoning

Duplicate plans rot independently. One canonical doc per language pair —
shared architecture in the body, language-specific deltas in clearly
labelled sections.

### Source

PR #135.

**See also:** [CHANGELOG.md §2026-05-06 — Lexical pipelines](CHANGELOG.md)
for the original two-doc landing.

---

## Decision 16: FST as parallel scorer in dict step 1

**Date:** 2026-05-07

### Context

Pre-PR-#127, the FST runtime in
[`pkg/lemmatizer-fi-et/`](../pkg/lemmatizer-fi-et/) was wired in as a
step-5 fallback only — it fired on dict miss. This made morphology a
fallback rather than an evidence source: the parser couldn't surface FEATS
for forms whose lemma resolved cleanly through the dict.

### Decision

The FST contributes candidates in parallel with dict step 1
([PR #127](https://github.com/sagarinbabel/finnestdb/pull/127)).
`FormResolution.Feats` is enriched from FST candidate merge
([PR #129](https://github.com/sagarinbabel/finnestdb/pull/129)).
Per-attribute FEATS evaluation is added so regressions on individual
morphological attributes are visible
([PR #130](https://github.com/sagarinbabel/finnestdb/pull/130)).

### Reasoning

Dict-only resolution can't surface FEATS for forms whose lemma resolves
cleanly through dict — the lemma is right but the morphology bucket is
empty. Parallel scoring + candidate merge gives FEATS coverage on dict
hits without sacrificing dict's lemma accuracy. Per-attribute eval
prevents silent regressions on individual features (Case, Number, Person,
Tense) hiding behind a stable composite "grammar accuracy" number.

### How to Revisit

If the smoke-fixture FST starts dragging dict accuracy down (e.g. emits
wrong-case homonyms that win the merged ranking), re-litigate the merge
order. The current ranking gives dict the lemma vote and FST the FEATS
vote; that asymmetry is intentional but tunable.

---

## Decision 15: Single-folder bootstrap rule

**Date:** 2026-05-07

### Context

Legacy `data/ud-cache/` and gitignored carve-outs under
`testdata/parser-eval/` made bootstrap require multiple archives or a
custom recipe that knew the carve-outs. Handing a teammate a "fast
bootstrap" zip was a foot-gun.

### Decision

Every gitignored runtime artifact lives under
[`localdata/`](../localdata/). The `data/` directory is disallowed for new
artifacts; `.gitignore` carries belt-and-braces guards for legacy paths.
`localdata/` covers UD cache, parser-eval gold/train carve-outs, frequency
baselines, lemmatizer tables, Kotus distribution, Ekilex artifacts, etc.

### Reasoning

One tarball captures the entire bootstrap state:

```bash
tar czf finnestdb-bootstrap.tgz localdata/ finnestdb.db
```

No carve-outs to remember. `make setup-local` summarizes the tree on
completion and emits the bootstrap-tar instruction.

### Source

PR #131. See [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) "Single-folder
bootstrap rule" section for the full policy and rationale.

**See also:** [CHANGELOG.md §2026-05-07 — Single-folder data root](CHANGELOG.md).

---

## Decision 14: `feats` column not backfilled on existing kaikki.org rows — REVERSED 2026-05-07k

**Date:** 2026-05-06 · **Reversed:** 2026-05-07 (PR #139)

### Context

When the `feats` column was added to `forms` (Phase 1 schema delta), an
implicit question was whether to retroactively populate it on existing
kaikki.org-sourced rows.

### Original decision (2026-05-06)

Leave `feats=NULL` on existing kaikki.org form rows. Voikko-generated /
FST-derived rows fill in features at higher priority going forward.

### Original reasoning

kaikki.org form rows don't carry UD-style FEATS in the source data, and
mining them post-hoc from the surface/lemma pair is exactly what the FST
does at runtime. Backfilling synthetically would create a second,
lower-quality FEATS source competing with the FST output. Cleaner to
keep kaikki.org rows feature-less and let higher-priority sources
contribute FEATS.

### Reversal (2026-05-07k)

The original reasoning was wrong: kaikki form rows DO carry the
morphology, just not in UD-FEATS shape. Each `forms[]` entry has a
`Tags []string` field with exactly the lowercase English vocabulary
that maps deterministically to UD FEATS — `["illative","singular"]`
→ `Case=Ill|Number=Sing`. PR [#139](https://github.com/sagarinbabel/finnestdb/pull/139) implements this projection in
`cmd/importdict/feats.go::kaikkiTagsToFeats`, covering Case (15 entries),
Number, Person, Tense, Mood, Voice, VerbForm, Degree, Reflex (lexical-static),
PronType. The translation is lossless — we're not synthesising FEATS we
can't defend; we're reading the same morphological annotation from a
different field of the same source.

The "competing FEATS source" concern is also resolved by the precedence
already in place: dict-layer FEATS yield to FST-layer FEATS via
`enrichResolutionWithFST` when the FST has a richer analysis, and
`featsFromFSTAnalysis` always wins when both fire. So kaikki-derived
FEATS act as the floor (better than NULL) without competing with the
FST ceiling.

---

## Decision 13: Wikisanakirja for monolingual FI definitions; Kielitoimiston deferred

**Date:** 2026-05-06

### Context

Finnish has two practical sources for monolingual definitions:
Kielitoimiston sanakirja (authoritative, restricted redistribution) and
Wikisanakirja (Wiktionary's Finnish edition, openly licensed).

### Decision

Use Wikisanakirja (via kaikki.org's Finnish edition extract) for
monolingual FI definitions. Kielitoimiston is **not** in scope for alpha.

### Reasoning

Kielitoimiston's redistribution restrictions make bulk import infeasible.
Wikisanakirja coverage is sufficient for alpha; kaikki.org already extracts
and normalizes the Finnish-edition Wiktionary data. Revisit Kielitoimiston
only if Wikisanakirja proves insufficient and only as a runtime lookup
that respects the license.

### How to Revisit

Track Wikisanakirja coverage gaps. If a meaningful fraction of high-frequency
lemmas come back without definitions, add Kielitoimiston as a runtime
(non-redistributed) lookup with explicit license-compliant attribution.

---

## Decision 12: Idempotent ALTER TABLE migration pattern; real framework deferred

**Date:** 2026-05-06

### Context

We need to add columns and tables as the lexical schema evolves. A real
migration framework (numbered SQL files, version table) is the textbook
answer, but it's also a one-shot up-front investment.

### Decision

Schema migrations stay on the established codebase pattern: each migration
is an idempotent `ALTER TABLE ... ADD COLUMN` (or `CREATE TABLE IF NOT
EXISTS`) that tolerates the SQLite "duplicate column name" error on
re-run. Grouped backfills get exported helpers named `EnsureXxx` in
[`internal/store/db.go`](../internal/store/db.go), called by both the
server's `ensureSchema` and any standalone importer that needs the same
shape. No `PRAGMA user_version`. References:
`EnsureDictionarySourceColumns` (#67), `EnsureLexicalEnrichmentColumns`
(Phase 1).

A real migration framework is deferred until at least one of these is
true: a non-additive migration is needed (column rename, stateful
backfill); >5 versioned migrations and merge conflicts start; rollback
support is required.

### Reasoning

The codebase already established the idempotent-ALTER convention.
Introducing a parallel mechanism (PRAGMA user_version, numbered SQL
files) before it's needed would just give us two patterns to maintain.
Migrations are infrequent and append-only today.

### How to Revisit

When the trigger conditions hit (non-additive migration, merge-conflict
pressure, rollback need), introduce the real framework as a single PR —
not lazily alongside a feature.

---

## Decision 11: Kotus distribution as authoritative lemma source

**Date:** 2026-05-06

### Context

There are two practical paths for Kotus class data: the official Kotus
sanalista distribution (https://kaino.kotus.fi/sanat/nykysuomi/) and
Voikko's `joukahainen` re-export.

### Decision

Use the official Kotus distribution. The 2024 distribution is fetched into
the gitignored `localdata/kotus/` via `make setup-local` (CC BY 4.0; see
[`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md)) and imported via
`cmd/importkotus`.

### Reasoning

The official distribution is more likely to be maintained and is the
canonical authority for Kotus class assignments. Voikko's re-export adds
a layer of indirection without adding signal.

---

## Decision 10: Adapter packaging — separate cmd/ binaries per rich source

**Date:** 2026-05-06

### Context

PRs #67 and #68 added Ekilex import via a `-source-key` flag on the
existing `cmd/importdict/` binary. Subsequent rich sources (Kotus, Voikko)
needed a packaging decision.

### Decision

Each rich source gets its own `cmd/import<source>/` (or
`cmd/gen<artifact>/`) binary, matching the precedent set when
`cmd/importekilex/` landed on main:

- `cmd/importkotus/` — Kotus sanalista TSV
- `cmd/genlemmatizertables/` — generated FST tables (FI smoke today)
- `cmd/importekilex/` — Ekilex public-headword snapshot
- `cmd/importekilexdetails/` — bulk Ekilex reduced JSONL
- `cmd/importdict/` — kaikki.org / Wiktionary (the original)

### Reasoning

Each binary handles its own input shape (XML, TSV, JSONL, generated
lookups) and shares only the schema bootstrap pattern. Keeping
`cmd/importdict/` as a multi-source dispatcher would mix unrelated input
parsers in one entry point and dilute its responsibility.

### Note

PRs #67 and #68 use the `-source-key` flag inside `cmd/importdict/` —
that work predates `cmd/importekilex/` landing on main and will likely
rebase against the separate-binary pattern.

---

## Decision 9: Production morphology tables deferred; smoke fixtures only

**Date:** 2026-05-06

### Context

The lemmatizer-fi-et package needs morphology tables to function. Generating
production-scale FI/ET tables requires running upstream analysers
(VFST/HFST) locally and committing the output. Smoke fixtures
(9 FI keys, 7 ET keys) are enough to prove the integration path.

### Decision

The current committed FI/ET tables under
[`pkg/lemmatizer-fi-et/tables/`](../pkg/lemmatizer-fi-et/tables/) and
[`testdata/lemmatizer/`](../testdata/lemmatizer/) are smoke fixtures only.
They prove the integration and the artifact policy, not production
morphology coverage. Production tables are generated locally via
`make gen-lemmatizer-tables-fi VFST_PATH=...` and written to
`localdata/lemmatizer-fi-et/tables/` (gitignored). Broad
runtime/eval claims wait until production tables, provenance, and fresh
baselines all land together in a single PR.

### Reasoning

Committing production tables prematurely would mean shipping artifacts
without provenance and gating eval claims on data nobody else can
reproduce. Holding the smoke fixtures separate from production tables
makes the boundary explicit.

### How to Revisit

A production FI/ET table PR adds a production word list, provenance,
generator command, row counts, and fresh eval baselines as a single unit.

---

## Decision 8: Translations and definitions tables ship before Sõnaveeb

**Date:** 2026-05-06

### Context

Both the FI and ET lexical plans need separate `translations` and
`definitions` tables (rather than overloading `lemmas.gloss`). The question
was whether to land them with the FI plan, or hold for Sõnaveeb integration.

### Decision

Land `translations` and `definitions` tables with the FI Phase 1 schema
delta. Both languages benefit; landing them once avoids two parallel
migrations.

### Reasoning

The Finnish plan needs translations and definitions; the Estonian plan
benefits from them. Sequential landings would mean either ET ships
without these tables and migrates later, or FI waits on ET — both worse
than landing once.

---

## Decision 7: Generated-table deployment policy

**Date:** 2026-05-06

### Context

The generated-table runtime (`pkg/lemmatizer-fi-et/`) loads JSON tables
generated from upstream analysers (Voikko VFST, Giellalt HFST). The
question is what gets committed to git: just the runtime code, the
generated tables, or also the upstream analyser blobs.

### Decision

The build/generation pipeline may run local upstream analysers, but the
repository ships **neither** analyser blobs **nor** the derived factual
tables. Both live under `localdata/lemmatizer-fi-et/` (gitignored).
The runtime loads tables from disk on `New()`. Smoke fixtures (small
hand-checked tables) live in
[`testdata/lemmatizer/`](../testdata/lemmatizer/) and
[`pkg/lemmatizer-fi-et/tables/`](../pkg/lemmatizer-fi-et/tables/) — those
exist purely to prove the integration path and are 9–12 entries each.

See [`docs/ARTIFACT_POLICY.md`](ARTIFACT_POLICY.md) for the full policy.

### Reasoning

Upstream analyser blobs are large, license-constrained, and are
reproducibly generated from public source. Committing them would bloat
the repo and create license-compatibility questions. Generated tables
have the same problem at smaller scale: regenerable from upstream
analysers, not human-readable, and license-derivative.

The runtime-loads-from-disk choice means a fresh checkout has a working
package (using smoke fixtures) and a production-quality install requires
running the generator locally and pointing the runtime at
`localdata/lemmatizer-fi-et/tables/`.

**See also:** [CHANGELOG.md §2026-05-06 — Lexical pipelines](CHANGELOG.md).

---

## Decision 6: Numeric-Hyphen Tokenization Lives in the Shared Tokenizer

**Date:** 2026-05-06

### Context

A user pasted Estonian text containing `65-aastane` ("65-year-old") into the
parser during manual testing and noticed neither `65` nor `aastane` showed up
as separate words. Pure numbers like `65` weren't tagged `NUM` either. The
same construction is just as productive in Finnish (`65-vuotias`,
`1990-luvulla`, etc.), and the tokenizer at
[`parser/src/lib.rs:308`](../parser/src/lib.rs:308) takes an unused `_lang`
parameter — Finnish was guaranteed to have the identical bug.

### Decision

Fix this in the shared Rust tokenizer with four pure-tokenizer rules. Do **not**
add per-language entries to
[`internal/parserules/finnish.go`](../internal/parserules/finnish.go) or
[`internal/parserules/estonian.go`](../internal/parserules/estonian.go).

- **R1** — split a chunk at the first hyphen where one side is pure digits and
  the other starts with a letter (`65-aastane`, `65-vuotias`, `1990-luvulla`).
  Skip mixed-prefix abbreviations (`B1-tase`, `well-known`).
- **R2** — `guess_pos` returns `NUM` for `^\d+$`, `^\d+\.\d+$`,
  `^\d+,\d+$`, with internal whitespace stripped.
- **R3** — post-pass that merges `\d{1,3}( \d{3})+` runs into one NUM token.
  Form keeps spaces (`"250 000"`); lemma drops them (`"250000"`) so SI-spaced
  and unspaced numbers group as one entry in the words list.
- **R4** — split a chunk at the only hyphen if both sides are pure digits
  (`1990-2020`). Multi-hyphen forms (ISO dates `2026-05-06`) stay whole because
  R4 requires exactly one hyphen.

### Reasoning

[`docs/CROSS_LANGUAGE_STRATEGY.md`](CROSS_LANGUAGE_STRATEGY.md) lists
*tokenization or sentence-split error* and *compound segmentation error* as
shared error categories, and prescribes investing in shared infrastructure
when one language surfaces a problem the other has too. This is exactly that
case: the four rules are language-agnostic (digits, letters, hyphens, spaces
are universals across FI/ET) and any per-language inflection of the freed
stems (`aastane`, `vuotias`, `luku`) is handled by the existing language-
specific lookup chain. Splitting the tokenizer fix into two per-language
implementations would have been duplicate work with a high risk of drift.

### Trade-off Accepted

- Conservative R4 (one-hyphen-only) leaves ISO dates whole as a single
  unresolved NOUN literal, which is the same as the pre-fix state for those
  forms (no regression). A dedicated date detector can layer on later.
- ET `65-aastast` (partitive of `65-aastane`) splits cleanly via R1 but
  `aastast` doesn't lemmatize back to `aastane` without a `-ne` ADJ
  inflection table — separate piece of work.
- Negative numbers like `-5` stay as one token. Acceptable for alpha;
  negation is usually written `miinus 5` in both languages.

### How We Measure Success

- 13 new Rust unit tests added; full suite: 41 passed, 0 failed.
- Zero regression on all 6 existing gold datasets (et-grammar-v1,
  et-manual-v1, fi-core-v1, fi-manual-v1, fi-manual-v2, fi-grammar-v1) at the
  Phase-2 baseline DB. Numbers identical to
  [2026-05-06b](baselines/2026-05-06b-summary.md).
- A direct probe of 13 sentences across both languages (5 FI + 5 ET fix
  cases + 3 regression cases) confirms R1–R4 produce the intended tokens
  and freed stems (`aastane`, `luku`) hit the dictionary cleanly. Full
  trace in
  [`docs/qa-reports/2026-05-06-numeric-hyphen-tokenization.md`](qa-reports/2026-05-06-numeric-hyphen-tokenization.md).

**See also:** [CHANGELOG.md §2026-05-06 — Numeric-hyphen tokenization](CHANGELOG.md).

---

## Decision 5: Don't Extend the Case-Suffix Table; Generated Morphology Tables Are the Real Answer

**Date:** 2026-05-06

### Context

`internal/parserules/{finnish,estonian}.go` defines suffix→case-label tables
(15 Finnish entries, 17 Estonian entries) used by
`internal/store/dict.go::tryCaseSuffixStrip`. The matcher strips a suffix off
the surface form, looks up the residual stem in the `lemmas` table, and on
hit returns `(lemma, pos, grammar_label)`.

The natural-feeling reaction to the 0% grammar-accuracy result on
`grammar_label` (see `docs/baselines/2026-05-06b-summary.md`) is to grow this
table — add more suffix entries, encode consonant gradation, handle ternary
compounds, etc. Existing TODO items #15 (three-part compound splitting) and
#16 (consonant gradation rules) point in that direction. We are choosing
**not** to.

### Decision

**Freeze the case-suffix table at its current size.** Further morphology
investment goes into generated factual morphology tables loaded by
[`pkg/lemmatizer-fi-et`](../pkg/lemmatizer-fi-et/) from
`localdata/lemmatizer-fi-et/tables/` (gitignored), plus the offline
generator/reader code that can reproduce them from local upstream
analysers. Per [docs/ARTIFACT_POLICY.md](ARTIFACT_POLICY.md), upstream
transducer blobs and the derived tables are both local-only and are
not committed.

Two near-term exceptions are in scope:

1. **The stopgap label-attach pass on dict hits**
   (`attachCaseLabelIfStemMatches` in `internal/store/dict.go`). Lifts grammar
   accuracy off zero on tokens whose stem doesn't change under inflection.
   Explicitly stopgap; removed once production generated tables emit FEATS
   for direct hits. **Updated 2026-05-07k**: the stopgap path now also
   projects UD FEATS via `featsFromCaseLabel` (a small map lookup against
   `pkg/lemmatizer-fi-et/udfeats::LegacyLabelToUDCase`). The `Case=` it
   emits is the only attribute it can safely commit to from a stripped
   suffix; Number/Tense/Mood/Person stay empty. The suffix table itself
   is unchanged — the addition is a projection on top, not an extension.
2. **Bug fixes** to existing entries if a wrong label is being attached.

### Reasoning

Suffix-stripping is the wrong *shape* of operation for Finnish/Estonian
morphology. Five reasons, each grounded in real tokens from our gold sets:

1. **Stem alternation can't be expressed by end-of-string rules.**
   `toas → tuba` (et-grammar-v1, inessive of "room"): suffix-strip removes
   `s`, leaves `toa`, but the lemma is `tuba` — `o ↔ u` flips inside the
   stem with consonant alternation. A suffix table operates only on the
   suffix; it has no way to encode "after stripping `s`, also rewrite
   `oa → uba` for stems in grade-alternation class III." Encoding that is
   reimplementing an FST with worse abstractions. Same with `Naabri →
   naaber` (epenthetic vowel), `linnas → linn` (final-`a` deletion after
   `s`-stripping), `majja → maja` (gemination + `a`-insertion produced
   the illative; reverse isn't a suffix strip at all).

2. **Suffix-shaped lemmas trigger false positives.** `aas` (meadow), `mees`
   (man), `loss` (castle) all end in `s`. Stripping `s` gives `aa` / `mee` /
   `los` — none of which are lemmas. The table has no way to know which
   `-s` is paradigmatic and which is part of the lemma. An FST knows
   because it has the lexicon and the inflectional paradigm together.

3. **Genuine ambiguity needs a candidate set, not a single answer.**
   `linnas` is `Case=Ine|Number=Sing` of `linn` ("in the city") AND, in
   some readings, `Case=Gen|Number=Sing` of a personal name `linnas`. A
   suffix table emits one tuple `(lemma, label)`; the alternative is
   discarded. Real morphology produces a candidate set and lets the
   disambiguator pick. `pickBestFormCandidate` already exists for direct
   dict — case-suffix-strip output should be folded into the same ranker
   in the FST world, not the suffix world.

4. **Compound interaction.** Estonian compounds extensively. Suffix-strip
   over-fires on compounds: `linnasüda` ("city heart") ending in
   suffix-shaped `a` parses as essive of a fake lemma. Compounds need to
   be split *before* suffix logic, and the split needs paradigm-class
   awareness — FST territory.

5. **We are already building a table-backed morphology path.** PRs
   [#107](https://github.com/sagarinbabel/finnestdb/pull/107),
   [#108](https://github.com/sagarinbabel/finnestdb/pull/108), and
   [#110](https://github.com/sagarinbabel/finnestdb/pull/110) add the
   generated-table runtime and offline analyser readers. The suffix table
   should remain fallback code while production tables are generated.

### Trade-off Accepted

The stopgap will not produce grammar labels for stem-alternating forms
(`toas`, `Naabri`, `linnas`-as-inessive). That's acceptable because:

- Stem-alternating forms are exactly what generated analyser-derived
  tables are meant to cover;
- The existing 15+17 entries are sufficient to lift grammar accuracy off
  zero on the easy majority case (Finnish has cleaner suffixation than
  Estonian; Estonian's harder cases were always going to need the FST).

### What This Closes

- **TODO #15** (three-part compound splitting) — DEFER to FST migration.
  The VFST handles compounds natively via concatenated `[Xp]...[X]`
  segments; see `pkg/lemmatizer-fi-et/voikkomap/` parser in PR #107.
- **TODO #16** (consonant gradation rules in suffix-strip) — REJECT.
  Gradation lives in the FST's lexicon-aware paradigm tables, not in
  string-rewrite rules over the surface.

Both items are restated in `TODO.md` under the
"FST migration supersedes" section.

### How to Revisit

If the FST migration stalls or is reversed, this decision should be
re-litigated. Until then, PRs that add suffix-table entries or implement
gradation/ternary-compound logic in `internal/parserules/` or
`internal/store/dict.go` should be redirected to
`pkg/lemmatizer-fi-et/` instead.

**See also:** [CHANGELOG.md §2026-05-06b — Eval harness parity + grammar-label stopgap](CHANGELOG.md).

---

## Decision 4: Parse Feedback Requires Login (v1)

**Date:** 2026-04-29

### Context

PR #53 introduces a parse-feedback flow: a user reviewing a parse result can flag
that the parser tagged a token incorrectly (wrong lemma/POS/grammar label) and
propose a correction. The endpoint that accepts this feedback (`POST
/api/parse/feedback`) needed an auth model.

### Decision

**Require login to submit parse feedback in v1.** No anonymous feedback path.

### Reasoning

- **Spam control.** Without an authenticated identity, the feedback queue is open
  to drive-by submissions with no rate-limit anchor and no way to hold a reporter
  accountable.
- **Admin signal-to-noise.** Admins reviewing the queue need to be able to
  weight reporters (returning user vs. one-time submitter). Anonymous breaks that.
- **Follow-up.** If a correction is ambiguous, admins need a way to ask the
  reporter for context. Anonymous feedback can't be followed up on.
- **Scope.** A "light anonymous feedback" path requires its own design (one-shot
  feedback tokens scoped to a parse session, separate rate limiting, separate
  admin UI). Out of scope for alpha.

### Trade-off Accepted

Some users will hit a parser bug, want to flag it, and won't bother creating an
account to do so — that signal is lost. We accept this for v1 in exchange for a
clean, low-noise feedback queue.

### Related Source-Text Retention Decision

The original product call here was **Option B**:

- do not persist parse sessions during `/api/parse`
- persist parse-session context only when a logged-in user explicitly submits
  feedback
- treat feedback submission as the consent boundary for storing source text

Rationale from a user perspective:

- The parse UI feels ephemeral; users will paste personal/sensitive content
  (private messages, work documents, copyrighted material) without expecting it
  to be stored.
- Storing only on feedback-submit aligns persistence with consent: the user has
  actively asked us to look at this parse, so retaining the context is
  justified.
- Eliminates unbounded growth from anonymous parse traffic.

### Amendment (2026-04-30): alpha shipped as Option A

Alpha shipped as **Option A** instead:

- authenticated `/api/parse` calls create `parse_sessions` rows
- anonymous `/api/parse` calls do **not** create `parse_sessions` rows
- anonymous parses do **not** persist source text
- parse feedback still requires login and still references a server-issued
  `parse_id`

We accepted the deviation from the original Option B decision for alpha because
it:

- solves the immediate unbounded-growth concern from anonymous parse traffic
- keeps the frontend and backend contract simpler
- preserves a clean path to a future parse-history UI for logged-in users

### Trade-off Accepted

This leaves a real privacy gap for alpha:

> Logged-in users have their pasted text stored automatically during Inspect,
> without a separate per-paste consent moment.

That gap is accepted for alpha only. It will be closed with:

- a parse-history UI
- per-user delete controls for stored parse sessions
- an opt-in ephemeral parse mode for logged-in users

### Post-v1 Reconsideration

If parser-quality work outgrows the volunteer feedback signal, revisit anonymous
"light feedback" as a separate, rate-limited path with its own queue.

**See also:** [CHANGELOG.md §2026-04-29 — Consumer alpha execution plan](CHANGELOG.md).

---

## Decision 3: Evaluation-Driven Development

**Date:** 2026-04-28

### Approach

Parser improvements are driven by **measured evaluation** against gold test cases,
not intuition.

### Infrastructure Built

- **Gold datasets:** `testdata/parser-eval/fi/gold/fi-manual-v1.json` (22 cases)
- **Eval CLI:** `cmd/parsertest` — runs parsers against datasets, outputs accuracy metrics
- **Metrics:** Lemma accuracy, POS accuracy, grammar label accuracy, coverage, timing

### Evaluation Workflow

```
1. Run eval:  go run ./cmd/parsertest -dataset fi-gold.json -parsers custom
2. Review failures: Which tokens did we get wrong?
3. Add rule: Fix the failure pattern
4. Re-run eval: Confirm improvement, no regressions
```

### Future: Automatic Improvement

**Status:** parked post-live idea. This section is historical context and
should not be treated as active roadmap work before FinEstDB is shipped and
live. Do not block parser or product changes on autoresearch behavior unless a
user explicitly asks for it.

Inspired by [karpathy/autoresearch](https://github.com/karpathy/autoresearch), we plan
to build an automated improvement loop:

```
1. Agent modifies parser rules
2. Run eval automatically
3. If accuracy improves: commit and continue
4. If accuracy drops: revert and try different approach
5. Repeat overnight → 100 experiments vs 5 manual attempts
```

This requires:
- Larger gold dataset (100+ sentences, currently 22)
- Consolidated rules in one file (clear edit scope for agent)
- Automated git workflow

**Update 2026-05-06c:** the gold dataset expanded from 22 cases to ~14k
committed FI cases via UD treebank ingestion (Plan C / PR 1) — see
[CHANGELOG.md §2026-05-06c — UD treebank gold expansion](CHANGELOG.md).

---

## Decision 2: Parser Architecture

**Date:** 2026-04-28

### Current Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Rust Parser (parser/src/lib.rs)                                │
│  - NFC normalization                                            │
│  - Sentence splitting                                           │
│  - Tokenization                                                 │
│  - Heuristic POS guessing                                       │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ FFI (JSON)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Go Parse Core (internal/parsecore/parsecore.go)                │
│  - Production parser registry (basic, custom)                   │
│  - Dictionary resolution                                        │
│  - Enrichment rules (possessive, compound, case suffix)         │
│  - Gloss lookup                                                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  SQLite Dictionary (internal/store/dict.go)                     │
│  - forms table: surface form → lemma + POS                      │
│  - lemmas table: lemma + POS → gloss                            │
│  - Source: kaikki.org (Wiktionary-derived)                      │
└─────────────────────────────────────────────────────────────────┘
```

Eval-only baselines live in `internal/evalparsers`, which registers
`omorfi`/`estnltk`, owns adapter subprocess discovery and timeouts, and feeds
the normalized FFI-shaped result through parsecore's external-analyzer result
builder. The server-facing parser registry does not expose those lab modes.

### Parser Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `basic` | Dictionary lookup only, no enrichment | Speed baseline |
| `custom` | Dictionary + possessive/compound/case rules | Production parser |
| `omorfi` | External FI adapter for comparison | Evaluation only |
| `estnltk` | External ET adapter for comparison | Evaluation only |

### Enrichment Rules (in `custom` mode)

1. **Possessive suffix stripping** (Finnish)
   - `kirjassani` → strip `-ni` → `kirjassa` → lookup → `kirja`

2. **Compound word splitting** (Finnish + Estonian)
   - `pankkiautomaatti` → `pankki` + `automaatti` → lookup both

3. **Case suffix stripping** (Finnish + Estonian)
   - `kirjassa` → strip `-ssa` → `kirja` + grammar label "inessive"

### Update 2026-05-07: FST migration shipped

The original "Future Consideration: FST" note has been superseded.
The FST migration shipped via PRs
[#106](https://github.com/sagarinbabel/finnestdb/pull/106) /
[#107](https://github.com/sagarinbabel/finnestdb/pull/107) /
[#127](https://github.com/sagarinbabel/finnestdb/pull/127) /
[#129](https://github.com/sagarinbabel/finnestdb/pull/129) /
[#130](https://github.com/sagarinbabel/finnestdb/pull/130). The FST runtime
in [`pkg/lemmatizer-fi-et/`](../pkg/lemmatizer-fi-et/) is now wired in
parallel with dict step 1 — see Decision 16 above. Production tables are
generated locally; smoke fixtures are committed (Decision 9).

---

## Decision 1: Build a Custom Parser (Not Use Omorfi Directly)

**Date:** 2026-04-28

### Context

Omorfi (Open Morphology of Finnish) is the gold standard for Finnish morphological
analysis. It uses finite-state transducers (FST) built over 15+ years of linguistic
work. The question: should we use Omorfi directly, or build our own parser?

### Decision

**Build our own custom parser**, using Omorfi as a quality benchmark.

### Reasoning

| Factor | Our Parser | Omorfi |
|--------|-----------|--------|
| **Speed** | ~40–50k words/s on FI on `custom`, single core ([baseline](baselines/2026-05-06-fi-summary.md#measured-throughput)) | FST traversal + Python subprocess startup; not yet benchmarked under our harness |
| **Deployment** | Self-contained Go binary | Requires Python + HFST + .hfst files |
| **Customization** | Add a rule in Go, redeploy | Fork FST project, recompile transducers |
| **Licensing** | Permissive (we control it) | GPL-3.0 (copyleft implications) |
| **Estonian support** | Same architecture, different rules | Would need separate tool (EstNLTK) |

### Trade-off Accepted

We accept that our parser may not match Omorfi's accuracy on edge
cases. The goal is **comparable lemma/POS accuracy with deployment
and licensing properties Omorfi can't give us** — speed is a
property we own, not the headline argument.

### How We Measure Success

- **Accuracy:** Compare lemma/POS output against gold-annotated test cases.
- **Speed:** `cmd/parsertest` reports per-case latency (avg / p50 / p95
  in ms, ns-precision under the hood) and aggregate `words/s` and
  `chars/s` per parser per dataset. Current floor on Finnish is
  ~40–50k words/s on `custom` against the 2026-05-06 dictionary state;
  treat anything below that on the same datasets as a regression to
  triage. Speed claims must always cite a measurement — comparing
  finnestdb against external baselines requires running both under the
  same harness, not eyeballing numbers from external papers.
- **Coverage:** Percentage of tokens resolved to dictionary entries.

> **Speed claims policy:** never quote a "we're faster than X" number
> in this repo without a `cmd/parsertest` run on a comparable dataset
> and a link to the JSON report. The 2026-05-06 timer fix (PR #103)
> exists because we previously couldn't.

### Omorfi's Role

Omorfi is used as:
- A **benchmark** to measure our parser's accuracy against
- A **tool for generating gold annotations** when building test cases
- Not a production runtime dependency

---

## Open Questions

_Questions are date-tagged with the date they were first recorded._

1. **(2026-04-28) FST for novel words:** At what accuracy level do we need
   FST-like morphological analysis for unseen words? Current heuristics may
   plateau at ~95%. _Partial answer 2026-05-07: FST runtime now contributes
   in parallel with dict (Decision 16); the question evolves to "production
   table coverage targets" — see LEXICAL_PLAN.md "Production generated-table
   scope" open question._

2. **(2026-04-28) Gold data source:** Should we use Omorfi to generate
   candidate annotations, then human-verify? Or fully manual annotation?
   _Partial answer 2026-05-06c: UD treebanks now provide ~14k committed
   FI cases / ~8k local-only ET cases of human-checked morphology — see
   CHANGELOG.md §2026-05-06c. Manual gold remains for targeted regression
   probes._

3. **(2026-04-28) Auto-improvement scope:** Which files should the agent be
   allowed to modify? Just rules? Or also the Rust tokenizer?

4. **(2026-05-06) Production generated-table scope:** Pick the FI and ET
   word lists, table names, row-count targets, provenance format, and
   eval gates for the first production generated-table PR.

5. **(2026-05-06) ET generation path:** Add a generator command for local
   Giellalt/HFST Estonian analyses, analogous to the current FI VFST
   smoke generator.

---

## Historical: Project Roadmap (preserved)

> **Note:** The project roadmap moved to TODO.md's "What's in main" /
> "What's not in main yet" sections. The Track A/B/C breakdown below is
> preserved as historical context for how we framed the work in
> 2026-04-28, not as the current roadmap.

### Track A: Core Product (User-Facing)

| Phase | Work | Status |
|-------|------|--------|
| A1 | Parse Experience — results table polish, coverage gauge | Largely shipped |
| A2 | Deck Creation — "Save as Deck" CTA from results | Shipped |
| A3 | Known Words — mark known/ignored in results table | Shipped |
| A4 | Navigation Shell — nav bar, dark theme alignment | Shipped |
| A5 | SRS Core — review queue, card scheduling, session UI | Shipped |
| A6 | Known Words Loop — SRS → known list → dims in future parses | Shipped |
| A7 | Import Known Words — upload CSV of already-known vocabulary | See TODO.md |

### Track B: Parser Quality (Development Infrastructure)

| Phase | Work | Status |
|-------|------|--------|
| B1 | Gold Data Expansion — 100+ annotated FI sentences | Shipped (~14k via UD) |
| B2 | Baseline Benchmark — record current accuracy/speed | Shipped |
| B3 | Rule Consolidation — all rules in one file | Shipped |
| B4 | Omorfi Comparison — side-by-side accuracy measurement | Shipped |
| B5 | Auto-Improvement Loop — autoresearch-style experiments | Parked post-live idea |

### Track C: Estonian (Parallel Path)

| Phase | Work | Status |
|-------|------|--------|
| C1 | Estonian Gold Data — expand from 1 to 50+ cases | Shipped (~8k via UD-ET) |
| C2 | Estonian Dictionary — verify kaikki.org coverage | Shipped (Ekilex bulk) |
| C3 | Estonian Rules — case suffixes, compounds | Shipped |

---

## Changelog

| Date | Change |
|------|--------|
| 2026-04-28 | Initial decisions documented: custom parser rationale, architecture, evaluation approach, roadmap (Decisions 1–3) |
| 2026-04-29 | Decision 4 added: parse feedback requires login in v1; source_text persisted only on feedback submit |
| 2026-04-30 | Recorded parse-feedback persistence amendment: alpha ships authenticated parse-session storage as Option A |
| 2026-05-06 | Decision 5 added: freeze the case-suffix table; further morphology work goes into generated morphology tables under `pkg/lemmatizer-fi-et/tables/` |
| 2026-05-06 | Decision 6 added: numeric-hyphen tokenization (R1–R4) lives in the shared Rust tokenizer, no per-language rule tables |
| 2026-05-06 | Decisions 7–14 absorbed from `LEXICAL_PLAN.md` "Locked Decisions" (generated-table deployment, translations/definitions tables, production-table deferral, adapter packaging, Kotus distribution, ALTER TABLE migrations, Wikisanakirja for FI defs, kaikki.org `feats` not backfilled) |
| 2026-05-07 | Decision 15 added: single-folder bootstrap rule — every gitignored runtime artifact lives under `localdata/` |
| 2026-05-07 | Decision 16 added: FST contributes candidates in parallel with dict step 1 (PRs #127/#129/#130) |
| 2026-05-07 | Decision 17 added: ESTONIAN_LEXICAL_PLAN consolidated into `docs/LEXICAL_PLAN.md` (PR #135) |
| 2026-05-07 | Decision 18 added: IMPLEMENTATION.md split into PARSER_FEEDBACK_LOOP.md + README sections (PR #135) |
| 2026-05-07 | Document reordered latest-first; roadmap moved to TODO.md (preserved here as historical) |
| 2026-05-07 | Decision 5 amended (PR #139): case-suffix stopgap also projects UD `Case=` into `forms.feats` via `featsFromCaseLabel`; suffix table itself stays frozen. **Decision 14 (kaikki `feats` not backfilled) reversed**: `cmd/importdict/feats.go::kaikkiTagsToFeats` now projects Wiktionary tag arrays into UD FEATS at import time, populating `forms.feats` for every kaikki row |
