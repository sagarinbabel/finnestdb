# `data/fi/`

Licensing stubs / provenance notes for upstream Finnish morphology sources.

## Policy

This repository does **not** vendor or embed transducer blobs (e.g.
`.vfst`, `.hfstol`, `.hfst`) in-tree. We ship **generated factual tables**
derived offline from upstream analysers instead (see
`pkg/lemmatizer-fi-et/tables/` and `docs/ARTIFACT_POLICY.md`).

The licenses in this directory are retained as references for the
upstream sources used when generating those tables.

## Upstream: libvoikko Finnish morphology

The Voikko Finnish morphology transducer is typically distributed as
`mor.vfst`, compiled from the [voikko-fi](https://github.com/voikko/voikko-fi)
lexc/twolc sources, tri-licensed under MPL 1.1 / GPLv2+ / LGPLv2.1+.
See `LICENSE-libvoikko.txt` for the full license text.

We may use a locally-installed `mor.vfst` during *offline* table
generation, but the blob itself should not be committed to the repo.

## Upstream: GiellaLT lang-fin + HFST optimised-lookup

GiellaLT Finnish morphology analysers are often produced as HFST
optimised-lookup artifacts (e.g. `analyser-gt-desc.hfstol`), compiled from
[`giellalt/lang-fin`](https://github.com/giellalt/lang-fin).

HFST tooling/runtime and many GiellaLT resources are distributed under
GPL-family licenses; those transducer blobs must not be committed here.
Instead, use them offline to generate factual tables.
