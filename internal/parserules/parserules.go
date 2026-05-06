// Package parserules holds all the language-specific data tables used by the
// custom parser's enrichment chain (possessive stripping, compound splitting,
// case-suffix stripping).
//
// All rules are pure data — no DB access, no I/O. The functions in
// internal/store/dict.go consume these tables when resolving surface forms.
//
// File layout:
//
//	parserules.go  — shared types
//	finnish.go     — Finnish (FI) suffix tables
//	estonian.go    — Estonian (ET) suffix tables
//
// To extend the parser with a new rule:
//
//  1. Add an entry to the relevant table in finnish.go or estonian.go.
//  2. Run `go test ./...` and `go run ./cmd/parsertest -dataset … -parsers custom`.
//  3. Diff the new report against the matching baseline in docs/baselines/.
//  4. If accuracy improved with no regressions, commit the change.
//
// This is the file an agent (or a human) edits when iterating on parser rules.
// It deliberately has zero coupling to DB, parser-mode plumbing, or eval code.
package parserules

// CaseSuffix is a single case-marking suffix paired with the human-readable
// grammar label that identifies it.
//
// Tables of CaseSuffix entries must be ordered longest-first so that the
// stripping loop tries "ssaan" before "ssa" before "n", etc.
type CaseSuffix struct {
	Suffix string
	Label  string
}
