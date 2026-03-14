# FinEstDB Architecture

> Parser workbench for Finnish and Estonian text — dictionary-backed lemmatization and parser evaluation.

## System Overview

```mermaid
graph TB
    subgraph Frontend["Frontend (TypeScript SPA)"]
        UI["index.html + app.ts"]
        LD["Language Detection<br/><i>character frequency analysis</i>"]
        RT["Results Table<br/><i>sortable, coverage score</i>"]
        TH["Theme Manager<br/><i>light / dark</i>"]
    end

    subgraph Server["Go HTTP Server (cmd/server)"]
        MUX["HTTP Router<br/>/api/parse · /api/decks · /api/me"]

        subgraph Pipeline["5-Step Parse & Enrich Pipeline"]
            direction TB
            S1["① Rust FFI Parse<br/><i>tokenize + split sentences</i>"]
            S2["② Reconstruct Sentences<br/><i>smart punctuation joining</i>"]
            S3["③ BatchLookupForms<br/><i>dictionary fallback chain</i>"]
            S4["④ BatchLookupGlosses<br/><i>lemma → English definition</i>"]
            S5["⑤ enrichWords()<br/><i>aggregate by lemma+POS</i>"]
            S1 --> S2 --> S3 --> S4 --> S5
        end

        subgraph DictFallback["Form Resolution Chain (Step ③)"]
            direction TB
            D1["Direct Dict Lookup"]
            D2["Possessive Suffix Strip<br/><i>FI only: -nsa, -mme, …</i>"]
            D3["Compound Split<br/><i>longest-left-first</i>"]
            D4["Case Suffix Strip<br/><i>FI: -ssa, -sta / ET: -sse, -st</i>"]
            D1 -.->|not found| D2 -.->|not found| D3 -.->|not found| D4
        end
    end

    subgraph RustParser["Rust Parser Library (parser/src/lib.rs)"]
        NRM["normalize_text()<br/><i>NFC normalization</i>"]
        SPL["split_sentences()<br/><i>heuristic splitting</i>"]
        TOK["tokenize()<br/><i>separate punctuation</i>"]
        POS["guess_pos()<br/><i>heuristic POS tagging</i>"]
        NRM --> SPL --> TOK --> POS
    end

    subgraph Storage["SQLite Database"]
        direction LR
        DICT[("forms + lemmas<br/><i>Wiktionary dictionary</i>")]
        USER[("users + decks<br/>sentences + occurrence")]
        CARDS[("cards + card_state<br/><i>FSRS review — stub</i>")]
    end

    subgraph CLI["CLI Tools"]
        IMP["cmd/importdict<br/><i>kaikki.org JSONL → SQLite</i>"]
    end

    UI -->|"POST /api/parse<br/>{lang, text, parser}"| MUX
    LD -.-> UI
    MUX --> S1
    S1 -->|"CGO / FFI"| NRM
    POS -->|"JSON AnalysisResult"| S2
    S3 --> D1
    S3 & S4 --> DICT
    S5 -->|"ParseResponse JSON"| RT
    MUX -->|"/api/decks"| USER
    IMP -->|"import"| DICT
    MUX -.->|"/api/review/*<br/>(stub)"| CARDS

    style Frontend fill:#e8f4fd,stroke:#4a90d9,color:#000
    style Server fill:#f0f8e8,stroke:#5a9e4b,color:#000
    style RustParser fill:#fef3e2,stroke:#d4941a,color:#000
    style Storage fill:#f5f0ff,stroke:#8b6fc0,color:#000
    style Pipeline fill:#e8f8e8,stroke:#5a9e4b,color:#000
    style DictFallback fill:#fafff0,stroke:#5a9e4b,color:#000
    style CLI fill:#fff5f5,stroke:#c0392b,color:#000
```

## Data Flow

