# FinEstDB — PRD (Alpha)

**One‑line:** Upload long Finnish/Estonian texts → parse → build decks → learn/review with FSRS.

**Tech:** Go monolith with Rust parsing via FFI. Vanilla TypeScript front end. No PWA/offline in alpha. Open source (MIT).

---

## 0) Scope & Principles

- **Languages:** Finnish (FI), Estonian (ET).
- **Alpha must‑haves**
  - Google sign‑in. Email+password to enable “Forgot password” flow.
  - Responsive UI with light/dark mode.
  - Upload/paste text → parse → create deck.
  - Learning/review with FSRS‑6. Buttons: Again/Hard/Good/Easy + Ignore + Mark as known.
  - Example sentences: from decks or generated; show per‑deck occurrence counts.
  - MWE (set phrase) recognition (seed lexicon).
- **Non‑goals in alpha:** Offline/PWA, sharing, exports/imports, admin UI.

---

## 1) Landing + Auth

### 1.1 Landing page
- Hero value prop, short bullets: *Upload text → parse → learn with FSRS*, *Finnish + Estonian*, *Open source*.
- CTAs: **Create account**, **Log in**.
- Theme toggle.

### 1.2 Accounts
Support both Google and email to enable password reset.

- **Google OAuth 2.0, Authorization Code.**
- **Email + password**
  - Passwords bcrypt.
  - Email verification required before deck creation.
  - **Forgot password:** request → email reset link (signed token, 30‑min TTL) → set new password.
- Successful auth redirects to **/dashboard**.

**Security:** HTTP‑only SameSite=Strict cookies; CSRF token on state‑changing requests; rate limit auth routes.

**Auth endpoints**
```
POST /api/auth/google/callback
POST /api/auth/signup            {email,password}
POST /api/auth/login             {email,password}
POST /api/auth/forgot            {email}
POST /api/auth/reset             {token,new_password}
POST /api/auth/logout
```

---

## 2) Post‑login “Main Page” (Dashboard)

### 2.1 KPIs (top tiles)
- **Words known** — distinct (lemma,pos) where (user_marked_known OR FSRS stability ≥ S_KNOWN) AND not ignored. Default `S_KNOWN=30`.
- **Words to review** — cards due `next_due <= now`, not ignored.
- **Words to learn (today)** — `max(0, new_words_per_day – learned_today)` and show backlog size (unseen items in selected decks).

### 2.2 Primary actions
- **Learn** button
  - If reviews due → start reviews.
  - Else → start learning up to `new_words_per_day – learned_today`.
- **Upload & create deck** (modal): *Title* (required), *Language* (FI/ET), *Paste text* or *Upload .txt/.md* (≤ 2 MB).
- **Your decks** list/table per deck: title, language, progress `known/unique`, due count, actions: **Review**, **Details**, **Delete**.

### 2.3 Layout (mobile‑first)
Header (logo, theme, user menu) → Tiles (3 KPIs) → **Learn** → **Upload** → Deck list.

---

## 3) Text input & Deck creation

### 3.1 Constraints
- Text up to **2 MB** per submission.
- Accept `.txt`/`.md` or paste. Required **Title**.

### 3.2 Flow
1) User submits → server parses (synchronous for alpha) → returns **Deck summary**:
   - total tokens, unique lemmas, new vs known, MWEs found, overlaps with existing decks.
2) Persist: deck record, sentences, per‑deck occurrence counts.

---

## 4) Learning & Review System

### 4.1 Card presentation
- **New item:** show **full card** immediately.
  - Front: sentence with **target highlighted**.
  - Back: **meaning**, **grammar label**, **lemma/root**, **example sentences** with per‑deck counts (e.g., `1× Deck A, 2× Deck B`). Badge `NEW`.
- **Review item (front modes, chosen randomly with weights):**
  1) **Word only**.
  2) **Sentence from any deck** containing the word (highlighted).
  3) **Generated sentence** that uses a different inflection/tense/case/voice (see 4.3).  
  Back = meaning + grammar label + lemma/root + examples + per‑deck counts.

### 4.2 Buttons and semantics
- **Again (0):** fail; quick retry; FSRS lapse update.
- **Hard (1):** hesitant recall; shorter interval.
- **Good (2):** normal success; default choice.
- **Easy (3):** instant recall; longer interval.
- **Ignore:** add to `user_ignored_lemmas` → excluded from queues now and future.
- **Mark as known:** add to `user_known_lemmas`; seed FSRS as mature (no learning steps).

