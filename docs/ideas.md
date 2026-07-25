# FinnEst Development Ideas & Decisions

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

Surasura extracts text from EPUBs and Anki decks. FinnEst now accepts pasted
text plus `.txt`, `.md`, and `.epub` uploads for Inspect/workbench, and supports
known-word import through paste, simple files, and AnkiConnect. Offline Anki
`.apkg` extraction is still missing.

EPUB support has shipped. The implementation follows the same shape:

- EPUBs are zip files containing XHTML content documents
- Extract chapter text, strip HTML, concatenate in spine order
- Feed into the existing parse pipeline

The remaining Anki-file equivalent is `.apkg` extraction for known-word import,
not parse upload.

### Author-partnered learnable texts (owner vision, 2026-07-04)

Longer-term catalog direction beyond PD/CC sources: collaborate directly with
authors who want their work hosted and made learnable - especially easy-Finnish
(selkokieli) texts. Many are funded through Finnish cultural grants, so authors
may be open to licensed hosting for learners. This would give the catalog
modern, purpose-written easy texts that public-domain sources structurally
cannot provide (PD Finnish/Estonian is pre-1950s by definition). Requires: a
permission/licensing record per text (the catalog's license/attribution fields
already carry this), and eventually an author-facing intake path.

### Known-word import from external tools

Surasura imports known vocabulary from Anki, Migaku, and Jiten.moe so that
coverage metrics and study sequencing are useful from the first session.

Without a bootstrap mechanism, a returning learner's comprehension shows as
0% until they manually mark hundreds of known words. That defeats the purpose
of the coverage metric.

Recommended import sources for FinnEst:

- **AnkiConnect**: already implemented for local running Anki desktop decks.
- **Anki export** (`.apkg`): future offline path; extract front fields and feed
  the same known-word pipeline.
- **CSV/TSV/text files**: already implemented as one word per line or
  first-column import; add clearer export guidance.
- **Future**: other SRS tools popular in the Finnish/Estonian learning community

Data-model note: current imports submit surface strings but store resolved
lemma/POS rows in `user_known_lemmas`. The product target should preserve the
submitted surface forms as first-class known-word evidence and treat lemma/POS
as derived data.

### Progress dashboard

Surasura has an interactive HTML dashboard showing learning progress over time.
Our frontend already has a dashboard tab placeholder.

Dashboard should show:

- Total known vocabulary over time (cumulative chart). Current implementation is
  lemma-backed; the target model is surface-first.
- Cards in review vs mature vs new
- Comprehension trend per deck ("you went from 60% to 78% on this book")
- Daily review count and streak

## Making it AI native

Today FinnEst is not AI-native: parsing is rule-based (`parser/src/lib.rs`,
`internal/parsecore/`), the dictionary is a static kaikki.org import, review
scheduling is FSRS, and the API is plain CRUD with no generative endpoints,
streaming, or tool-calling. "AI-native" here means making an LLM a
load-bearing part of the product rather than a bolt-on. The highest-leverage
move is a Claude-powered tutor grounded in the learner's own deck state, then
growing outward.

### Phase 1 - LLM-grounded explanations (small surface, high value)

- New Go handler `POST /api/explain` taking `{lemma, surface, sentence}`,
  returning a streamed natural-language explanation: morphology breakdown,
  register/usage notes, fresh examples at the learner's level.
- Wire to Claude via the Anthropic Go SDK. Default to Sonnet 4.6
  (`claude-sonnet-4-6`) for quality/latency, Haiku 4.5
  (`claude-haiku-4-5-20251001`) for a cheap fast path.
- Ground the prompt with the existing dictionary entry pulled from
  `internal/store` (we already have structured lookups - no embeddings needed
  yet).
- Use **prompt caching** on the system prompt + per-lemma dictionary chunk.
  The dictionary context is large and stable per lemma → ideal cache target,
  big cost win on repeat lookups.
- Stream tokens via SSE to the SPA.
- Critical files: new `internal/ai/claude.go`, new handler appended to
  `internal/api/handlers.go`, new `web/explain.ts` UI module, route
  registration in `cmd/server/main.go`.

### Phase 2 - Conversational practice partner

- New `/api/chat` endpoint with multi-turn history persisted to SQLite (add a
  `chats` table to `internal/store`).
- System prompt enforces target language (FI or ET), CEFR level inferred from
  deck mastery, corrections in the learner's L1.
- **Tool use**: expose `lookup_lemma`, `add_to_deck`, `mark_card_due`,
  `list_due_cards` as Claude tools. This is the agentic piece - the AI takes
  real actions against the DB on the user's behalf.
- Critical files: `internal/ai/tools.go` (tool schemas + dispatch),
  `internal/api/chat.go`, store extension in `internal/store/chats.go`.

### Phase 3 - Hybrid parser fallback

- When `internal/parsecore` returns "unknown" for a token (rare lemma,
  code-switching, typo), fall back to an LLM call that returns a structured
  `MorphAnalysis` JSON. Deterministic parsers stay primary - LLM is the long
  tail.
- Cache results in a new SQLite table keyed on `(surface, lang)` so each
  unknown is paid for once across all users.
- Critical files: extend `internal/parsecore/registry.go` with a `claude`
  adapter; new `internal/store/parse_cache.go`.

### Phase 4 - Embeddings for semantic features

- Add an embedding column (BLOB + cosine in Go is fine at SQLite scale;
  pgvector if we outgrow it) to `dictionary_glosses` and `sentences`.
- Use Voyage or OpenAI embeddings, or `nomic-embed-text` locally to preserve
  the "all local" ethos.
- Unlocks: "sentences using X in a similar sense", "recommend next reading at
  my level", deduping near-identical cards in `internal/store/cards.go`.

### Phase 5 - Speech (optional, transformative)

- Whisper for ASR on user-recorded pronunciation; LLM judges fluency and
  gives feedback. Out of scope for a first cut.

### Recommended starting point

**Phase 1 only.** Roughly 300 lines of Go plus a small UI change, gives users
an obvious AI moment ("explain this word in context, in my language, with new
examples"), and proves the integration pattern (streaming, caching,
grounding) before committing to bigger changes.

### Verification

- Unit test the Claude client against a recorded fixture
  (`internal/ai/claude_test.go`).
- Integration test behind a build tag (`-tags=live`) that hits `/api/explain`
  with a real `ANTHROPIC_API_KEY`, asserts the response streams and
  references the supplied dictionary entry.
- Manual: paste a Finnish sentence in the SPA, click a token, watch the
  explanation stream in. Compare Sonnet 4.6 vs Haiku 4.5 on quality and
  latency.
- Cost check: log `cache_read_input_tokens` vs `input_tokens` from the API
  response - cache hit rate should be >80% on repeated lookups for the same
  lemma.
