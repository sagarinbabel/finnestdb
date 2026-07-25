# Public Frequency Baselines - Finnish & Estonian

_Created: 2026-05-07. Methodology, coverage analysis, and license
attribution for the public frequency lists used as comparison baselines.
The lists themselves live under `localdata/frequency/` (gitignored);
populate them by running `cmd/fetchfrequency` (or `make
fetch-frequency-baselines`)._

This project's primary contribution on inflected-form frequency is the
user-pasted-text aggregation tracked in [`TODO.md`](../TODO.md) "Discover
the most-frequent inflected forms in user-pasted text" and
[`docs/ML_IDEAS.md` §2b](ML_IDEAS.md). The lists described here are
**baselines for comparison**, not the contribution.

## Why this exists

Frequency lists for Finnish and Estonian are not novel. Plenty of
corpora have published top-N rankings. **What is novel in this project**
is:

1. Frequency aggregation of **inflected surface forms**, not lemmas -
   most published lists rank lemmas, which is the wrong unit for a
   learner reading running text.
2. Aggregation over **text users actually paste**, weighted by real
   reading interest, not by what corpus engineers picked.
3. Comprehension-coverage curves surfaced as a learner-facing UX metric
   (see `Comprehension prediction per deck` in `TODO.md`).

The lists in this directory let us answer the question "how different
is the user-aggregated ranking from each baseline corpus's ranking?" -
which is the only reason the comparison matters.

## What's in `localdata/frequency/`

Populated by `cmd/fetchfrequency`. Layout:

```
localdata/frequency/
├── fi/
│   ├── opensubtitles-2018-fi-50k.txt           # top 50k forms, OpenSubtitles 2018
│   ├── UD_Finnish-TDT-forms.tsv                # all forms (incl. punct), UD-FI-TDT train
│   ├── UD_Finnish-TDT-forms-words.tsv          # words only (PUNCT/SYM filtered)
│   └── UD_Finnish-TDT-lemmas.tsv               # lemmas, for FI lemma-vs-form comparison
└── et/
    ├── opensubtitles-2018-et-50k.txt
    ├── UD_Estonian-EDT-forms.tsv
    ├── UD_Estonian-EDT-forms-words.tsv
    └── UD_Estonian-EDT-lemmas.tsv
```

### File formats

- **OpenSubtitles files** (`opensubtitles-2018-*-50k.txt`):
  space-separated `<form> <count>` rows, ranked by count descending.
  Native upstream format from [hermitdave/FrequencyWords][hermitdave].
  **Forms are surface inflected forms, not lemmas** - exactly what we
  need.
- **UD files** (`UD_*-{forms,lemmas}*.tsv`): tab-separated
  `<token>\t<count>` rows, ranked by count descending. Derived from the
  CoNLL-U train splits of the upstream UD treebanks. Column 2 (FORM)
  for forms files, column 3 (LEMMA) for lemmas files. The `-words`
  variants drop tokens whose UPOS is `PUNCT` or `SYM`.

## Coverage curves - top N forms vs. % of running text

This is the headline analysis these baselines exist to support.

### Finnish

| Source | top-100 | top-500 | top-1000 | top-2000 | top-5000 | top-10000 |
|---|---:|---:|---:|---:|---:|---:|
| OpenSubtitles 2018 (FI) | 37.5% | 56.9% | **65.2%** | 72.8% | 82.0% | 88.3% |
| UD-Finnish-TDT (words) | 20.9% | 33.4% | **40.1%** | 47.7% | 59.1% | 68.8% |

### Estonian

| Source | top-100 | top-500 | top-1000 | top-2000 | top-5000 | top-10000 |
|---|---:|---:|---:|---:|---:|---:|
| OpenSubtitles 2018 (ET) | 45.1% | 65.5% | **72.9%** | 79.5% | 87.1% | 91.9% |
| UD-Estonian-EDT (words) | 22.7% | 36.1% | **42.9%** | 50.6% | 61.8% | 70.7% |

### What the gap means

The "top 1000 forms" claim is **register-dependent**:

- A learner reading subtitles or watching dialogue understands ~65–73%
  of running text from the top 1000 forms.
- A learner reading news/literature understands ~40–43% of running
  text from the top 1000 forms.

Conversational text has a tighter Zipf head; written text has a long
tail of rare nouns and proper names. So "1000 forms = 80% comprehension"
is roughly true for conversational ET, much less true for written FI.

The user-aggregated frequency this project will produce reflects the
register mix of pasted text, which is presumably what learners actually
want to read. If your users mostly paste novels, expect curves closer
to the UD lines. If they mostly paste TV scripts, expect curves closer
to OpenSubtitles. **Don't promise a fixed comprehension % at a fixed
form count without specifying the register.**

See [`docs/CROSS_LANGUAGE_STRATEGY.md` "Measurable Divergences"](CROSS_LANGUAGE_STRATEGY.md)
for the cross-language interpretation of these curves.

## Caveats and biases

