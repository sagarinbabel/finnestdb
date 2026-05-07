# `data/et/`

Licensing stubs / provenance notes for upstream Estonian morphology sources.

## Policy

This repository does **not** vendor or embed transducer blobs (e.g.
`.hfstol`, `.hfst`) in-tree. We ship **generated factual tables** derived
offline from upstream analysers instead (see `pkg/lemmatizer-fi-et/tables/`
and `docs/ARTIFACT_POLICY.md`).

The licenses in this directory are retained as references for the
upstream sources used when generating those tables.

## Upstream: GiellaLT lang-est-x-utee + HFST optimised-lookup

GiellaLT Estonian morphology analysers are often produced as HFST
optimised-lookup artifacts (e.g. `analyser-gt-desc.hfstol`), compiled from
[`giellalt/lang-est-x-utee`](https://github.com/giellalt/lang-est-x-utee).

HFST tooling/runtime and many GiellaLT resources are distributed under
GPL-family licenses; those transducer blobs must not be committed here.
Instead, use them offline to generate factual tables.

## A note on the repo name

The Estonian Giellalt morphology lives at `lang-est-x-utee` (not `lang-est`,
which doesn't exist). The `-x-utee` suffix follows IETF BCP-47 private-use
subtag conventions; it indicates the University of Tartu's variant of the
Estonian morphology, which is the de facto Estonian FST in the Giellalt
ecosystem.