```mermaid
sequenceDiagram
    actor User
    participant FE as Frontend (TS)
    participant API as Go Server
    participant FFI as Rust Parser (FFI)
    participant DB as SQLite

    User->>FE: Paste text
    FE->>FE: Detect language (FI/ET)
    FE->>API: POST /api/parse {lang, text, parser}

    rect rgb(254, 243, 226)
        Note over API,FFI: Step 1 — Parse
        API->>FFI: AnalyzeText(lang, text)
        FFI->>FFI: normalize → split → tokenize → POS
        FFI-->>API: AnalysisResult JSON
    end

    rect rgb(232, 248, 232)
        Note over API,DB: Steps 2–5 — Enrich
        API->>API: Reconstruct sentences
        API->>DB: BatchLookupForms (dict → possessive → compound → case)
        DB-->>API: FormResolution map
        API->>DB: BatchLookupGlosses
        DB-->>API: Gloss map
        API->>API: enrichWords() — aggregate by lemma+POS
    end

    API-->>FE: ParseResponse {words[], total_tokens, duration}
    FE->>FE: Render sortable table + coverage score
    FE-->>User: Vocabulary list with definitions
```

## Tech Stack

| Layer | Technology | Language |
|-------|-----------|----------|
| Frontend | Vanilla SPA | TypeScript / HTML / CSS |
| Backend | HTTP server | Go 1.21+ |
| Parser | Tokenizer + POS heuristics | Rust (FFI via CGO) |
| Database | SQLite (go-sqlite3) | SQL |
| Dictionary | Wiktionary via kaikki.org | JSONL import |
| Build | Make + Cargo | Makefile |

## Directory Map

```
finnestdb/
├── cmd/
│   ├── server/main.go          # HTTP server entry point
│   └── importdict/main.go      # Dictionary import CLI
├── internal/
│   ├── api/
│   │   ├── handlers.go         # Route handlers + pipeline orchestration
│   │   └── enrichment.go       # Pure enrichWords() — CGO-free, testable
│   ├── store/
│   │   ├── db.go               # Schema, CRUD, user/deck management
│   │   └── dict.go             # Dictionary lookups + fallback chain
│   └── parserffi/
│       └── bindings.go         # CGO wrapper for Rust library
├── parser/
│   └── src/lib.rs              # Rust: tokenizer, sentence splitter, POS guesser
├── web/
│   ├── index.html              # SPA shell
│   ├── app.ts                  # Frontend logic (compiled → app.js)
│   └── styles.css              # Light + dark themes
└── Makefile                    # Build: Rust → Go → run
```

## Database Schema

```mermaid
erDiagram
    users ||--o{ decks : owns
    decks ||--o{ sentences : contains
    decks ||--o{ occurrence : tracks
    sentences ||--o{ occurrence : "token in"
    users ||--o{ cards : has
    cards ||--|| card_state : "review state"
    users ||--o{ user_known_lemmas : mastered
    users ||--o{ user_ignored_lemmas : skipped

    forms {
        text form PK
        text lang PK
        text lemma
        text pos
    }

    lemmas {
        text lemma PK
        text pos PK
        text lang PK
        text gloss
    }

    users {
        int id PK
        text email UK
        bool email_verified
        text settings_json
    }

    decks {
        int id PK
        int user_id FK
        text title
        text lang
    }

    sentences {
        int id PK
        int deck_id FK
        text text
        text lang
    }

    occurrence {
        int deck_id FK
        int sentence_id FK
        int token_ix
        text lemma
        text pos
    }

    cards {
        int id PK
        int user_id FK
        text lemma
        text pos
    }

    card_state {
        int card_id PK
        text fsrs_json
        datetime next_due
        datetime last_answer_at
    }
```

## Build Pipeline

```mermaid
graph LR
    A["make run"] --> B["make build"]
    B --> C["make parser<br/><i>cargo build --release</i>"]
    C --> D["libparser.a"]
    D --> E["go build<br/><i>CGO links Rust lib</i>"]
    E --> F["./finnestdb binary"]
    F --> G["HTTP :8080"]

    style A fill:#f9f9f9,stroke:#333,color:#000
    style D fill:#fef3e2,stroke:#d4941a,color:#000
    style F fill:#e8f4fd,stroke:#4a90d9,color:#000
    style G fill:#e8f8e8,stroke:#5a9e4b,color:#000
```
