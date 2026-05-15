# Swift Offline Migration Plan

_Created 2026-05-15. Planning baseline: `main` after fast-forward to
`origin/main`._

## Summary

Port FinEstDB from the current Go/Rust/SQLite web app into native Swift apps
for iOS and macOS, with full parity for learner and admin workflows. The app
must work offline by default, using local profiles instead of server login.
Parser feedback queues locally and syncs later when an online backend exists.

Baseline facts from `main`:

- Runtime today: Go API + SQLite + Rust tokenizer/parser FFI + TypeScript
  browser UI.
- Local dictionary: `finnestdb.db` is about 4.9 GB, with about 33.1M `forms`,
  615k `lemmas`, 186k `translations`, and 320k `definitions`.
- Morphology tables: FI about 55 MB and ET about 76 MB JSON under
  `localdata/lemmatizer-fi-et/tables/`.
- Current product surface: Inspect, Results, Decks, Review, Known Words,
  parser feedback, admin users, admin parser workbench, and admin feedback
  review.

## Key Changes

- Start implementation from `main`; do not base the migration on a feature
  branch.
- Create a shared native-core strategy with Chickendude:
  - Required: shared versioned data-pack schema, parser fixtures, parser
    baselines, and correction/export formats.
  - Default: shared Rust tokenizer/parser core exposed to Swift and Android
    through FFI unless Chickendude objects.
  - Native Swift and Android own their UI, local profile handling, app storage,
    import UX, and platform integration.
- Build a Swift package split into:
  - `FinEstCore`: parser contracts, deck/review models, learning-state logic,
    import extraction interfaces.
  - `FinEstData`: SQLite access, migrations, dictionary-pack mounting, user
    profile DB.
  - `FinEstParser`: Rust FFI wrapper plus Swift result mapping.
  - `FinEstApp`: SwiftUI iOS/macOS shell.
  - `FinEstAdmin`: admin workbench, feedback queue, eval/report views inside
    the app behind local admin mode.
- Replace server auth with local profiles:
  - Local profile table replaces `users`, `user_sessions`, cookie auth, and
    server roles.
  - First local profile is a normal user by default.
  - Admin mode is a local profile flag gated by local device authentication or
    an explicit admin unlock setting.
  - No network is required to inspect, create decks, review, manage known words,
    or use admin tools.
- Preserve current data concepts:
  - Keep dictionary tables equivalent to `lemmas`, `forms`, `translations`,
    `definitions`, and `dict_metadata`.
  - Keep learner tables equivalent to `decks`, `sentences`, `occurrence`,
    `cards`, `card_state`, `user_known_lemmas`, `user_ignored_lemmas`,
    `parse_sessions`, and `parse_feedback`.
  - Convert server-only request/response structs into Swift domain models
    matching the current API contract where practical.
- Parser behavior:
  - Port `parsecore` semantics into Swift around the shared Rust tokenizer.
  - Keep `basic` and `custom` parser modes.
  - Keep FI/ET language validation and the 1,500,000 character cap unless
    product later lowers it for mobile memory.
  - Keep sentence/chapter parse shape for EPUB imports.
  - Keep parser versioning and baseline IDs in app-visible metadata.
- Data-pack delivery scenarios:
  - Recommended: downloadable FI/ET packs. App Store binary stays small; packs
    are versioned, checksum-verified, resumable, deletable, and independently
    updateable.
  - Bundle all data: highest first-launch offline reliability, but likely
    impractical due multi-GB app size and App Store review/update friction.
  - Bundle lite data: include a starter dictionary and prompt for full FI/ET
    packs for high-quality offline parsing.
  - Implement the app so all three remain possible, but make downloadable packs
    the default architecture.
- Offline feedback and sync-later:
  - Parser feedback is stored locally with source text, parser version,
    data-pack version, and local profile ID.
  - Admin review mutates the local queue.
  - Add an export format for reviewed feedback and corrections so a later sync
    service can ingest them.
  - Do not build live sync in the first port; design the schema with stable IDs
    and timestamps so sync can be added without rewriting local data.

