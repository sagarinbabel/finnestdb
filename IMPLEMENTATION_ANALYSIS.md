# FinEstDB Implementation Analysis

## Parser Implementation: Challenges & Solutions

### Your Concern is Valid

The parser described in the PRD is **extremely complex** and represents a significant engineering challenge. Here's why:

### What Makes It Hard

1. **FST Integration (Omorfi/Vabamorf)**
   - These are mature but complex C/C++ libraries
   - FFI bindings to Rust then to Go adds layers of complexity
   - Version compatibility and build dependencies can be painful
   - Performance overhead from multiple FFI layers

2. **Viterbi Disambiguation with Trigram Models**
   - Requires training data from UD treebanks
   - Model training pipeline is non-trivial
   - Need significant computational linguistics expertise
   - Accuracy depends heavily on training data quality

3. **MWE Detection with Slot Constraints**
   - No existing library does this for Finnish/Estonian
   - Custom algorithms needed for DP segmentation
   - Slot constraint validation is complex
   - Requires extensive linguistic knowledge to build MWE lexicon

4. **Sentence Generation with FST Synthesis**
   - Most challenging part of the system
   - Feature manipulation (case/tense/voice changes) is error-prone
   - Validation by re-parsing is computationally expensive
   - High failure rate likely, requiring multiple retries

### Pragmatic Approaches (Recommended)

#### Phase 1: MVP with Simpler Parsing