**FSRS defaults**
- Algorithm: FSRS‑6 with target retention **0.90**.
- New items: single learn step; `Again` returns to queue; `Good` promotes to FSRS.
- Lapses: stability reset per FSRS; one relearn step.

**Daily policy**
- Cap new items by `new_words_per_day` (user setting). Reviews unlimited.

### 4.3 Example sentence selection & generation
- **Selection:** prefer deck sentences from the current deck; then other decks; include MWE sentence if part of a matched phrase.
- **Generation:** choose a feature change (FI examples: `Case Nom→Par`, `Tense Past→Pres`, `Voice Pass→Act`; ET analogues).
  - Inflect with **FST synthesizer** (Omorfi/EstNLTK/Vabamorf).
  - Validate by re‑parsing that the target features are present; retry if not.
  - Length ≤ 12 words; neutral register.

---

## 5) Parser (Rust via FFI)

### 5.1 Pipeline
1) **Normalize** (NFC), split sentences.
2) **Tokenize** with language rules; mark enclitics (FI: `-kin, -kaan, -pa/-pä`).
3) **Morph analysis (candidates)**
   - **FI:** Omorfi FST (+guesser).
   - **ET:** Vabamorf / EstNLTK.
4) **Disambiguation:** Viterbi over trigram POS model trained on UD treebanks; priors from lemma frequency lists; heuristics for capitalization and numeric tokens.
5) **Compounds (FI):** keep lemma = headword; record components if provided.
6) **MWEs / set phrases:**
   - Seed **MWE lexicon** of idioms with **slot constraints** for inflection.
   - Statistical n‑gram scoring (PMI/LLR) to suggest additions; confirm via morph slots.
   - Left‑to‑right DP segmentation; attach `mwe_id` to matched spans.
7) **Grammar labels:** map analyzer features → human labels.
   - Examples:  
     - `NOUN|Number=Sing|Case=Ill` → “Illative singular (‑iin)”.  
     - `NOUN|Number=Sing|Case=Ess` → “Essive singular (‑na/‑nä)”.  
     - `VERB|Mood=Ind|Tense=Past|Person=1|Number=Sing` → “Past, 1st sg”.
8) **Outputs per token (example)**
```json
{
  "form":"pankkiin",
  "lemma":"pankki",
  "pos":"NOUN",
  "feats":{"Case":"Ill","Number":"Sing"},
  "grammar_label":"Illative singular (-iin)",
  "mwe_id": null
}
```

**Note:** *toissapäivänä* is **Essive singular (‑nä)**, not Adessive.

### 5.2 FFI surface
- `analyze_text(lang, text) -> Sentences[Tokens]`
- `inflect(lang, lemma, pos, target_feats) -> form`
- `detect_mwes(lang, sentences) -> MweMatches[]`

### 5.3 Data sources (alpha)
- **FI:** Omorfi; FinnWordNet for glosses.
- **ET:** Vabamorf/EstNLTK; Estonian Wordnet for glosses.
- **Frequency:** public corpora (FI Internet Parsebank, Estonian National Corpus).
- **MWE seed:** JSON including *“leuka rintaan ja kohti uusia pettymyksiä”* with slot constraints.

---

## 6) Data Model (core)

```
users(id, google_id nullable, email nullable, email_verified, password_hash nullable, settings_json)
decks(id, user_id, title, lang, created_at)
sentences(id, deck_id, text, lang)

lemmas(lemma, pos, gloss, lang)                 -- dictionary headwords
occurrence(deck_id, sentence_id, token_ix, lemma, pos)

mwe(id, name, pattern_json, gloss, lang)

user_known_lemmas(user_id, lemma, pos)
user_ignored_lemmas(user_id, lemma, pos)

cards(id, user_id, lemma, pos, mwe_id nullable) -- one per user/target
card_state(card_id, fsrs_json, next_due, last_answer_at)
```

**Settings JSON (per user)**: `{"new_per_day":20,"retention":0.9,"theme":"system|light|dark"}`

---

## 7) API (minimal REST)

```
GET  /api/me                                 -> {known_count, due_count, new_capacity_today, decks:[...]}
PUT  /api/settings                           -> {new_per_day, retention, theme}

POST /api/decks                              -> {title, lang, text | file} -> {deck_id}
GET  /api/decks/:id/summary                  -> {tokens, unique_lemmas, new, known, mwe:[...]}

GET  /api/review/next                        -> card JSON (see §8) or 204 when empty
POST /api/review/answer                      -> {card_id, quality:0..3}
POST /api/card/ignore                        -> {lemma,pos}
POST /api/card/known                         -> {lemma,pos}
```

