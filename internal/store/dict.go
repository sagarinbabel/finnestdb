package store

import (
	"database/sql"
	"strings"

	"finnestdb/internal/parserules"
)

// FormResolution holds the result of resolving a surface form to its canonical
// lemma and POS, along with metadata about how it was resolved.
//
//	Source values: "dict", "possessive", "compound", "case_suffix"
//	GrammarLabel: e.g. "inessive" from case suffix stripping; empty for direct dict hits
type FormResolution struct {
	Lemma        string
	POS          string
	GrammarLabel string // e.g. "inessive", empty for direct dict hits
	Source       string // how this resolution was found
}

// Suffix tables (FinnishPossessiveSuffixes, FinnishCaseSuffixes,
// EstonianCaseSuffixes) live in the parserules package — see
// internal/parserules/ for the data and how to extend it.

// BatchLookupForms resolves a slice of surface forms to their canonical
// (lemma, pos) pairs for the given language.
//
// Fallback chain (each step only runs if previous steps missed):
//
//	Step 1: Direct dictionary lookup (lowercase form in forms table)
//	Step 2: [FI only] Possessive suffix strip → re-lookup in forms table
//	Step 3: [FI + ET] Compound split → both halves in forms table
//	Step 4: [FI + ET] Case suffix strip → stem lookup in lemmas table
//	Step 5: Not resolved → absent from map → caller falls back to stub
//
// Returns a map from form → FormResolution.
// Forms that cannot be resolved are absent from the map.
func (d *DB) BatchLookupForms(forms []string, lang string, parserMode string) map[string]FormResolution {
	result := make(map[string]FormResolution, len(forms))
	if len(forms) == 0 {
		return result
	}

	// Pre-prepare both statements once — passed to all callees.
	stmtForms, err := d.db.Prepare(`SELECT lemma, pos FROM forms WHERE form = ? AND lang = ?`)
	if err != nil {
		return result
	}
	defer stmtForms.Close()

	stmtLemmas, err := d.db.Prepare(`SELECT lemma, pos FROM lemmas WHERE lemma = ? AND lang = ?`)
	if err != nil {
		return result
	}
	defer stmtLemmas.Close()

	for _, form := range forms {
		// Dictionary rows are stored in lowercase (kaikki.org normalization).
		// Normalize the lookup key so sentence-initial capitals ("Kirjassa")
		// and proper nouns resolve correctly.
		lower := strings.ToLower(form)

		// Step 1: Direct dictionary lookup.
		var lemma, pos string
		if err := stmtForms.QueryRow(lower, lang).Scan(&lemma, &pos); err == nil {
			result[form] = FormResolution{Lemma: lemma, POS: pos, Source: "dict"}
			continue
		}

		// Steps 2-4 only run in "custom" parser mode.
		if parserMode != "custom" {
			continue
		}

		// Step 2: Finnish possessive suffix fallback.
		if lang == "FI" {
			if resolved, ok := tryStripPossessive(stmtForms, lower, lang); ok {
				result[form] = resolved
				continue
			}
		}

		// Step 3: Compound word splitting (FI + ET).
		if resolved, ok := tryCompoundSplit(stmtForms, lower, lang); ok {
			result[form] = resolved
			continue
		}

		// Step 4: Case suffix stripping (FI + ET).
		suffixes := parserules.FinnishCaseSuffixes
		if lang == "ET" {
			suffixes = parserules.EstonianCaseSuffixes
		}
		if resolved, ok := tryCaseSuffixStrip(stmtLemmas, lower, lang, suffixes); ok {
			result[form] = resolved
			continue
		}
	}
	return result
}