**Option A: Use Existing Libraries Directly (Easiest)**
- **For Finnish:** Use [Turku Neural Parser](https://turkunlp.org/Turku-neural-parser-pipeline/) or [UralicNLP](https://github.com/mikahama/uralicNLP)
- **For Estonian:** Use [Stanza Estonian](https://stanfordnlp.github.io/stanza/) or [EstNLTK](https://github.com/estnltk/estnltk)
- **Pro:** These are Python libraries with good documentation
- **Pro:** Already handle tokenization, POS tagging, lemmatization, morphology
- **Con:** Python integration with Go (not ideal, but workable via REST API or subprocess)

**Option B: Pre-process with Python, Store in DB**
- Run Python parsing service separately
- Store parsed results in Postgres
- Go backend only handles FSRS logic and UI
- **Pro:** Decoupled architecture, easier to develop/test
- **Pro:** Can swap parsers without touching Go code
- **Con:** Two codebases to maintain

**Option C: Use Commercial NLP API**
- Services like [Giellatekno](https://giellatekno.uit.no/) provide Uralic language tools
- Could potentially offer API access
- **Pro:** Offload complexity entirely
- **Con:** Cost, dependency on external service
- **Con:** May not offer all features you need

#### Phase 2: Simplify MWE Detection

**Instead of complex DP + slot constraints:**

1. **Start with Exact Match Dictionary**
   - Build JSON list of ~100-500 common idioms
   - Simple string matching (with normalization)
   - Good enough for alpha

2. **Statistical Detection Later**
   - Add PMI/LLR scoring in future versions
   - Collect user corpus data first
   - Build from actual usage patterns

#### Phase 3: Defer Sentence Generation

**Alternative approach:**
- **Show only actual sentences from decks** (simpler, more reliable)
- **Add "similar examples" from corpus** (pre-computed offline)
- **Defer synthetic generation** to post-alpha or remove entirely

Users probably prefer real examples over generated ones anyway!

---

## Data Sourcing: What You Actually Need

### Critical Data (Must Have)

#### 1. Lemma → Meaning Dictionary
**Finnish:**
- [FinnWordNet](https://github.com/frankier/fiwn) (Open, CC BY 4.0)
- [Kaikki.org](https://kaikki.org/) (Wiktionary extraction, free)
- Quality: Good but incomplete coverage

**Estonian:**
- [Estonian Wordnet](https://github.com/keeleleek/Estonian-Wordnet-Pipeline) (Open)
- [Kaikki.org Estonian](https://kaikki.org/)
- Quality: Decent but sparser than Finnish

**Your Options:**
- Use Kaikki.org as primary (better coverage)
- Supplement with WordNet for rare words
- **AI-generated fallback:** For missing entries only, with user validation flag

#### 2. Morphological Analyzer
**Finnish:**
- [UralicNLP](https://pypi.org/project/uralicNLP/) (wraps Omorfi, MIT license)
- [Voikko](https://voikko.puimula.org/) (GPL, more restrictive)

**Estonian:**
- [EstNLTK v1.7+](https://github.com/estnltk/estnltk) (GPL-2.0)
- [Vabamorf](https://github.com/Filosoft/vabamorf) (GPL)

**Licensing Note:** Most are GPL, which is compatible with your MIT if you use them as separate services/processes, not linked libraries.

#### 3. Frequency Lists
**Finnish:**
- [Finnish Internet Parsebank](https://korp.csc.fi/) (research access)
- [OpenSubtitles frequency](https://github.com/hermitdave/FrequencyWords) (free)
- Quality: Subtitle data is informal but good enough

**Estonian:**
- [Estonian National Corpus](https://www.cl.ut.ee/korpused/segakorpus/) (research access)
- OpenSubtitles (same as above)

**Pragmatic Approach:**
- Use OpenSubtitles for both (free, no restrictions)
- Bias towards more formal sources for grammar labels if needed

### Optional Data (Nice to Have)

#### 4. Example Sentences
**Best approach:**
- **Tatoeba.org** (CC BY 2.0, 30k+ FI sentences, 10k+ ET)
- User-uploaded deck sentences (your own corpus grows over time)
- AI-generated as last resort with "(AI-generated)" label

#### 5. MWE Seed Lexicon
**Reality Check:**
- No comprehensive MWE dictionary exists for FI/ET
- You'll need to **manually curate** initial 100-500 idioms
- Sources:
  - Finnish idiom books/websites
  - "Idioomisõnastik" for Estonian
  - Crowd-sourced from beta users

**Time investment:** ~40-80 hours to build decent seed lexicon

---

## AI-Generated Data: When It's Acceptable

### ✅ Safe Uses
1. **Supplemental glosses** for rare words (with flag: "AI-suggested")
2. **Example sentences** for words with <2 real examples (labeled clearly)
3. **Grammar explanations** for features (validated against linguistic resources)
4. **Translation of MWE definitions** (human-validated)

### ❌ Risky Uses
1. **Core lemma→meaning mappings** (too error-prone)
2. **POS tags or morphological features** (will break FSRS logic)
3. **Frequency rankings** (completely unreliable)

### Best Practice
- Always label AI content: `"source": "ai-generated"`
- Allow users to report errors
- Gradually replace with validated data

---

## Recommended MVP Architecture

### Simplified Tech Stack

```
┌─────────────────┐
│  Go Backend     │
│  (FSRS + API)   │
└────────┬────────┘
         │
         ↓
┌─────────────────┐        ┌──────────────────┐
│  PostgreSQL     │  ←───→ │  Python Parser   │
│  (Data + Queue) │        │  (UralicNLP/     │
└─────────────────┘        │   EstNLTK)       │
                           └──────────────────┘
         ↑
         │
┌─────────────────┐
│  Vanilla TS UI  │
└─────────────────┘
```

### MVP Parser (Python Service)

**Simple REST API:**
```python
# parser_service.py
from flask import Flask, request, jsonify
from uralicNLP import uralicApi  # Finnish
from estnltk import Text  # Estonian

@app.route('/parse', methods=['POST'])
def parse_text():
    text = request.json['text']
    lang = request.json['lang']

    if lang == 'FI':
        return parse_finnish(text)
    else:
        return parse_estonian(text)

def parse_finnish(text):
    sentences = split_sentences(text)
    parsed = []

    for sent in sentences:
        tokens = []
        for word in sent.split():
            # Use UralicNLP
            analyses = uralicApi.analyze(word, "fin")
            if analyses:
                best = disambiguate_simple(analyses)
                tokens.append({
                    'form': word,
                    'lemma': best['lemma'],
                    'pos': best['pos'],
                    'feats': best['feats'],
                    'grammar_label': make_label(best)
                })
        parsed.append(tokens)

    return jsonify({'sentences': parsed})
```

**No FFI, no Rust, no Viterbi (for MVP):**
- Use UralicNLP's built-in heuristics for disambiguation
- Simple frequency-based selection
- Good enough for alpha testing

---

## Phased Roadmap

### Alpha (3-6 months)
- ✅ Basic auth (Google + email)
- ✅ Text upload → simple parsing (Python service)
- ✅ Deck creation from parsed text
- ✅ FSRS-6 learning/review
- ✅ Real example sentences only
- ✅ Manual MWE list (100 entries)
- ✅ Grammar labels (basic mapping)
- ❌ No sentence generation
- ❌ No sophisticated disambiguation
- ❌ No automatic MWE detection

### Beta (6-12 months)
- Add statistical MWE scoring
- Improve disambiguation (train simple model)
- Expand MWE lexicon (500+ entries)
- Add sentence generation (basic FST)
- User-reported corrections

### Version 1.0 (12-18 months)
- Full Viterbi disambiguation
- Comprehensive MWE detection
- Advanced sentence generation
- High-quality validated data
- Mobile apps

---

## Key Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Parser too complex to build | **HIGH** | Critical | Use existing Python libraries, defer custom features |
| Insufficient dictionary data | Medium | High | Use Kaikki.org + user validation system |
| FST integration fails | High | Medium | Skip sentence generation for alpha |
| MWE lexicon too small | **HIGH** | Medium | Start with 100 entries, crowd-source from users |
| GPL licensing conflicts | Low | High | Use libraries as separate process (not linked) |
| Performance issues | Medium | Medium | Pre-compute parsing, cache aggressively |

---

## Bottom Line Recommendations

### To Make This Project Viable:

1. **START SIMPLE:**
   - Use UralicNLP + EstNLTK via Python service
   - Skip Rust/FFI entirely for alpha
   - No custom Viterbi - use built-in heuristics

2. **DATA STRATEGY:**
   - Kaikki.org for lemma→meaning
   - OpenSubtitles for frequency
   - Manually curate 100 core MWEs
   - Use AI sparingly, always labeled

3. **DEFER HARD PARTS:**
   - No sentence generation in alpha
   - No statistical MWE detection initially
   - Simple string-match MWEs

4. **VALIDATE WITH USERS:**
   - Launch with Finnish only first
   - Get feedback on parsing accuracy
   - Iterate based on real usage
   - Add Estonian after proving concept

### Timeline Estimate:
- **Alpha with simplified parser:** 3-4 months (solo dev) or 6-8 weeks (with help)
- **Full PRD implementation:** 12-18 months (ambitious even for a team)

### Your AI Data Concern:
- ✅ Use AI for supplemental glosses/examples (labeled)
- ❌ Don't use AI for core linguistic data
- ✅ Build validation system where users can flag errors
- ✅ Plan to gradually replace with vetted data

---

## Reality Check

The PRD describes a system that would typically require:
- 2-3 full-time developers
- 1 computational linguist
- 12-18 months
- €200k-500k budget

**For a solo/small team project:**
- Cut scope by 70%
- Focus on core FSRS learning loop
- Use off-the-shelf parsing
- Iterate based on user feedback

The mockup shows what the UX *could* be. The backend can start much simpler and grow over time.
