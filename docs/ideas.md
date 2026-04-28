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

## Learner UX Ideas (Surasura-Informed)

These ideas are informed by analyzing [SonicSandbox/surasura](https://github.com/SonicSandbox/surasura),
a Japanese/Chinese immersion learning tool. The architectural approaches differ
significantly (Python desktop GUI vs our Rust+Go+TS web stack), but several of
its user-facing concepts translate well.

### Comprehension prediction as a core metric

Surasura's central UX is: "before studying this content you understand X%,
after learning the top N words you'll understand Y%." This is a compelling
motivator.

We already define token-weighted coverage in `srs-deck-spec.md`. The next step
is to surface it prominently:

- Show predicted comprehension % on each deck card in the study list
- Show a before/after projection when the user is deciding what to study next
- Compute marginal gain per word so the user can see "learning these 20 words
  moves you from 72% to 85%"

### Cross-deck study optimization

Surasura generates "highest-leverage order" study sequences by analyzing word
frequency across a user's entire content library, not just one text.

Our current new-card ranking sorts by token_count within the selected source.
A cross-deck ranking would ask: "which word, if learned, unlocks the most
tokens across all the user's active decks?" This is a better signal when a
learner has multiple books or shows queued.

Implementation sketch:

- For each candidate lemma, sum token_count across all study-list decks
- Weight by deck priority (high/medium/low, set by study-list sort order or
  explicit priority field)
- Use this cross-deck score as the primary ranking for new-card introduction

### Content priority weighting

Surasura organizes content into HighPriority, LowPriority, and GoalContent
folders. Users care more about understanding some content than others.

Our deck hierarchy and study-list sort order partially address this. To make
it explicit:

- Add an optional `priority` field to `study_list_entries` (high/medium/low)
- Use priority as a multiplier in cross-deck study optimization
- Show priority labels in the study-list UI

### EPUB and file upload

Surasura extracts text from EPUBs and Anki decks. Currently FinEstDB only
accepts pasted text, which creates friction for book learners.

Adding EPUB support is straightforward:

- EPUBs are zip files containing XHTML content documents
- Extract chapter text, strip HTML, concatenate in spine order
- Feed into the existing parse pipeline

This should come after the import deck endpoint is working but before the
learning features are complete.

### Known-word import from external tools

Surasura imports known vocabulary from Anki, Migaku, and Jiten.moe so that
coverage metrics and study sequencing are useful from the first session.

Without a bootstrap mechanism, a returning learner's comprehension shows as
0% until they manually mark hundreds of known words. That defeats the purpose
of the coverage metric.

Recommended import sources for FinEstDB:

- **Anki export** (.apkg or tab-delimited .txt): extract front fields, run
  through our dictionary lookup + fallback chain to resolve lemma+POS, and
  insert into `user_known_lemmas`
- **CSV/TSV**: simple format for users with custom word lists
- **Future**: other SRS tools popular in the Finnish/Estonian learning community

### Progress dashboard

Surasura has an interactive HTML dashboard showing learning progress over time.
Our frontend already has a dashboard tab placeholder.

Dashboard should show:

- Total known lemmas over time (cumulative chart)
- Cards in review vs mature vs new
- Comprehension trend per deck ("you went from 60% to 78% on this book")
- Daily review count and streak