## Implementation Phases

1. **Extraction and contracts**
   - Freeze Swift/Android shared contracts: parser JSON shape, SQLite data-pack
     schema, profile DB schema, correction bundle format, fixture format.
   - Add a data-pack build pipeline from the existing `finnestdb.db` and FST
     tables that emits signed/checksummed packs.
   - Add cross-platform parser fixture tests from current Go parser baselines.

2. **Native core**
   - Build Rust parser as an XCFramework-compatible static or dynamic library
     for iOS device, simulator, and macOS.
   - Implement Swift parser pipeline equivalent to `parsecore`: tokenize, batch
     lookup, gloss lookup, enrichment, stats, words, sentences, chapters.
   - Implement SQLite access with prepared statements and indexes tuned for
     mobile read-heavy dictionary lookup.
   - Decide whether FST tables stay JSON initially or are repacked into SQLite
     or binary tables for faster mobile startup; default to repacking if JSON
     parse cost is high.

3. **Learner app**
   - Build SwiftUI Inspect, Results, Decks, Deck Detail, Review, Known Words,
     import, and dashboard screens.
   - Support `.txt`, `.md`, and `.epub` imports locally.
   - Preserve existing deck creation semantics: parse text, store sentences and
     occurrences, create global cards by `(profile, lang, lemma, pos)`.
   - Preserve current review scheduler behavior first; defer FSRS upgrade unless
     it is already implemented before migration starts.

4. **Admin parity**
   - Build local admin workbench with `basic`/`custom` parser selection, timings,
     source counts, traces where available, and parser version metadata.
   - Build parser feedback queue with status filters and local review actions.
   - Build admin user/profile management as local profile administration, not
     server account management.
   - Include local eval runner for bundled parser fixtures and baseline
     comparisons.

5. **Android coordination**
   - Chickendude consumes the same data packs, parser fixtures, correction
     bundle format, and Rust core contract.
   - Swift and Android publish parser outputs against the same fixture suite
     before either app changes parser behavior.
   - Any parser-behavior change requires a shared baseline update, not
     independent native tweaks.

## Test Plan

- Unit tests:
  - Swift parser output matches current Go/Rust fixture JSON for FI and ET.
  - Dictionary lookup resolves multi-lemma forms like ET homonyms.
  - Known/ignored state and card identity stay scoped by local profile and
    language.
  - Review answer updates `card_state` exactly like current backend behavior.
  - Feedback queue supports submit, filter, review, and export.
- Integration tests:
  - Fresh install with no data pack shows offline-limited state and can install
    a pack.
  - Full FI/ET packs mount correctly and report `dict_metadata`.
  - Inspect parses pasted text and EPUB chapters offline.
  - Create deck from parse, review a due card, mark known/ignored, reopen app,
    state persists.
  - Admin workbench parse output matches learner parse output for the same
    parser/data-pack version.
- Cross-platform acceptance:
  - Swift and Android produce byte-equivalent parser fixture outputs where the
    contract requires exactness.
  - Differences must be recorded as explicit platform exceptions before release.
  - Data-pack checksums and schema versions are identical across Swift and
    Android.
- Performance checks:
  - Cold launch with no pack.
  - Pack mount time.
  - First parse after launch.
  - Parse latency for small paste, article-length text, and max-size text.
  - SQLite lookup throughput for large unique-form sets.
  - Memory use on lowest supported iPhone/iPad/Mac targets.

## Assumptions

- `main` is the source branch for migration.
- Full parity means learner flows plus admin workbench, admin feedback review,
  and local profile administration.
- Offline-first means no server dependency for core use.
- Local profiles replace current password/session auth in the native app.
- Feedback sync is designed for later but not implemented in v1.
- Downloadable data packs are the recommended path, while bundle-all and
  bundle-lite remain supported packaging scenarios.
- Shared artifacts are mandatory with Android; shared Rust core is the default
  unless Chickendude rejects it.
