# `data/et/`

Vendored Estonian morphology data compiled by Giellalt upstream.

## `analyser-gt-desc.hfstol`

The Giellalt Estonian morphology analyser in HFST optimised-lookup
binary format. Compiled from
[`giellalt/lang-est-x-utee`](https://github.com/giellalt/lang-est-x-utee)
lexc/twolc sources via `make`, using HFST 3.17.1 (`hfst-fst2fst -O`).
Licensed under GPLv3+; see `LICENSE-hfst.txt` (HFST runtime algorithm)
and `LICENSE-giellalt-lang-est.txt` (linguistic data).

This file is read at runtime by [`pkg/lemmatizer-fi-et/hfstol`](../../hfstol)
and embedded into the binary at compile time via `//go:embed` from
[`../../lemmatizer.go`](../../lemmatizer.go).

To refresh: rebuild lang-est-x-utee against a current HFST toolchain
(see [docs/FST_LEMMATIZER_ROADMAP.md](../../../../docs/FST_LEMMATIZER_ROADMAP.md))
and copy the new `.hfstol` here.

## Provenance

```
analyser-gt-desc.hfstol:
  sha256 = fd3e5ec6179484e2a0eaf6f9b87e5285d0f5d41045478d8b971e586ba1150e17
  source: built locally from giellalt/lang-est-x-utee@HEAD
  HFST version: 3.17.1
  built: 2026-05-06
```

## A note on the repo name

The Estonian Giellalt morphology lives at `lang-est-x-utee` (not `lang-est`,
which doesn't exist). The `-x-utee` suffix follows IETF BCP-47 private-use
subtag conventions; it indicates the University of Tartu's variant of the
Estonian morphology, which is the de facto Estonian FST in the Giellalt
ecosystem.
