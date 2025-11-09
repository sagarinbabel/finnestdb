# FinEstDB Development Ideas & Decisions

## Parser Strategy

### Parser Concerns (You're Right!)

- The PRD describes a system that would take 12-18 months with a team
- Rust FFI + Viterbi + MWE detection is extremely complex
- **Solution:** Use existing Python libraries (UralicNLP/EstNLTK) via REST API
- Defer sentence generation and sophisticated features to post-alpha

### Data Sourcing

#### ✅ Approved Sources
- **Use Kaikki.org** for lemma→meaning dictionaries (free, good coverage)
- **Use OpenSubtitles** frequency lists (free, no restrictions)
- **AI-generated content is OK** for supplemental glosses (labeled clearly)

#### ⚠️ Manual Work Required
- Manually curate **100 core MWEs** to start (40-80 hours work)
  - Focus on high-frequency expressions
  - Cover essential patterns for learners
  - Can expand later based on usage data

#### ❌ Don't Use
- **Don't use AI for core linguistic data**
  - No AI-generated lemmas, POS tags, or grammatical features
  - No AI-generated frequency rankings
  - Keep AI contributions clearly labeled and limited to supplemental content
