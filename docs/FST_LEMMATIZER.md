# Generated-table lemmatizer

This document describes the Finnish + Estonian lemmatizer package after
the artifact-policy cleanup. The runtime does **not** ship upstream
transducer blobs, and as of the
`pkg/lemmatizer-fi-et/tables/*` → `localdata/` migration it does not
ship the derived JSON tables either. Tables live under
`localdata/lemmatizer-fi-et/tables/` (gitignored) and are loaded from
disk on `New()`.

See also:

- [docs/ARTIFACT_POLICY.md](ARTIFACT_POLICY.md) for the repository policy.
- [docs/FST_LEMMATIZER_ROADMAP.md](FST_LEMMATIZER_ROADMAP.md) for the
  historical migration plan and what changed.
- [experiments/2026-05-06-phase3.5-voikko-generator-spike.md](../experiments/2026-05-06-phase3.5-voikko-generator-spike.md)
  for the generator spike that led to this package.

## Current status

[`pkg/lemmatizer-fi-et/`](../pkg/lemmatizer-fi-et/) provides:

- Disk-based loading of generated JSON analysis tables. `New()` resolves
  the directory from `LEMMATIZER_TABLES_DIR` (env), falling back to
  `localdata/lemmatizer-fi-et/tables/`. Per-language coverage is
  independent: a deployment that has only `fi_min.json` works for FI
  and returns no analyses for ET.
- `NewFromDir(dir string)` - explicit-path constructor used by tests
  (against [`testdata/lemmatizer/`](../testdata/lemmatizer/)) and by
  callers that don't want to touch env.
- Offline reader/parser packages for upstream analyser formats:
  - `vfst/` for local `mor.vfst` files.
  - `hfstol/` for local HFST optimised-lookup files.
- Mapping packages that normalize native analyser tags into the parser's
  `Analysis` shape:
  - `voikkomap/`
  - `giellaltmap/`
- [`cmd/genlemmatizertables`](../cmd/genlemmatizertables/), a FI/ET table
  generator that reads a local `mor.vfst` for FI or `.hfstol` analyser for
  ET plus a wordlist and writes generated JSON to
  `localdata/lemmatizer-fi-et/tables/`. The current FI Make target uses
  a DB-derived local wordlist; the ET target still uses a small smoke
  wordlist until a production ET wordlist is chosen.

Test fixtures under [`testdata/lemmatizer/`](../testdata/lemmatizer/)
are hand-authored and intentionally tiny - they cover the words used
by `pkg/lemmatizer-fi-et/lemmatizer_test.go` and nothing else. They
are **not** production tables; they are the unit-test ground truth.

## Artifact policy

Allowed in git:

- Generator code and tests.
- Hand-authored seed wordlists for the generator (e.g.
  [`cmd/genlemmatizertables/wordlists/fi_smoke.txt`](../cmd/genlemmatizertables/wordlists/fi_smoke.txt)).
- Hand-authored unit-test fixtures under
  [`testdata/lemmatizer/`](../testdata/lemmatizer/).
- Upstream license and attribution text under
  `pkg/lemmatizer-fi-et/data/{fi,et}/` (no transducer blobs).

Not allowed in git:

- `mor.vfst`, `analyser-gt-desc.hfstol`, `.hfst` files, or any other
  upstream analyser blob that directly ships the transducer.
- The factual JSON tables generated from those analysers
  (`{fi,et}_min.json` and any future production tables) - these belong
  under `localdata/lemmatizer-fi-et/tables/`, gitignored.

Maintainers may keep upstream analysers locally, for example under
`localdata/` or via an absolute path passed to a generator command. The
generated table is a runtime asset that is regenerated rather than
reviewed.

## Runtime architecture

```
parser query
  |
  v
internal/store.BatchLookupForms()
  |
  |-- existing SQLite dictionary and fallback rules
  |
  `-- pkg/lemmatizer-fi-et
        |
        |-- New() reads localdata/lemmatizer-fi-et/tables/fi_min.json
        |-- New() reads localdata/lemmatizer-fi-et/tables/et_min.json
        `-- return []Analysis for exact surface-form matches
```

The runtime path is deliberately simple:

1. `lemmatizer.New()` (or `NewFromDir`) reads each language's table from
   disk. Missing files degrade the language to "no analyses"; both
   files missing is a hard error.
2. `Lemmatize("FI", word)` returns entries from the FI table.
3. `Lemmatize("ET", word)` returns entries from the ET table.
4. Unknown languages or unknown words return no analyses.

