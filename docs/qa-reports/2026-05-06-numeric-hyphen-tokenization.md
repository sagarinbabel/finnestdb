# Numeric-Hyphen Tokenization Fix

Date: 2026-05-06
Author: Claude (manual-test bug surfaced in chat)
Languages: Finnish, Estonian (shared tokenizer)

## Scope

This report records:

1. The bug we hit during manual testing.
2. The root cause in the shared Rust tokenizer.
3. The four-rule fix we agreed on, with examples in both languages.
4. Measured impact: existing-baseline regression check + a probe across the
   actual rule set.
5. Known gaps and follow-ups.

## How we found it

Manual parser testing on Estonian text. The user pasted a sentence that
contained `65-aastane` ("65-year-old") and noticed neither `65` nor
`aastane` showed up as separate words in the words list — `65-aastane`
was being treated as one unresolved unit. Pure numeric tokens like `65`
also weren't tagged `NUM`.

The user asked us to extend the same treatment to Finnish before any
fix, since the Rust tokenizer is shared. Confirmation:
[`parser/src/lib.rs:308`](../../parser/src/lib.rs:308) takes a `_lang`
parameter that is unused (note the underscore). All four parsers
(`basic`, `custom`, `omorfi`, `estnltk`) tokenize through the same code,
so Finnish has the identical bug for `65-vuotias`, `1990-luvulla`,
`B1-tason`, etc.

[`docs/CROSS_LANGUAGE_STRATEGY.md`](../CROSS_LANGUAGE_STRATEGY.md)
classifies this kind of fix as shared-pipeline work: the *error
taxonomy* lists "tokenization or sentence-split error" and "compound
segmentation error" as shared categories, and the *How They Improve
Together* section says shared infrastructure is the right place to
invest when one language surfaces a problem the other has too. We
followed that path: one tokenizer change ships to both languages on the
same day, with no per-language rule tables.

## Root cause

Three things in [`parser/src/lib.rs`](../../parser/src/lib.rs) combined
into the bug:

1. `is_punct` does not include the ASCII hyphen `-`, so `tokenize`
   keeps hyphenated chunks intact ("well-known" stayed together by
   design — but so did `65-aastane`).
2. `guess_pos` had no `NUM` branch, so all-digit tokens defaulted to
   `NOUN` with the digits as their lemma.
3. The whitespace-based chunker emitted `250` and `000` as separate
   tokens with nothing fusing them back into a single number.

## The fix

Four rules in `parser/src/lib.rs`. All four are tokenizer-only and apply
to both languages — no entries added to
[`internal/parserules/finnish.go`](../../internal/parserules/finnish.go)
or [`internal/parserules/estonian.go`](../../internal/parserules/estonian.go).

- **R1 — digit/letter hyphen split.** In a chunk, find the first hyphen
  where one side is pure digits and the other starts with a letter;
  split there into `digits`, `-` (PUNCT), `letters`. Triggers:
  `65-aastane`, `65-vuotias`, `1990-luvulla`. Skips: `B1-tase`,
  `well-known`.
- **R2 — `NUM` POS detection.** In `guess_pos`, return `NUM` for forms
  matching `^\d+$`, `^\d+\.\d+$`, or `^\d+,\d+$`. Whitespace inside the
  form is stripped before matching so SI thousand notation also matches.
- **R3 — thousand-space merge.** Post-pass over the token list. A
  non-punct `\d{1,3}` token followed by one or more `\d{3}` non-punct
  tokens collapses into a single token whose form preserves the spaces
  (`"250 000"`); `create_token` strips the spaces when forming the
  lemma so `"250 000"` and `"250000"` group as one entry.
- **R4 — single-hyphen digit/digit range split.** If a chunk has
  exactly one hyphen and both sides are pure digits, split into
  `NUM`, `-` (PUNCT), `NUM`. Triggers: `1990-2020`, `12-15`. ISO dates
  like `2026-05-06` have two hyphens and stay whole.

## Worked output (post-fix)

Captured by running `parsecore.Analyze(db, lang, text, "custom")` against
the Phase-2 baseline DB (`/tmp/finnestdb-baseline.db`). Format: `form
| POS | resolved? | lemma | source`.

### Estonian

`Ta on 65-aastane.`

```
Ta         NOUN   resolved   ta          dict
on         VERB   resolved   olema       dict
65         NUM    stub       65          stub
-          PUNCT             -           punct
aastane    ADJ    resolved   aastane     dict
.          PUNCT             .           punct
```

