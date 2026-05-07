# Omorfi Adapter Notes

This project does not bundle Omorfi. The `omorfi` parser mode in
`internal/parsecore` is an adapter slot that expects an external command via
`FINNESTDB_OMORFI_CMD`.

## Recommended Source

Use official Omorfi upstream sources:

- GitHub: `flammie/omorfi`
- PyPI: `omorfi`

The most practical starting options are:

1. Python package route
   - install the upstream Python package
   - write a thin adapter command that reads text from stdin
   - emit JSON matching FinEstDB's parser token/sentence shape

2. Command-line script route
   - install/build Omorfi from the official repo
   - use the provided analysis scripts
   - wrap them in a normalizing adapter that converts their output to JSON

## Why An Adapter Is Needed

FinEstDB expects the same shape returned by the Rust FFI parser:

```json
{
  "sentences": [
    {
      "tokens": [
        {
          "form": "kirjassani",
          "lemma": "kirja",
          "pos": "NOUN",
          "feats": {
            "Case": "Ine",
            "Number": "Sing",
            "Number[psor]": "Sing",
            "Person[psor]": "1"
          },
          "grammar_label": "inessive",
          "mwe_id": null
        }
      ]
    }
  ]
}
```

The `feats` object holds UD FEATS as key/value pairs; `parsecore.featsFromJSON`
sorts and joins them into the canonical `Case=Ine|Number=Sing|...` string used by
`internal/eval` and the rest of the pipeline. The omorfi adapter at
[`scripts/omorfi_adapter_example.py`](../scripts/omorfi_adapter_example.py)
populates this from `analysis.get_ufeats()`; the parallel EstNLTK adapter at
[`scripts/estnltk_adapter_example.py`](../scripts/estnltk_adapter_example.py)
runs Vabamorf form codes through [`scripts/_vabamorf_feats.py`](../scripts/_vabamorf_feats.py)
to produce the same shape.

Omorfi's own CLI/API formats are not identical to this, so the adapter is the
normalization layer.

## Current FinEstDB Contract

Set:

```bash
export OMORFI_ANALYSE_HFST="/absolute/path/to/omorfi.analyse.hfst"
export FINNESTDB_OMORFI_CMD="/absolute/path/to/adapter-command"
```

Then run:

```bash
go run ./cmd/parsertest \
  -dataset ./testdata/parser-eval/fi-gold-small.json \
  -parsers basic,custom,omorfi
```

The adapter command must:

1. read raw source text from stdin
2. accept `--lang FI`
3. return JSON on stdout
4. exit non-zero on failure

The subprocess timeout defaults to `5s` and can be overridden with a Go
duration string in `FINNESTDB_OMORFI_TIMEOUT` (e.g. `30s`, `1m`).

Current local example:

```bash
export OMORFI_ANALYSE_HFST="/Users/sagar/Downloads/projects/finnestdb/.cache/omorfi/omorfi.analyse.hfst"
export FINNESTDB_OMORFI_CMD="/Users/sagar/Downloads/projects/finnestdb/.venv-omorfi/bin/python /Users/sagar/Downloads/projects/finnestdb/scripts/omorfi_adapter_example.py"
```

## Practical Recommendation

Start with the Python package route first.

Reasons:

- less operational friction than full local Omorfi builds
- easier to write a small JSON adapter
- easier to swap or inspect during early benchmarking

Only move to the full CLI/build route if the Python route lacks the analyses or
performance characteristics you need.

## Estonian Parallel Path

For Estonian, the analogous baseline is now the `estnltk` parser mode, backed
by EstNLTK / Vabamorf and normalized through
`scripts/estnltk_adapter_example.py`.

Practical maintained source:

- PyPI: `estnltk`
- repo: `estnltk/estnltk`

Set:

```bash
export FINNESTDB_ESTNLTK_CMD="/absolute/path/to/python /absolute/path/to/scripts/estnltk_adapter_example.py"
```

or run:

```bash
make setup-estnltk
```

Then compare:

```bash
go run ./cmd/parsertest \
  -dataset ./testdata/parser-eval/et/gold/et-manual-v1.json \
  -parsers basic,custom,estnltk
```

See `docs/LEXICAL_PLAN.md` "Estonian-specific source choices and adapter
contract" for the EKI/Ekilex lexical-data import plan and attribution
requirements.