If `New()` fails (for example, on a fresh clone where setup-local.sh
hasn't generated tables yet), `internal/store` logs a single warning
and falls back to the dict + case-suffix path. No transducer blob is
opened at runtime.

### Store-level candidate merge

`internal/store.BatchLookupForms` uses generated FST tables in two
places when `parserMode == "custom"`. Both are preceded by a small
Step 0 short-circuit that catches the highest-frequency analyser
traps before any general-purpose ranking runs.

0. **Lexical-overlay short-circuit (PR #183).** Before the dict
   lookup, the store checks `pkg/lemmatizer-fi-et/lexadverbs` for the
   surface. The overlay catalogues forms whose productive analysis is
   a known bug - Finnish `tuskin`/`varsin`/`yleensä` (closed-class
   adverbs the parser keeps unfolding as productive case forms),
   Finnish `vuotta`/`siitä`/`muuta` (kaikki-imported with bad
   lemmas), and Estonian `peale`/`jaoks`/`seal`/`välja` (closed-class
   adpositions/adverbs read as productive case). The ET table also
   covers high-frequency closed-class learner traps such as `ei`,
   `ma`, and `sina`, where raw dictionary alternatives include nominal,
   abbreviation, or source-language-only rows. When the overlay hits,
   the curated analysis is returned outright and Steps 1 and 5 are
   skipped. Source tag `lex-overlay` lets eval reports attribute hits
   to this layer. The overlay is custom-mode-only: basic-mode baselines
   stay stable.
1. **Direct dictionary hits.** The store now treats dictionary rows and
   generated-table FST analyses as one candidate set for the surface
   form. Candidates are keyed by `(lemma, POS)`.
2. **Fallback misses.** If dictionary, possessive, compound, and
   suffix-strip paths all miss, the FST table can still provide a
   standalone resolution.

The direct-hit merge is deliberately conservative:

- When dictionary and FST agree on `(lemma, POS)`, the dictionary
  candidate remains the resolution and the FST analysis enriches it
  with `Feats` and the legacy `GrammarLabel` projection.
- When dictionary and FST disagree, the dictionary candidate normally
  wins. A disagreeing FST candidate can win only when the dictionary row
  is weak legacy data (no source priority and no morphology) and the FST
  has stronger case/POS/morphology evidence. A dictionary row with any
  FEATS or projected grammar label is treated as morphologically
  authoritative for now, even if the FST analysis is richer.
- If local FST tables are missing, behavior degrades to the dictionary
  path plus the existing case-suffix label stopgap.

#### ET source-backed learner guards (2026.05.12c)

The ET dictionary path has two narrow guards for Sõnaveeb/Ekilex-backed
data that is valid as source data but misleading as a learner-primary
parse row:

- Special-capitalized lemmas such as `mA` and `MA` require an exact
  bare-surface match in both `basic` and `custom` direct-dictionary
  lookup. Lowercase `ma` and sentence-initial `Ma` resolve to the
  pronoun, while exact `mA`/`MA` can still reach their source dictionary
  entries. Exact all-caps forms such as `TA` bypass lowercase lexical
  overlays for the same reason.
- Nominal case-only FEATS are cleared from invariant closed-class exact
  rows (`ADV`, `ADP`, `CCONJ`, `SCONJ`, `INTJ`, `PART`, `X`). This
  prevents duplicate Ekilex morphology rows from displaying genitive or
  illative labels on words such as `ei` and `kui`.
- Exact ET verb dictionary forms whose source FEATS contain
  `Case=Ill` and `VerbForm=Sup` display as `VerbForm=Inf`, so entries
  such as `olema` do not show learner-facing case labels. The check is
  attribute-based rather than tied to importer key order.
- Known ET source-language-only alternatives are filtered by exact
  `(surface, lemma, POS)`, for example `kui/NOUN`, so stale nominal
  FEATS cannot outrank useful closed-class readings.

The runtime guard protects already-built SQLite dictionaries. Future
Ekilex imports also handle this at source: `ID` form rows keep empty
FEATS, and same-key `SgN` rows can replace earlier stale case
duplicates with nominative FEATS for bare dictionary forms.

#### MA-infinitive bias (PR #183)

Finnish MA-infinitive surfaces (-maan/-mään, -massa/-mässä,
-masta/-mästä, -malla/-mällä, -matta/-mättä with a matching
`Case=` attribute) trigger a ranking bias in
`pickBestResolutionCandidate`. Kaikki ships many of these surfaces
keyed under derived nouns (`tarjoamaan → tarjoama/NOUN/Case=Ill`)
with stale FEATS (`Person=3|Number=Sing` instead of
`InfForm=Ma|VerbForm=Inf`), which the productive merge would
otherwise promote over the verb reading. The bias does two things:

- Demotes any candidate whose lemma ends in `-ma`/`-mä` on a MA-
  infinitive surface, regardless of POS. Lemma shape is a more
  reliable signal of "noun-cousin trap" than POS, because kaikki
  occasionally tags the derived noun as VERB.
- Promotes any candidate whose POS is VERB with `InfForm=Ma`.

In parallel, `udfeats.NormalizeMaInfinitive` runs on both FST output
and dict candidates (via `formResolutionFromCandidate`), stripping
the noun-cousin `Person=3|Number=Sing` signature and asserting
`Case=X|InfForm=Ma|VerbForm=Inf|Voice=Act`. `Voice=Act` is added
only when no Voice attribute is present (explicit `Voice=Pass` is
preserved). This means the merged resolution carries the right
FEATS regardless of which source supplied the lemma.

A documented residual: when the FST tables ship no MA-infinitive
surface entries at all (the current state - the production tables
have verb headwords but not their MA-infinitive inflections), the
dict-only candidate is the only option and the verb-lemma
reconstruction (e.g. `tarjoamaan → tarjota`) is impossible at
runtime. POS/FEATS/gloss come out right; only the lemma stays
wrong. Closing this needs FST table regeneration with MA-infinitive
forms included.

#### Bad-lemma filter (PR #183)

`lookupFormCandidates` filters dict candidates through a two-tier
blocklist before merge:

- **`alwaysBadDictLemmasFI`** - lemmas that are never legitimate
  standalone words. Short fragments (`as`, `taa`, `ku`),
  compound-clip prefixes (`sisä-`, `ylä-`), and documented
  kaikki-import bugs (`poli` for `poliisi` inflected forms).
  Filtered regardless of surface - no learner asks for the bare
  surface `sisä-` expecting that prefix as the lemma.
- **`badSurfaceLemmaFI`** - (surface, lemma) pairs where the lemma
  is legitimate elsewhere but wrong for the specific trap surface.
  `(varsin, varsi)`, `(vuotta, vuo)`, `(siitä, siittää)`, etc.
  The bare-lemma lookup `varsi → varsi/NOUN` is preserved; only the
  trap surface is filtered.

Seeded from `yle_subs/card_overrides/bad_lemmas.tsv` +
`SUSPICIOUS_SURFACE_LEMMAS`. When all candidates for a surface get
filtered, the surface falls through to Steps 2-5 (possessive strip,
compound split, case-suffix strip, FST). The lex-overlay above
covers the common trap surfaces directly.

FST morphology is projected into UD-style FEATS before it leaves the
store. For example, `GrammarLabel=inessive` plus `Number=Sing` becomes
`Case=Ine|Number=Sing`; verb analyses can carry
`Mood=Ind|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act`. The
legacy `GrammarLabel` field remains for older grammar-label metrics,
and is back-projected from `Case=` when possible.

### Voikko Voice extraction

Voice on Voikko verbs comes from the `[P*]` (person) tag, not from a
dedicated voice tag - Finnish passive is grammatically the "4th
person":

| Voikko `[P*]` | UD Person | UD Voice |
|---|---|---|
| `[P1]`, `[P2]`, `[P3]` | 1 / 2 / 3 | Act |
| `[P4]` | (empty - P4 is not a UD Person) | Pass |

Passive participles set Voice independently: `[Rt]` (TU-participle,
passive past) and `[Ra]` (TAVA-participle, passive present) emit
`Voice=Pass` via `applyParticiple`. Active participles (`[Rv]`,
`[Ru]`, `[Rm]`, `[Re]`) leave Voice unset; `[R*]` always clears
finite-only fields (Mood, Tense, Person) so a participle never
composes contradictory FEATS like `Tense=Past|VerbForm=Part` - UD
encodes the past/present participle distinction in `PartForm=`, not
`Tense=`.

The `[E*]` tags (connegative: `[Ef]`=false, `[Et]`=true, `[Eb]`=both)
are documented in the Voikko source but not projected to UD here.
The runtime gets `Connegative=Yes` from the orthogonal `[Cn]` signal
in `applyComparison` instead.

The composition logic is centralised in
[`pkg/lemmatizer-fi-et/udfeats`](../pkg/lemmatizer-fi-et/udfeats/udfeats.go),
which owns the `LegacyLabelToUDCase` / `UDCaseToLegacyLabel` maps and the
`Compose(...)` / `ComposeMap(...)` functions. Both `voikkomap.Parse`
and `giellaltmap.Parse` build a UD FEATS string from their structured
fields at parse time and persist the result on `Analysis.Feats` -
voikkomap uses its local `composeFeats` (which delegates to
`udfeats.ComposeMap` for the canonical alphabetical ordering),
giellaltmap calls `udfeats.Compose` directly. As of `2026.05.07k`
the smoke FST tables under `testdata/lemmatizer/{fi,et}_min.json`
include this `Feats` field on every analysis; the runtime composer in
`internal/store::featsFromFSTAnalysis` prefers the persisted value and
falls back to recomposing on the fly for legacy table files (so older
`localdata/lemmatizer-fi-et/tables/` snapshots stay loadable without
regenerating).

## Offline generation

Generated-table values use canonical alphabetical UD FEATS ordering
(enforced by `udfeats.Compose`), so all producers emit the same string
shape for the same morphology.

The Finnish generator:

```sh
make gen-lemmatizer-tables-fi VFST_PATH=/absolute/path/to/mor.vfst
```

That target writes two gitignored runtime artifacts:

- `localdata/lemmatizer-fi-et/wordlists/fi.txt`
- `localdata/lemmatizer-fi-et/tables/fi_min.json`

If the FI wordlist is missing, the target first derives it from
`finnestdb.db`:

```sh
make gen-lemmatizer-wordlist-fi
```

Then it runs the table generator:

```sh
mkdir -p localdata/lemmatizer-fi-et/tables
go run ./cmd/genlemmatizertables \
  -lang fi \
  -vfst "$VFST_PATH" \
  -wordlist localdata/lemmatizer-fi-et/wordlists/fi.txt \
  -out localdata/lemmatizer-fi-et/tables/fi_min.json
```

`VFST_PATH` must point to a local Voikko `mor.vfst`. The analyzer file,
generated wordlist, and generated JSON table must not be committed.

`scripts/setup-local.sh` invokes the FI generator best-effort - if
`VFST_PATH` is set, it generates; otherwise it skips with a warning
and the FST step in custom-mode parsing is disabled until tables exist.

The Estonian generator:

```sh
make gen-lemmatizer-tables-et
```

That target runs:

```sh
mkdir -p localdata/lemmatizer-fi-et/tables
go run ./cmd/genlemmatizertables \
  -lang et \
  -hfstol "$HFSTOL_PATH" \
  -wordlist cmd/genlemmatizertables/wordlists/et_smoke.txt \
  -out localdata/lemmatizer-fi-et/tables/et_min.json
```

By default, `HFSTOL_PATH` is
`localdata/lemmatizer-fi-et/analyser-gt-desc.hfstol`. Override it only
when `make doctor` reports a noncanonical local copy, for example:

```sh
make gen-lemmatizer-tables-et HFSTOL_PATH=/absolute/path/to/analyser-gt-desc.hfstol
```

See [`docs/LOCAL_TOOLING.md`](LOCAL_TOOLING.md) before assuming the
analyzer is absent. The analyzer file itself must not be committed. The
current ET Make target still uses a smoke wordlist; production ET
promotion still needs a production wordlist, provenance notes, row
counts, and an eval gate before any accuracy claim is made from those
local ET tables.

## What the test fixtures prove

The fixtures under [`testdata/lemmatizer/`](../testdata/lemmatizer/)
prove that:

- The runtime can consume generated factual analyses without shipped
  transducer blobs.
- FI and ET can share one runtime package and `Analysis` shape.
- The runtime stays deterministic and testable on a hermetic file set.
- Future production ET tables can be reviewed as plain generated data.

They do **not** prove broad runtime coverage, grammar-label gains, or
final eval deltas. Any accuracy or coverage claim must be generated
from the exact production tables under `localdata/` on the branch
making the claim, with the generator command and upstream version
recorded alongside.

## Production-table promotion checklist

Before promoting this package as a production replacement for the older
dictionary/rule path:

1. Confirm the production input word list for each language.
2. Generate FI and ET factual tables from local upstream analysers
   into `localdata/lemmatizer-fi-et/tables/`.
3. Record the generator command, upstream source/version, and table
   row counts in `docs/PARSER_EVOLUTION.md`.
4. Re-run parser eval against those local tables.
5. Update `docs/baselines/` only with results from runs that loaded
   production tables.

Until ET has a production wordlist and current eval, describe ET table
coverage as smoke/local-only. Describe FI table coverage by the exact
local `fi_min.json` that `make doctor` reports, not by a committed
artifact.

## Upstream attribution

The offline generation path may use:

- **libvoikko / voikko-fi** for Finnish VFST analyses.
- **HFST** for optimised-lookup tooling and format reference.
- **GiellaLT lang-fin / lang-est-x-utee** for Finnish and Estonian
  morphological resources.

License text and provenance notes live under
`pkg/lemmatizer-fi-et/data/{fi,et}/`. They are retained for auditability
because those upstream projects may be used during local table
generation, even though their transducer blobs are not committed.

## Follow-up work

- Store FI table provenance in machine-readable metadata next to the JSON
  table, including the DB snapshot used to derive
  `localdata/lemmatizer-fi-et/wordlists/fi.txt`.
- Promote the ET generator path from smoke wordlists to a production wordlist
  with provenance, row counts, and fresh eval.
- Rebaseline parser eval only after production generated tables land.