`aastane` resolves cleanly. Pre-fix, `65-aastane` was a single
unresolved NOUN with lemma `65-aastane`.

`Maja maksis 250 000 eurot.`

```
Maja       NOUN   resolved   maja        dict
maksis     NOUN   resolved   maksi       dict     (pre-existing FI/ET dict mismatch)
250 000    NUM    stub       250000      stub
eurot      NOUN   resolved   euro        dict     [partitive]
.          PUNCT             .           punct
```

`250 000` is one NUM token with lemma `250000`. Pre-fix, this produced
two separate `NOUN` stubs (`250` and `000`).

`1990. aastal sündis ta Tallinnas.`

```
1990       NUM    stub       1990        stub
.          PUNCT             .           punct
aastal     NOUN   resolved   aasta       dict     [adessive]
sündis     VERB   resolved   sündima     dict
ta         PRON   resolved   ta          dict
Tallinnas  PROPN  resolved   Tallinn     dict     [inessive]
.          PUNCT             .           punct
```

`1990` is now NUM (was NOUN pre-fix).

`Tegevus toimus aastatel 1990-2020.`

```
Tegevus    NOUN   resolved   tegevus     dict
toimus     VERB   resolved   toimuma     dict
aastatel   NOUN   resolved   aasta       dict
1990       NUM    stub       1990        stub
-          PUNCT             -           punct
2020       NUM    stub       2020        stub
.          PUNCT             .           punct
```

Year range splits at the hyphen; both sides become NUM. Pre-fix,
`1990-2020` was a single unresolved NOUN.

`B1-tase on raske.` (regression check)

```
B1-tase    NOUN   stub       b1-tase     stub
on         VERB   resolved   olema       dict
raske      NOUN   resolved   rask        dict
.          PUNCT             .           punct
```

`B1-tase` stays whole because `B1` is not pure digits — R1 skipped.

`Kohtumine toimub 2026-05-06 hommikul.` (regression check)

```
Kohtumine  NOUN   resolved   kohtumine   dict
toimub     VERB   resolved   toimuma     dict
2026-05-06 NOUN   stub       2026-05-06  stub
hommikul   NOUN   resolved   hommik      dict
.          PUNCT             .           punct
```

ISO date stays whole because R4 requires exactly one hyphen.

### Finnish

`Hän on 65-vuotias.`

```
Hän        PRON   resolved   he          dict     (pre-existing dict gloss)
on         VERB   resolved   on          dict
65         NUM    stub       65          stub
-          PUNCT             -           punct
vuotias    NOUN   stub       vuotias     stub     (* see "Pre-existing dict gaps")
.          PUNCT             .           punct
```

`65` is NUM. `vuotias` is now a separate token; the dict didn't
resolve it as ADJ in this run because the bare form is missing from
the FI form table — that is a separate Voikko/Wiktionary coverage
issue, not a tokenizer one (Phase 4 will close it).

`1990-luvulla syntyi paljon lapsia.`

```
1990       NUM    stub       1990        stub
-          PUNCT             -           punct
luvulla    NOUN   resolved   luku        dict
syntyi     VERB   resolved   syntyä      dict
paljon     ADV    resolved   paljon      dict
lapsia     NOUN   resolved   lapsi       dict
.          PUNCT             .           punct
```

The big FI win: `luvulla` resolves to `luku` ("decade") via the
existing dict lookup. Pre-fix, `1990-luvulla` was one unresolved NOUN.

`Hinta on 12,50 euroa.`

```
Hinta      NOUN   resolved   hinta       dict
on         VERB   resolved   on          dict
12,50      NUM    stub       12,50       stub
euroa      NOUN   resolved   euro        dict     [partitive]
.          PUNCT             .           punct
```

Comma-decimal NUM detection works.

`Tehdas valmistaa 1 234 567 yksikköä.`

```
Tehdas     NOUN   resolved   tehdas      dict
valmistaa  VERB   resolved   valmistaa   dict
1 234 567  NUM    stub       1234567     stub
yksikköä   NOUN   resolved   yksikkö     dict     [partitive]
.          PUNCT             .           punct
```

Three-group SI thousand notation merges into one NUM token; lemma
collapses the spaces.

`B1-tason kurssi on alkanut.` and `well-known henkilö.` (regression
checks): both stay whole, as expected.

## Measured impact on existing baselines

