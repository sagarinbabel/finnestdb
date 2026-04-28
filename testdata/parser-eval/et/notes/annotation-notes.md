# Estonian Annotation Notes

Use this file while building the Estonian gold set.

## Goal

Create a sparse gold set for parser comparison.

Do not annotate every token. Annotate only the tokens that are useful for
measuring parser quality differences.

## What To Annotate

Prioritize:

- case-inflected nouns
- compounds
- verbs with potentially ambiguous lemma/POS
- stem-alternation cases
- short forms likely to confuse a parser

Annotate each target token with:

- `surface`
- `lemma`
- `pos`
- optional `grammar_label`
- optional `occurrence`

## Rules

1. Use the base dictionary lemma, not the surface form.
2. Use `occurrence` when the same token string appears more than once in a case.
3. Add `grammar_label` only when it matters for comparison.
4. Keep each text short enough that manual review is fast.
5. Prefer `1-4` annotated target tokens per case.
6. If a form is genuinely ambiguous, add a note in review discussion rather than guessing silently.

## Estonian-Specific Watchpoints

- case endings
- stem alternation
- compound boundaries
- forms where a parser may return an inflected stem instead of a lexical lemma

## Annotation Process

1. Put the raw text in `../sources/` with provenance metadata.
2. Add the candidate case to `../drafts/et-manual-seed-v0.json`.
3. Run the evaluator or parse UI and inspect parser outputs.
4. Fill in the gold lemma/POS for only the target tokens.
5. Add a `grammar_label` only if you want that distinction tested.
6. Once a case is reviewed, move it into a `gold/*.json` file.

## First Pass Target

Start with:

- `15-20` short Estonian texts
- `40-60` annotated target tokens

That is enough to expose recurring parser issues without turning this into a
large annotation project immediately.
