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

## Provenance

```
sha256(mor.vfst) = 5d3bfa406e589db45a90378c53b3132fb3ef4882cd5f00846eb4e3c50d0f957d
source: /opt/homebrew/Cellar/libvoikko/4.3.3/lib/voikko/5/mor-standard/mor.vfst
libvoikko version: 4.3.3
copied: 2026-05-06
```
