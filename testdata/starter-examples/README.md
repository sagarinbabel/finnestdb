# Starter-deck example sentences

`{fi,et}-examples-v1.tsv` are the curated corpus example sentences that back the
cold-start "Top N words" starter deck. `cmd/pickexamples` produces them and
`cmd/seedcolddeck -examples <file>` attaches one to each matching deck card, so
every starter card carries a real usage sentence instead of a bare headword.

## Schema

Tab-separated, one header row, columns:

| column          | meaning                                                        |
|-----------------|----------------------------------------------------------------|
| `lemma`         | dictionary lemma of the deck card                              |
| `pos`           | part of speech (the sense discriminator paired with `lemma`)   |
| `form`          | the exact inflected surface form of the lemma in `sentence`    |
| `sentence`      | the curated example sentence (single line)                     |
| `source_corpus` | the corpus the sentence came from (`fi` / `et`)                |

At most two rows per `(lemma, pos)` (`examplesPerLemma`); the first row per
lemma is the one seedcolddeck attaches, the second is a spare alternate
inflection. The files are small by construction (≤ 2 × top lemmas per language).

## How the sentences were chosen

`cmd/pickexamples` ranks the same Top-N lemmas as `cmd/seedcolddeck`
(`internal/starterdeck.TopLemmas` over the OpenSubtitles baseline), then, for
each lemma, gathers candidate sentences from the corpus pipeline's
per-surface-form example index (`wordlist_user_friendly.tsv`'s `example_ref_id`)
and keeps the best 1–2 under deterministic "beautiful evocative" heuristics
(see `cmd/pickexamples/select.go`): a complete sentence (capitalized start,
terminal punctuation), 4–14 words, no digits/URLs/ALL-CAPS/quote fragments or
subtitle artifacts (leading dashes, speaker colons), the target form not
sentence-initial when alternatives exist (so it shows real inflection in
context), a preference for sentences whose other words are high-frequency
(readable at a beginner's level, using the OpenSubtitles rank list), and a
tie-break toward shorter/concrete sentences. Selection is deterministic: a
rerun on the same corpus yields the same file.

## Licensing note

Owner decision (2026-07-04): starter-deck cards must carry example sentences,
and individual sentences drawn from the project's local licensed corpora are an
acceptable source for this. The sensible boundary is that these are **single
sentences used as dictionary-style usage examples** - the same way a dictionary
quotes one line to illustrate a word - and never bulk reproduction of the
underlying corpus text. Each row records its `source_corpus` so provenance
travels with the sentence. This artifact intentionally caps at one to two
sentences per starter word; it is not, and must not become, a channel for
redistributing corpus bodies. See `docs/srs-deck-spec.md` "Example sentence
policy" for the product-level rule this implements.
