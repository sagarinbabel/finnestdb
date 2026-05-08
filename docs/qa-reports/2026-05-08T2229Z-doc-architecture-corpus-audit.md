# Documentation and Repository State Audit

Run time: 2026-05-08T2229Z / 2026-05-09T0129+0300.

## Scope

- Fetched and pruned `origin`.
- Fast-forwarded `codex/docs-architecture-corpus-refresh` to current
  `origin/main` before editing.
- Inventoried the tracked documentation set after the fast-forward: 97
  tracked Markdown/README-style docs, 21,008 lines before this report was
  added.
- Read the full tracked doc set. Historical reports, baseline summaries,
  testdata notes, and dated spike reports were treated as append-only records.
- Updated living navigation and architecture docs instead of rewriting dated
  history.

## GitHub State

- Open PRs: none.
- Recently merged cleanup/docs/corpus PRs confirmed:
  - #174 `claude/user-friendly-wordlist` merged 2026-05-08T22:17:05Z.
  - #173 `claude/meaning-sources-research` merged 2026-05-08T22:12:20Z.
  - #172 `unified-nlp-venv` merged 2026-05-08T21:39:52Z.
  - #171 `fix/tokenize-low-single-quote` merged 2026-05-08T21:16:47Z.
  - #170 `codex/sentence-export-epub-cleanup` merged 2026-05-08T21:25:41Z.
  - #166 `codex/compress-parser-eval-gold-fixtures` merged 2026-05-08T22:25:25Z.
  - #165 `codex/slim-baseline-json-artifacts` merged 2026-05-08T22:23:46Z.

## Local State

- Current work branch: `codex/docs-architecture-corpus-refresh`.
- Pre-existing or parallel local code changes preserved and not edited:
  `corpus_pipeline/internal/fetcher/fetcher.go`.
- Additional corpus extractor work visible in the local tree and preserved:
  `corpus_pipeline/cmd/extractcorpus/extract_misc.go` and untracked
  `corpus_pipeline/cmd/extractcorpus/extract_wiki.go`.
- Pre-existing untracked design prototypes preserved and not edited:
  `design/*`.
- Branch pruning was performed after explicit user approval. Deletion was
  limited to branches already merged into `origin/main` by ancestry plus the
  confirmed squash-merged PR #170 branch.

## Branch Pruning Result

Deleted:

- 89 local branches merged into `origin/main` by ancestry.
- 63 remote branches merged into `origin/main` by ancestry.
- The squash-merged PR #170 branch, `codex/sentence-export-epub-cleanup`,
  from both local and remote refs.

Skipped:

- 16 local branches merged by ancestry but currently checked out in other
  worktrees.
- 15 remote branches merged by ancestry whose matching local branches are
  checked out in other worktrees.
- 7 local branches with `upstream: gone` because they still have unique commits
  or are checked out by a worktree:
  - `claude/jovial-mirzakhani-0a8da4`
  - `claude/laughing-chaum-68ab35`
  - `claude/plan-c-pr2-baseline-report`
  - `codex/dictionary-source-priority`
  - `codex/finnish-gold-expansion`
  - `codex/pr4-known-words-global-cards`
  - `integration-test-2026-05-07`

Remaining branch counts after pruning and `git fetch --prune origin`:

- Local branches merged into `origin/main` by ancestry, excluding `main` and
  the current docs branch: 16.
- Local branches not merged into `origin/main` by ancestry, excluding the
  current docs branch: 48.
- Remote tracking refs merged into `origin/main` by ancestry, excluding
  `origin/HEAD` and `origin/main`: 15.

Recommended preview commands before deleting anything:

```sh
git branch --merged origin/main --format='%(refname:short)' \
  | sed '/^main$/d;/^codex\/docs-architecture-corpus-refresh$/d'

git branch -vv | awk '/: gone]/{print $1}'

git branch -r --merged origin/main --format='%(refname:short)' \
  | sed '/origin\/HEAD/d;/origin\/main/d'
```

## Documentation Findings Resolved

- `ARCHITECTURE.md` still treated the browser as primarily a workbench and
  did not model `corpus_pipeline/` as a first-class subsystem. It now does.
- The architecture Mermaid diagram omitted corpus fetch/extract/aggregate/
  verify/enrich/EPUB deck generation and localdata artifact boundaries. It
  now includes those paths.
- `corpus_pipeline/docs/CORPUS_PIPELINE.md` still described built tools such
  as `enrichcorpus`, `epubdeck`, and many extractors as future work. It now
  separates built capabilities from deferred trigger-gated work.
- `corpus_pipeline/docs/PR_ROADMAP.md` and `corpus_pipeline/v2plan.md`
  still marked meaning sources, user-friendly wordlist, and sentence export
  work as active/planned after PRs #170, #173, and #174 had merged. They now
  mark those items done and leave interlinear glossing as the later spike.
- `docs/FST_LEMMATIZER.md` still said the ET generated-table command was not
  present. It now documents `make gen-lemmatizer-tables-et HFSTOL_PATH=...`.
- `docs/INDEX.md`, `README.md`, and `TODO.md` had stale navigation/status
  around corpus pipeline and PR state. They now point to the current surfaces.
- `finnestdb-prd-alpha.md` called itself historical but still had a stale
  parse-only "Current State on main" section. It now has a dated
  implementation snapshot and directs readers to README/ARCHITECTURE for
  current truth.

## Append-Only Policy Applied

The following classes were read but not rewritten:

- `docs/qa-reports/*`
- `reports/parser-eval/*`
- `experiments/*`
- `docs/baselines/*` frozen reports and summaries
- `testdata/parser-eval/**/README.md`
- dated corpus reports and handoff notes

Future changes should keep that distinction: living navigation and
architecture docs can be edited; dated reports and baselines should get new
timestamped records.