Ran `cmd/parsertest` against the Phase-2 baseline DB
(`/tmp/finnestdb-baseline.db`, identical to the
[2026-05-06b baseline](../baselines/2026-05-06b-summary.md) DB):

| Dataset | Cases | Parser | Lemma | POS | Coverage | Δ vs 2026-05-06b |
|---|---:|---|---:|---:|---:|---|
| et-grammar-v1 | 50 | basic | 88.6 | 94.3 | 100.0 | identical |
| et-grammar-v1 | 50 | custom | 88.6 | 94.3 | 100.0 | identical |
| et-manual-v1 | 4 | basic | 88.9 | 88.9 | 91.7 | identical |
| et-manual-v1 | 4 | custom | 88.9 | 88.9 | 100.0 | identical |
| fi-core-v1 | 6 | basic | 85.0 | 90.0 | 91.3 | identical |
| fi-core-v1 | 6 | custom | 85.0 | 90.0 | 95.7 | identical |
| fi-manual-v1 | 22 | basic | 55.7 | 78.6 | 75.3 | identical |
| fi-manual-v1 | 22 | custom | 81.4 | 85.7 | 91.2 | identical |
| fi-manual-v2 | 4 | basic | 88.9 | 100.0 | 91.7 | identical |
| fi-manual-v2 | 4 | custom | 88.9 | 100.0 | 100.0 | identical |
| fi-grammar-v1 | 80 | basic | 96.8 | 98.1 | 99.7 | identical |
| fi-grammar-v1 | 80 | custom | 96.8 | 98.1 | 99.7 | identical |

**Zero regression on all 6 existing gold datasets in both languages.**

The existing gold sets contain no numeric-hyphen sentences, so they do
not measure the new behavior. The probe above is the qualitative
measurement of the fix's effect; gold-set deltas are zero by
construction. A focused gold dataset for numeric tokens is a
natural next-up — see Follow-ups.

## Test additions

13 new Rust unit tests added to
[`parser/src/lib.rs`](../../parser/src/lib.rs) (28 → 41 total),
covering:

- R1: digit/letter splits (ET `65-aastane`, FI `65-vuotias`,
  `1990-luvulla`, reverse order `aastane-65`).
- R1 negative: `B1-tase`, `well-known`.
- R3: 2-group merge (`250 000`), 3-group merge (`1 234 567`),
  non-3-digit stops the run, non-digit head doesn't trigger.
- R4: positive (`1990-2020`), negative (`2026-05-06`).
- R2: integer, period decimal, comma decimal, spaced thousands.
- `create_token` lemma stripping for NUM.
- End-to-end `analyze_text_internal` checks for ET 65-aastane, ET
  thousand-spaced, ET year range.

`cargo test --release`: 41 passed, 0 failed.
`go test ./...`: all packages pass.

## Pre-existing dict gaps surfaced (not in scope for this fix)

The probe revealed several pre-existing dict issues that are *unmasked*
by the fix but unrelated to the tokenizer. Listing them so they don't
get conflated with this PR:

- FI `vuotias` not in form table as a bare ADJ (only inside compounds
  like `kahdeksanvuotias`). Phase 4 (Voikko paradigm computation) is
  the right place to fix.
- ET `maksis` resolving as NOUN lemma `maksi` instead of VERB lemma
  `maksma`. Source-priority / multi-lemma issue, not this PR.
- ET `raske` resolving as NOUN lemma `rask` instead of ADJ lemma
  `raske`. Same class.
- FI `Hän` glossed as English `he` (the PRON resolves correctly to
  PRON; the gloss surface is what looks odd in the trace). Dict gloss
  issue.

## Follow-ups

- Add a focused gold dataset
  (`testdata/parser-eval/{et,fi}/gold/{et,fi}-numeric-v1.json`) so the
  numeric-hyphen behavior gets a permanent eval slot. Defer to the next
  PR — the change here is intentionally tokenizer-only.
- ET `-ne` adjective inflection table so `65-aastast` (partitive of
  `65-aastane`) lemmatizes back to `aastane` after R1 splits it.
  Currently splits cleanly, but `aastast` falls to the case-suffix
  stripper which lands on `aasta` (wrong). Separate piece of work.
- Optional: digit-`-`-digit range with 1–3 digit sides (sports scores
  `2-1`, page ranges `12-15` already split — confirmed in the probe).
  No change required; just noting that R4 covers them.
