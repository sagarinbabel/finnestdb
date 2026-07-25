# Parser Evaluation Datasets

This document describes how to build the first manual golden datasets for
parser evaluation.

## Start Now

Start creating the golden dataset now.

Do **not** wait for the third parser to be fully integrated. The dataset is
what lets you tell whether the third parser is actually better, worse, or just
different.

The recommended sequence is:

1. Create a small Finnish gold set immediately
2. Use it to compare `basic` vs `custom`
3. Integrate the third parser (`omorfi`) behind the same evaluator
4. Expand the gold set with cases where parser 3 disagrees with the first two

## First Dataset Size

For the first useful manual dataset, target:

- Finnish: `30-50` short texts
- Estonian: `15-25` short texts later
- annotated target tokens: `150-300` total to start

That is enough to expose recurring failures without turning annotation into a
huge upfront project.

## What To Collect

Use short public or openly licensed texts that resemble the reading material
you care about.

Good starting buckets:

- `10` news-like texts
- `10` encyclopedia/expository texts
- `10` conversational or general prose texts

For Finnish, prioritize texts containing:

- possessive forms
- case-rich noun phrases
- compounds
- sentence-initial capitalization
- common verb inflections
- ambiguous short forms

The goal is not random coverage. The goal is **failure-relevant** coverage.

## Suggested Source Strategy

Use material you have clear rights to store internally for evaluation.

Good source categories:

- openly licensed corpora
- Wikipedia / Wiktionary-derived example text where reuse terms are clear
- public-domain or explicitly reusable government text
- UD/treebank example sentences
- Tatoeba-style example corpora if the license fits your use
- publicly available news text only if you confirm you are comfortable using it for internal eval

Practical collection target:

- `20-30` Finnish texts from open-license or clearly reusable sources
- `10-20` Finnish texts from current real-world prose you care about, if you are comfortable using them internally
- later, the same pattern for Estonian

## Annotation Strategy

Do **not** annotate every token at first.

Annotate only the tokens that matter for parser comparison:

- inflected nouns
- inflected verbs
- compounds
- possessive forms
- words likely to trigger parser disagreement

For each target token, record:

- `surface`
- `occurrence` if repeated in the same text
- expected `lemma`
- expected `pos`
- expected `grammar_label` if that distinction matters

The current dataset format already supports that shape.

## Annotation Workflow

Recommended workflow:

1. Copy a short text into a dataset case
2. Run `basic` and `custom`
3. Look for tokens where they differ or look suspicious
4. Annotate those target tokens manually
5. Add a few control tokens that both parsers should obviously get right
6. Re-run the evaluator

This is faster than trying to annotate perfect full-token gold from scratch.

## How To Expand Once Parser 3 Exists

When `omorfi` is connected, prioritize these categories:

1. cases where `omorfi` fails and `basic` + `custom` both pass
2. cases where `omorfi` is right and the existing parsers are wrong
3. cases where `omorfi` is right but too slow
4. cases where an override rule changes `omorfi` behavior

Those are the highest-value examples because they directly guide rule tuning.

## Review Cadence

Use this cadence:

- initial seed dataset: `150-300` annotated tokens
- after third parser integration: add `20-40` new disagreement-driven tokens per iteration
- once the rule layer stabilizes: grow toward `500+` annotated tokens

## Recommended Next Manual Task

Do this next:

1. Collect `30` short Finnish texts
2. Build a first dataset with `80-120` annotated target tokens
3. Bias those annotations toward compounds, possessives, and case endings
4. Run `basic` vs `custom`
5. Keep adding only disagreement-heavy examples until the third parser lands


## Regression Fixtures vs. Coverage Fixtures

A regression fixture is a gold set whose purpose is to detect when a
previously-fixed bug comes back. It is small, hand-picked, and every
case is provably real - backed by a specific bug report, code
comment, or downstream incident.

Examples currently committed:

- [`testdata/parser-eval/fi/gold/fi-analyzer-traps-v1.json`](../testdata/parser-eval/fi/gold/fi-analyzer-traps-v1.json)
  - 20 Finnish cases seeded from `yle_subs/card_overrides/bad_lemmas.tsv`
  and `SUSPICIOUS_SURFACE_LEMMAS`. Every entry is a kaikki/Vabamorf
  analyser failure the downstream deck builder already had to patch
  with a manual override. Lex-overlay surfaces (`tuskin`, `varsin`,
  `vuotta`, `siitä`, `muuta`, ...), MA-infinitive noun-cousin traps
  (`tarjoamaan`, `lähtemään`, `juomassa`, `Tekemällä`), and bare-lemma
  surfaces where kaikki shipped a bad lemma (`Asia`, `Poliisi`).
- [`testdata/parser-eval/et/gold/et-analyzer-traps-v1.json`](../testdata/parser-eval/et/gold/et-analyzer-traps-v1.json)
  - 11 Estonian cases. Closed-class ADV/ADP forms read as productive
  cases: `välja` (read as `väli`/illative), `seal` (as
  `siga`/adessive), `peale` (as `pea`/allative), `jaoks` (as
  `jagu`/translative), `lihtsalt` (as `lihtne`/ablative), and others.

These files use the same JSON schema as the coverage fixtures
(`fi-manual-v1`, `et-grammar-v1`, the UD-derived sets, etc.) and
are picked up automatically by
[`scripts/parser-comparison.sh`](../scripts/parser-comparison.sh)
and
[`scripts/parser-comparison-et.sh`](../scripts/parser-comparison-et.sh)
via the `*.json` glob over `testdata/parser-eval/<lang>/gold/`. The
versioned `-v1` suffix follows the same convention as the other
gold sets - bump to `-v2` when adding new cases would break a
landed baseline.

When a fix lands that addresses one of these traps, the case stays
in the regression fixture forever. The point is to catch the bug
the second time, not just the first.
