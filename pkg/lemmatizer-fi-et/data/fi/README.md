# `data/fi/`

Vendored Finnish morphology data compiled by libvoikko upstream.

## `mor.vfst`

The Voikko unweighted finite-state transducer for Finnish morphological
analysis. Compiled from the [voikko-fi](https://github.com/voikko/voikko-fi)
lexc/twolc sources. Tri-licensed under MPL 1.1 / GPLv2+ / LGPLv2.1+; see
`LICENSE-libvoikko.txt` for the full license text.

This file is read at runtime by [`pkg/lemmatizer-fi-et/vfst`](../../vfst)
(the Go port of libvoikko's `UnweightedTransducer`) and embedded into
the finnestdb binary at compile time via `//go:embed` from
[`../../lemmatizer.go`](../../lemmatizer.go).

To refresh: copy a newer `mor.vfst` from a libvoikko release. The file
is byte-stable per upstream release; we don't regenerate it locally.

## `analyser-gt-desc.hfstol`

The Giellalt Finnish morphology analyzer in HFST optimised-lookup
binary format. Compiled from
[`giellalt/lang-fin`](https://github.com/giellalt/lang-fin) lexc/twolc
sources via `make`, using HFST 3.17.1 with `hfst-fst2fst -O` to produce
the `.hfstol` artifact. Licensed under GPLv3+; see `LICENSE-hfst.txt`
(HFST runtime algorithm) and `LICENSE-giellalt-lang-fin.txt`
(linguistic data).

This file is read at runtime by [`pkg/lemmatizer-fi-et/hfstol`](../../hfstol)
(the Go port of HFST's `Transducer::get_analyses` runtime) and embedded
into the binary at compile time via `//go:embed` from
[`../../lemmatizer.go`](../../lemmatizer.go).

To refresh: rebuild lang-fin against a current HFST toolchain (see the
Phase 3.5 spike report for the bootstrap recipe), then copy the new
`.hfstol` here.

## Provenance

```
mor.vfst:
  sha256 = 5d3bfa406e589db45a90378c53b3132fb3ef4882cd5f00846eb4e3c50d0f957d
  source: /opt/homebrew/Cellar/libvoikko/4.3.3/lib/voikko/5/mor-standard/mor.vfst
  libvoikko version: 4.3.3
  copied: 2026-05-06

analyser-gt-desc.hfstol:
  sha256 = 3bc4802d28fc3bb0a00b110454d638e5751c3a6ee41ac1bbed0b4db89b540326
  source: built locally from giellalt/lang-fin@HEAD
  HFST version: 3.17.1
  built: 2026-05-06
```
