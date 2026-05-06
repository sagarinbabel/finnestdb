# Kotus Nykysuomen sanalista — Tracked Snapshot

## Source

`nykysuomensanalista2024.txt` is a snapshot of the Kotus Nykysuomen
sanalista (2024 edition) downloaded from:

> https://kaino.kotus.fi/lataa/nykysuomensanalista2024.txt

Linked from the Kotus landing page:

> https://kotus.fi/sanakirjat/kielitoimiston-sanakirja/nykysuomen-sana-aineistot/nykysuomen-sanalista/

## License

**CC BY 4.0** — Creative Commons Attribution 4.0 International.
<https://creativecommons.org/licenses/by/4.0/>

## Attribution

Kotus, Kotimaisten kielten keskus / Nykysuomen sanalista (2024).

## Format

Tab-separated values with a header row:

| Column | Field | Notes |
|---|---|---|
| 1 | Hakusana | The headword. |
| 2 | Homonymia | Empty or `1`–`4` for homonyms. |
| 3 | Sanaluokka | Word class in Finnish (`substantiivi`, `verbi`, `adjektiivi`, …). Comma-separated when one entry has multiple classes (e.g. `adjektiivi, substantiivi` for `sininen`). |
| 4 | Taivutustiedot | Inflection class number, optionally followed by `*<gradation-letter>` (e.g. `38`, `1*I`, `41*A`). Empty for compounds and other entries Kotus doesn't classify. |

The 2024 edition has 104,742 data rows. About 54% have empty
`Taivutustiedot` — typically compound headwords whose inflection
follows from a base lemma already in the list.

## How this snapshot is used

The file is imported by [`cmd/importkotus`](../../cmd/importkotus) into
the `lemmas` table. Each row's `Taivutustiedot` becomes the row's
`paradigm_class` (e.g. `1-I`, `38`); the `Sanaluokka` is mapped to UPOS
via [`wordClassMap`](../../cmd/importkotus/main.go).

The importer **enriches** existing kaikki rows with `paradigm_class`
without claiming the `source` or `gloss` columns. New Kotus-only
headwords are inserted at `source='kotus'`, `source_priority=10`,
`gloss=NULL`.

## Refreshing this snapshot

To update against a newer Kotus distribution:

```bash
curl -L -o data/kotus/nykysuomensanalista2024.txt \
    https://kaino.kotus.fi/lataa/nykysuomensanalista2024.txt
make import-kotus-fi
```

When Kotus publishes a new annual edition, rename the file (e.g.
`nykysuomensanalista2025.txt`) and update `cmd/importkotus`'s default
`-file` path and this NOTICE.
