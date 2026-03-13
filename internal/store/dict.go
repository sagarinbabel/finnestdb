package store

import (
	"database/sql"
	"strings"
)

// finnishPossessiveSuffixes lists Finnish possessive suffixes to try when a
// surface form is not found directly in the forms table. Ordered longest-first
// to avoid partial matches (e.g. try -nsa before -sa).
//
// Stripping logic:
//   form → strip suffix → re-lookup stripped form in forms table
//   Accept only if the stripped form IS in the table (dictionary-validated).
//   E.g. "kirjassani" - strip "-ni" → "kirjassa" → found → lemma: kirja ✓
//        "talo" - try strip "-si" → "tal" → not found → reject ✗
var finnishPossessiveSuffixes = []string{
	"nsa", "nsä", // 3rd person (any number): "kirjansa"
	"mme",        // 1st person plural: "kirjamme"
	"nne",        // 2nd person plural: "kirjanne"
	"ni",         // 1st person singular: "kirjani"
	"si",         // 2nd person singular: "kirjasi"
}

// BatchLookupForms resolves a slice of surface forms to their canonical
// (lemma, pos) pairs for the given language.
//
// For Finnish forms not found directly in the forms table, possessive suffixes
// are stripped and the lookup is retried. Estonian does not use this fallback.
//
// Returns a map from form → [2]string{lemma, pos}.
// Forms that cannot be resolved are absent from the map (caller falls back to stub).
func (d *DB) BatchLookupForms(forms []string, lang string) map[string][2]string {
	result := make(map[string][2]string, len(forms))
	if len(forms) == 0 {
		return result
	}

	stmt, err := d.db.Prepare(`SELECT lemma, pos FROM forms WHERE form = ? AND lang = ?`)
	if err != nil {
		return result
	}
	defer stmt.Close()

	for _, form := range forms {
		var lemma, pos string
		if err := stmt.QueryRow(form, lang).Scan(&lemma, &pos); err == nil {
			result[form] = [2]string{lemma, pos}
			continue
		}
		// Finnish possessive suffix fallback.
		if lang == "FI" {
			if resolved, ok := tryStripPossessive(stmt, form, lang); ok {
				result[form] = resolved
			}
		}
	}
	return result
}

// tryStripPossessive attempts to resolve a Finnish surface form by stripping
// possessive suffixes and re-looking up the stripped form. Returns the resolved
// (lemma, pos) and true if a dictionary match is found after stripping.
func tryStripPossessive(stmt *sql.Stmt, form, lang string) ([2]string, bool) {
	lower := strings.ToLower(form)
	for _, suffix := range finnishPossessiveSuffixes {
		if !strings.HasSuffix(lower, suffix) {
			continue
		}
		stripped := form[:len(form)-len(suffix)]
		if len(stripped) < 2 { // sanity: skip 1-char candidates
			continue
		}
		var lemma, pos string
		if err := stmt.QueryRow(stripped, lang).Scan(&lemma, &pos); err == nil {
			return [2]string{lemma, pos}, true
		}
	}
	return [2]string{}, false
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