Auth endpoints in §1.2.

---

## 8) Front‑end contracts (vanilla TS)

### 8.1 Card JSON
```json
{
  "card_id":"c_123",
  "mode":"word|sentence|generated",
  "deck_counts":[["Deck A",1],["Deck B",2]],
  "front":{"type":"sentence","text":"Toissapäivänä menin pankkiin.","highlight":"Toissapäivänä"},
  "back":{
    "lemma":"toissapäivä",
    "meaning":"the day before yesterday",
    "grammar":"Essive singular (-nä)",
    "examples":[
      {"text":"Toissapäivänä menin pankkiin.","source_deck":"Everyday FI"}
    ]
  }
}
```

### 8.2 Dashboard JSON
```json
{
  "known_count": 1234,
  "due_count": 87,
  "new_capacity_today": 12,
  "decks":[
    {"id":"d1","title":"Everyday FI","lang":"FI","known":420,"unique":980,"due":20}
  ]
}
```

---

## 9) FSRS policy (defaults)

- Target retention: **0.90**. `Again=0, Hard=1, Good=2, Easy=3`.
- New items: one step; `Good` promotes; `Again` retries soon.
- Lapses: reset stability, one relearn step.
- Daily new cap: `new_per_day` (user). Reviews unlimited.

**When to press what (user docs)**
- **Again:** you failed recall.  
- **Hard:** barely recalled; want shorter interval.  
- **Good:** normal recall.  
- **Easy:** instant recall; want longer interval.  
- **Ignore:** irrelevant; hide forever.  
- **Mark as known:** you already know it; treat as mature.

---

## 10) Architecture

- **Monolith:** Go server orchestrates everything and calls Rust parsing library via FFI/cgo.
- **Front end:** single HTML + TS modules, plain CSS. No framework.
- **Themes:** CSS variables for light/dark.
- **Email:** SMTP provider for verification and reset emails.

**Repo layout**
```
/cmd/server         # Go main
/internal/api       # handlers
/internal/fsrs      # scheduler
/internal/store     # Postgres or SQLite
/internal/parserffi # cgo bindings
/web                # index.html, app.ts, styles.css
/docs               # this PRD and API docs
/mwe                # seed MWE patterns (JSON)
LICENSE, README.md
```

---

## 11) Security & Privacy

- HTTPS only. HSTS. Secure, HTTP‑only, SameSite=Strict cookies.
- CSRF token on POST/PUT/DELETE.
- Input limits: text ≤ 2 MB; MIME/type checks.
- Rate limiting per IP and per user for auth and parse routes.
- PII minimization: store email; no third‑party trackers.
- Audit logs for auth events and deck deletions.

---

## 12) Testing

- **Unit:** FSRS math; tag→label mapping; FFI glue; sentence selection; generator validate‑reparse.
- **Integration:** upload→parse→deck→review E2E.
- **Golden tests:** the sample sentence:  
  `Toissapäivänä menin pankkiin. Osoittautui, että se oli kiinni. No, leuka rintaan ja kohti uusia pettymyksiä!`  
  Expect: *toissapäivänä = Essive sg*; MWE detection for “leuka rintaan … pettymyksiä”.
- **Performance:** parse ≥ 1M tokens under target time on a laptop; bounded memory.

---

## 13) Acceptance criteria

- Upload 2 MB text → deck summary with unique lemmas, new vs known, MWE list, overlaps.
- Dashboard shows **Words known**, **To review**, **To learn (today)**, and deck list.
- **Learn** starts reviews if due; otherwise introduces new items up to daily cap.
- Card back shows meaning, grammar label, lemma/root, example list with per‑deck counts.
- Ignore/known affect queues immediately.
- Generated sentences validate by re‑parse.
- MWE “leuka rintaan ja kohti uusia pettymyksiä” recognized as a phrase item.

---

## 14) Out of scope (alpha)

- Offline/PWA, mobile apps, deck sharing, bulk imports/exports, admin UI, advanced analytics.

---

## 15) Open items

- Final dictionary sources and licenses (FI/ET).
- Size/coverage of the seed MWE lexicon.
- Corpora for frequency ranks.
- Exact FSRS default parameters (keep FSRS‑6 defaults unless user configures).
