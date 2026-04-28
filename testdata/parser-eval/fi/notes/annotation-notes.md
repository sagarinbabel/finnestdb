# Finnish Annotation Notes

Use this file while building the first Finnish gold sets.

## Goal

Create a sparse gold set for parser comparison.

Annotate only the tokens that are useful for measuring parser quality
differences. Do not try to annotate every token in every text.

## What To Annotate

Prioritize:

- possessive forms
- compounds
- case-inflected nouns
- ambiguous verb forms
- sentence-initial capitalized words
- forms where `basic`, `custom`, and later `omorfi` are likely to disagree

Annotate each target token with:

- `surface`
- `lemma`
- `pos`
- optional `grammar_label`
- optional `occurrence`

## Rules

1. Use the base dictionary lemma, not the surface form.
2. Use `occurrence` when the same token string appears more than once in a case.
3. Add `grammar_label` only when you want that distinction tested.
4. Keep each case short enough that review stays fast.
5. Prefer `1-4` annotated target tokens per case.
6. If a token is ambiguous, do not guess silently; note the ambiguity in review discussion.

## Finnish-Specific Watchpoints

- possessive suffixes such as `-ni`, `-si`, `-mme`, `-nne`, `-nsa/-nsä`
- case endings
- compounds and possible compound boundaries
- consonant gradation cases
- forms where direct dictionary lookup fails but a human can still identify the lexical lemma

## Annotation Process

1. Put the raw text in `../sources/` with provenance metadata.
2. Add the candidate case to `../drafts/fi-manual-seed-v0.json`.
3. Run the parsers and inspect outputs.
4. Fill in the gold lemma/POS for only the target tokens.
5. Add `grammar_label` only when it matters for comparison.
6. Once a case is reviewed, move it into a `gold/*.json` file.

## First Pass Target

Start with:

- `20-30` short Finnish texts
- `80-120` annotated target tokens

That is enough to evaluate rule changes and to expose where a third parser
fails while the earlier parsers pass.
