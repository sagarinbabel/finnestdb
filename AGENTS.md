# Agent Instructions

These instructions are for LLM agents working in this repository.

## Current Focus

FinEstDB is pre-go-live. Prioritize work that helps ship and operate the
language-learning app:

- parser accuracy and dictionary-entry attachment
- learner flows: inspect, decks, review, known words, parser feedback
- production safety: auth/session hardening, abuse controls, data retention
- evaluation, baselines, and regression checks for parser changes

## Autoresearch Is Parked

`cmd/autoresearch` and `docs/AUTORESEARCH.md` are a parked idea for after the
app is shipped and live. Treat all autoresearch references as "future ideas to
remember", not current implementation work.

Do not focus on autoresearch, fix it, expand it, review PRs around it, or block
unrelated work because of it unless the user explicitly asks for autoresearch in
the current turn.

If a PR touches `cmd/autoresearch` incidentally, review only for compile/test
breakage. Do not make it a product-quality requirement.