// tryStripPossessive attempts to resolve a Finnish surface form by stripping
// possessive suffixes and re-looking up the stripped form. Returns the resolved
// FormResolution and true if a dictionary match is found after stripping.
//
// form must already be lowercased — BatchLookupForms normalises before calling.
func tryStripPossessive(stmt *sql.Stmt, form, lang string) (FormResolution, bool) {
	for _, suffix := range parserules.FinnishPossessiveSuffixes {
		if !strings.HasSuffix(form, suffix) {
			continue
		}
		stripped := form[:len(form)-len(suffix)]
		if len(stripped) < 2 { // sanity: skip 1-char candidates
			continue
		}
		var lemma, pos string
		if err := stmt.QueryRow(stripped, lang).Scan(&lemma, &pos); err == nil {
			return FormResolution{Lemma: lemma, POS: pos, Source: "possessive"}, true
		}
	}
	return FormResolution{}, false
}

// ─── Compound word splitting ────────────────────────────────────────────────

// tryCompoundSplit attempts a longest-left-first binary split of a form.
// Both halves must exist in the forms table. The right component determines POS
// (Finnish/Estonian compounds have their head on the right).
//
// Guards:
//   - Form length: 6–30 runes (too short = false positives, too long = unlikely)
//   - Min part length: 3 runes (avoids false matches on short words like "ei", "on")
//   - Rune-based iteration: safe for multi-byte Finnish chars (ä, ö, ü)
func tryCompoundSplit(stmtForms *sql.Stmt, form, lang string) (FormResolution, bool) {
	runes := []rune(form)
	n := len(runes)
	if n < 6 || n > 30 {
		return FormResolution{}, false
	}

	// Try split points from longest-left to shortest-left.
	for i := n - 3; i >= 3; i-- {
		left := string(runes[:i])
		right := string(runes[i:])

		var leftLemma string
		var leftPOS string
		if err := stmtForms.QueryRow(left, lang).Scan(&leftLemma, &leftPOS); err != nil {
			continue
		}
		var rightLemma, rightPOS string
		if err := stmtForms.QueryRow(right, lang).Scan(&rightLemma, &rightPOS); err != nil {
			continue
		}
		return FormResolution{
			Lemma:  leftLemma + rightLemma,
			POS:    rightPOS, // head component determines POS
			Source: "compound",
		}, true
	}
	return FormResolution{}, false
}

// ─── Case suffix stripping ──────────────────────────────────────────────────

// tryCaseSuffixStrip strips case suffixes and validates the stem against the
// lemmas table (stricter than forms — reduces false positives from short suffixes).
//
// form must already be lowercased. The suffixes table is provided by the
// caller from the parserules package.
func tryCaseSuffixStrip(stmtLemmas *sql.Stmt, form, lang string, suffixes []parserules.CaseSuffix) (FormResolution, bool) {
	for _, cs := range suffixes {
		if !strings.HasSuffix(form, cs.Suffix) {
			continue
		}
		stem := form[:len(form)-len(cs.Suffix)]
		if len(stem) < 3 { // min stem length to avoid false positives
			continue
		}
		var lemma, pos string
		if err := stmtLemmas.QueryRow(stem, lang).Scan(&lemma, &pos); err == nil {
			return FormResolution{Lemma: lemma, POS: pos, GrammarLabel: cs.Label, Source: "case_suffix"}, true
		}
	}
	return FormResolution{}, false
}

// LemmaKey identifies a unique (lemma, pos) pair for use as a map key.
type LemmaKey struct {
	Lemma string
	POS   string
}

// BatchLookupGlosses resolves a slice of LemmaKeys to their English gloss strings.
// Returns a map from LemmaKey → gloss. Keys with no entry in the lemmas table
// are absent from the map (caller treats absent as empty gloss).
func (d *DB) BatchLookupGlosses(lemmas []LemmaKey, lang string) map[LemmaKey]string {
	result := make(map[LemmaKey]string, len(lemmas))
	if len(lemmas) == 0 {
		return result
	}

	stmt, err := d.db.Prepare(
		`SELECT COALESCE(gloss, '') FROM lemmas WHERE lemma = ? AND pos = ? AND lang = ?`,
	)
	if err != nil {
		return result
	}
	defer stmt.Close()

	for _, k := range lemmas {
		var gloss string
		if err := stmt.QueryRow(k.Lemma, k.POS, lang).Scan(&gloss); err == nil && gloss != "" {
			result[k] = gloss
		}
	}
	return result
}