- **OpenSubtitles 2018**: subtitle text. Skewed toward dialogue,
  present tense, second-person, and short utterances. Underrepresents
  written registers, formal language, and domain vocabulary.
  Year-frozen (2018), so post-2018 vocabulary (covid-era, recent
  loanwords) is absent or underweighted. Tokenization is OpenSubtitles'
  own; not necessarily aligned with this project's parser.
- **UD treebanks**: written text, mostly news and literature, manually
  annotated. Small (under half a million tokens for FI, under 350k for
  ET). Reliable annotation but the absolute counts have high variance
  in the long tail.
- **Corpus-size confound**: OpenSubtitles ET corpus is roughly half
  the size of FI; UD-Estonian-EDT is roughly 2× UD-Finnish-TDT. Smaller
  corpora inflate top-N coverage (shorter long tail). The fact that ET
  has higher coverage in *both* directions of this confound suggests
  the FI-vs-ET gap is at least partly real, but a size-matched
  re-measurement is on the research-goals list. See `TODO.md` "Re-test
  FI vs ET top-N coverage".
- **None of these use this project's parser/tokenizer.** When comparing
  against this project's user-text-derived frequency, account for
  tokenization differences (e.g. clitic handling, compound splitting,
  hyphen treatment).

## How to regenerate

```bash
make fetch-frequency-baselines
# or directly:
go run ./cmd/fetchfrequency
```

This downloads the six files into `localdata/frequency/{fi,et}/` and
derives the words-only / lemma TSVs from the UD CoNLL-U train splits.
Idempotent - re-running with no upstream change produces byte-identical
files.

## License & attribution

Each upstream's license governs the files derived from it. Files in
`localdata/frequency/` (gitignored) are not redistributed by this
repository - the fetcher pulls them at user request.

### OpenSubtitles 2018 lists

**Files:** `fi/opensubtitles-2018-fi-50k.txt`,
`et/opensubtitles-2018-et-50k.txt`

**Compiler:** Hermit Dave -
[hermitdave/FrequencyWords](https://github.com/hermitdave/FrequencyWords)

**License:** Creative Commons Attribution-ShareAlike 4.0 International -
<https://creativecommons.org/licenses/by-sa/4.0/>

**Underlying corpus:** OpenSubtitles 2018, distributed by [OPUS - the
open parallel corpus](https://opus.nlpl.eu/OpenSubtitles-v2018.php).
OpenSubtitles content is itself contributed by users of
[opensubtitles.org](https://www.opensubtitles.org/) under their terms.

**Citation (recommended):**

> Hermit Dave (2018). FrequencyWords: top frequency lists for ~50
> languages, derived from OpenSubtitles 2018 via OPUS.
> <https://github.com/hermitdave/FrequencyWords>

### UD-Finnish-TDT

**Files:** `fi/UD_Finnish-TDT-forms.tsv`,
`fi/UD_Finnish-TDT-forms-words.tsv`, `fi/UD_Finnish-TDT-lemmas.tsv`

**Source:** [UniversalDependencies/UD_Finnish-TDT](https://github.com/UniversalDependencies/UD_Finnish-TDT)

**License:** Creative Commons Attribution-ShareAlike 4.0 International -
<https://creativecommons.org/licenses/by-sa/4.0/>

**Citation (abbreviated):**

> Haverinen, K., Nyblom, J., Viljanen, T. et al. (2014). Building the
> essential resources for Finnish: the Turku Dependency Treebank.
> Language Resources and Evaluation 48(3): 493–531. Plus subsequent UD
> releases - see upstream repo's `README.md` for the full contributor
> list.

### UD-Estonian-EDT

**Files:** `et/UD_Estonian-EDT-forms.tsv`,
`et/UD_Estonian-EDT-forms-words.tsv`, `et/UD_Estonian-EDT-lemmas.tsv`

**Source:** [UniversalDependencies/UD_Estonian-EDT](https://github.com/UniversalDependencies/UD_Estonian-EDT)

**License:** **Creative Commons Attribution-NonCommercial-ShareAlike
4.0 International** -
<https://creativecommons.org/licenses/by-nc-sa/4.0/>

**Important:** The `BY-NC-SA` (non-commercial) clause means derived
files **cannot be used in commercial offerings** without separate
permission from the upstream maintainers. This project's intended use -
internal frequency comparison and research - is non-commercial and
within scope. Before any commercial deployment that ships these counts
(or rankings derived from them), revisit upstream for licensing.

**Citation (abbreviated):**

> Muischnek, K., Müürisep, K., Puolakainen, T. et al. (2014). Estonian
> dependency treebank and its annotation scheme. Plus subsequent UD
> releases - see upstream repo's `README.md` for the full contributor
> list.

## Anything derived in this project

**Frequency data computed from user-pasted text in this project is
separate from these baselines** and is governed by this project's own
license and the user privacy policy in `docs/GO_LIVE_CHECKLIST.md`.
Don't conflate the two when redistributing.

[hermitdave]: https://github.com/hermitdave/FrequencyWords
