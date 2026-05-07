package store

import (
	"database/sql"
	"sort"
	"strings"
	"unicode"

	"finnestdb/internal/parserules"
	lemmatizer "finnestdb/pkg/lemmatizer-fi-et"
)

// FormResolution holds the result of resolving a surface form to its canonical
// lemma and POS, along with metadata about how it was resolved.
//
//	Source values: "dict", "possessive", "compound", "case_suffix"
//	GrammarLabel: legacy single-attribute label (e.g. "inessive"); empty for
//	  direct dict hits unless attached by stopgap or projected from Feats
//	Feats: full UD FEATS string for the form (e.g.
//	  "Case=Ine|Number=Sing"). Currently populated for ET via
//	  cmd/importekilexdetails::ekilexMorphToFeats from Ekilex morph_codes.
//	  When non-empty, takes precedence over GrammarLabel for downstream
//	  display; GrammarLabel is back-projected from Feats' Case= attribute
//	  for back-compat with the case-only metric.
type FormResolution struct {
	Lemma        string
	POS          string
	GrammarLabel string // e.g. "inessive", empty when no case info available
	Feats        string // UD FEATS string, e.g. "Case=Ine|Number=Sing"
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
//	Step 4: [FI + ET] Case suffix strip → lemma-candidate lookup in lemmas table
//	Step 5: [FI only] VFST morphological analysis (libvoikko mor.vfst)
//	Step 6: Not resolved → absent from map → caller falls back to stub
//
// In "custom" mode, after step 1 succeeds we lazily attach a GrammarLabel
// (case name) to the dict result when the dict didn't supply a label/FEATS:
// the FST is consulted first (FI/ET only, gated on best.GrammarLabel == ""
// AND best.Feats == ""), and the case-suffix matcher acts as a fallback when
// the FST didn't fire, didn't agree on the lemma, or returned an empty label.
// The forms table only stores (lemma, pos) for many entries, so without this
// pass every dict hit without FEATS would carry an empty grammar label and
// grammar-accuracy would be 0% on those tokens. This is a stopgap until the
// FST runtime (PRs #106/#107) emits FEATS natively — see docs/DECISIONS.md
// for the rationale on not extending the suffix table further.
//
// Returns a map from form → FormResolution.
// Forms that cannot be resolved are absent from the map.
func (d *DB) BatchLookupForms(forms []string, lang string, parserMode string) map[string]FormResolution {
	result := make(map[string]FormResolution, len(forms))
	if len(forms) == 0 {
		return result
	}

	// Step 1 query fetches all candidates with source metadata so the picker
	// can rank them. Steps 2–4 remain single-row lookups because their
	// fallback paths commit to one resolution by construction.
	stmtFormsAll, err := d.db.Prepare(`SELECT lemma, pos, source, source_priority, COALESCE(feats, '') FROM forms WHERE form = ? AND lang = ?`)
	if err != nil {
		return result
	}
	defer stmtFormsAll.Close()

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

		// Step 1: Direct dictionary lookup. Multi-lemma rows mean a single
		// surface form can have multiple (lemma, pos) candidates; rank them
		// against the original-case surface so PROPN homonyms don't beat
		// common-noun lemmas on lowercase surfaces.
		if best, ok := lookupBestForm(stmtFormsAll, form, lower, lang); ok {
			// Lazy attach inside the dict-hit branch when the dict didn't
			// supply a label/FEATS. Two paths, in priority order:
			//   1. FST analysis (FI/ET only) — only run when both
			//      best.GrammarLabel == "" AND best.Feats == "" so we
			//      don't pay the per-token FST cost when the dict already
			//      back-projected a label from FEATS or carries any FEATS
			//      string at all (a labeled FEATS path is preferable to
			//      an FST guess). The lemma-equality guard prevents
			//      attaching a label that belongs to a different word.
			//   2. Case-suffix stopgap — only when FST didn't fire,
			//      didn't agree on the lemma, or returned an empty label.
			if parserMode == "custom" && best.GrammarLabel == "" {
				attached := false
				if best.Feats == "" && (lang == "FI" || lang == "ET") {
					if lem := d.fstLemmatizer(); lem != nil {
						if a, ok := pickFirstFSTAnalysis(lem.Lemmatize(lang, lower)); ok {
							if a.Lemma == best.Lemma && a.GrammarLabel != "" {
								best.GrammarLabel = a.GrammarLabel
								best.Source = best.Source + "+fst_label"
								attached = true
							}
						}
					}
				}
				if !attached {
					best = attachCaseLabelIfStemMatches(stmtLemmas, best, lower, lang)
				}
			}
			result[form] = best
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

		// Step 5: FST morphological analysis (FI + ET). Catches forms the
		// SQLite-driven steps couldn't resolve — e.g. less-common
		// derivations, compounds whose halves aren't both in the forms
		// table, and rare inflected forms whose lemmas aren't in the
		// lemmas table. For FI this hits both libvoikko (VFST) and
		// Giellalt (HFSTOL) with VFST-priority on overlap; for ET it's
		// Giellalt-only (Voikko has no Estonian model).
		if lang == "FI" || lang == "ET" {
			if lem := d.fstLemmatizer(); lem != nil {
				if resolved, ok := tryFSTAnalyze(lem, lang, lower); ok {
					result[form] = resolved
					continue
				}
			}
		}
	}
	return result
}

// tryFSTAnalyze runs the unified Lemmatizer against lower for the
// given lang and returns the highest-priority FormResolution if any
// analysis exists. The merge logic (VFST > Giellalt for FI; Giellalt
// only for ET; non-compound > compound) lives in pkg/lemmatizer-fi-et;
// this helper just picks the first valid analysis it returns.
func tryFSTAnalyze(lem *lemmatizer.Lemmatizer, lang, lower string) (FormResolution, bool) {
	a, ok := pickFirstFSTAnalysis(lem.Lemmatize(lang, lower))
	if !ok {
		return FormResolution{}, false
	}
	return FormResolution{
		Lemma:        a.Lemma,
		POS:          a.UPOS,
		GrammarLabel: a.GrammarLabel,
		Feats:        "",
		Source:       "fst",
	}, true
}

// pickFirstFSTAnalysis returns the best analysis with non-empty
// (Lemma, UPOS) — preferring any analysis that also carries a non-empty
// GrammarLabel, falling back to the first valid analysis when none does.
// Used both by tryFSTAnalyze (when dict missed) and by the lazy
// label-attach path in BatchLookupForms (when dict hit and we want to
// attach a grammar_label). The label-preference protects against
// dropping a labeled reading when an unlabeled one happens to come first
// in Lemmatize's output ordering.
func pickFirstFSTAnalysis(analyses []lemmatizer.Analysis) (lemmatizer.Analysis, bool) {
	var firstValid lemmatizer.Analysis
	found := false
	for _, a := range analyses {
		if a.Lemma == "" || a.UPOS == "" {
			continue
		}
		if a.GrammarLabel != "" {
			return a, true
		}
		if !found {
			firstValid, found = a, true
		}
	}
	return firstValid, found
}

// pickBestVFSTAnalysis ranks VFST analyses against the original surface
// using the same case/POS scoring as the dictionary path, plus a strict
// initial-case match so capitalized surfaces ("Turussa") prefer lemmas
// that also start uppercase ("Turku") over lowercase homonyms ("turku").
// Ties fall back to alphabetic lemma order — same chain as
// pickBestFormCandidate — so behavior matches the dict path exactly.
func pickBestVFSTAnalysis(surface string, analyses []lemmatizer.Analysis) lemmatizer.Analysis {
	if len(analyses) == 1 {
		return analyses[0]
	}
	scored := make([]lemmatizer.Analysis, len(analyses))
	copy(scored, analyses)
	sort.SliceStable(scored, func(i, j int) bool {
		ai, aj := scored[i], scored[j]
		aiCase, aiPOS := caseMatchScore(surface, ai.Lemma), posSanityScore(surface, ai.UPOS)
		ajCase, ajPOS := caseMatchScore(surface, aj.Lemma), posSanityScore(surface, aj.UPOS)
		if aiCase != ajCase {
			return aiCase > ajCase
		}
		if aiPOS != ajPOS {
			return aiPOS > ajPOS
		}
		aiStrict, ajStrict := strictCaseMatchScore(surface, ai.Lemma), strictCaseMatchScore(surface, aj.Lemma)
		if aiStrict != ajStrict {
			return aiStrict > ajStrict
		}
		if ai.Lemma != aj.Lemma {
			return ai.Lemma < aj.Lemma
		}
		return ai.UPOS < aj.UPOS
	})
	return scored[0]
}

// strictCaseMatchScore is a stricter complement to caseMatchScore: it
// returns 1 only when surface and lemma share the same uppercase/lowercase
// initial. This separates "Turussa" + "Turku" (1) from "Turussa" + "turku"
// (0), which caseMatchScore intentionally does not distinguish.
func strictCaseMatchScore(surface, lemma string) int {
	if startsUpper(surface) == startsUpper(lemma) {
		return 1
	}
	return 0
}

// formCandidate is the internal shape used while ranking multi-lemma matches.
// Source / SourcePriority come from the row-level dictionary metadata; older
// rows that pre-date the source-priority work have empty strings and zero
// priority, which is fine — the case-match and POS heuristics still rank them.
// Feats is the per-form UD FEATS string (e.g. "Case=Ine|Number=Sing"); empty
// for rows imported from sources without per-form morphology (e.g.
// kaikki.org). Populated for ET via Ekilex morph_codes (PR Plan-C/4).
type formCandidate struct {
	Lemma          string
	POS            string
	Source         string
	SourcePriority int
	Feats          string
}

// lookupBestForm runs the multi-row form lookup and ranks the candidates.
// Returns the best FormResolution and true if any candidate exists, else
// false. The original (uncased) surface is needed for case-match scoring.
func lookupBestForm(stmt *sql.Stmt, surface, lowerSurface, lang string) (FormResolution, bool) {
	rows, err := stmt.Query(lowerSurface, lang)
	if err != nil {
		return FormResolution{}, false
	}
	defer rows.Close()

	var candidates []formCandidate
	for rows.Next() {
		var c formCandidate
		if err := rows.Scan(&c.Lemma, &c.POS, &c.Source, &c.SourcePriority, &c.Feats); err != nil {
			return FormResolution{}, false
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return FormResolution{}, false
	}

	best := pickBestFormCandidate(surface, candidates)
	res := FormResolution{Lemma: best.Lemma, POS: best.POS, Feats: best.Feats, Source: "dict"}
	// Project Case= from Feats to GrammarLabel for back-compat with the
	// case-only metric. New code should consume Feats directly; the
	// projection lets the existing eval keep working unchanged.
	if best.Feats != "" {
		res.GrammarLabel = caseFromFeats(best.Feats)
	}
	return res, true
}

// caseFromFeats extracts the Case= attribute from a UD FEATS string and
// returns the lowercase English case name used by our existing
// grammar_label vocabulary. Returns "" when FEATS has no Case= attribute
// or when the value is Nominative (left implicit per existing convention,
// matching cmd/importud/main.go).
func caseFromFeats(feats string) string {
	if feats == "" {
		return ""
	}
	for _, pair := range strings.Split(feats, "|") {
		eq := strings.Index(pair, "=")
		if eq < 0 || pair[:eq] != "Case" {
			continue
		}
		val := pair[eq+1:]
		if val == "Nom" {
			return "" // nominative implicit
		}
		if label, ok := udCaseToLegacyLabel[val]; ok {
			return label
		}
		return ""
	}
	return ""
}

// udCaseToLegacyLabel maps UD Case= values to the lowercase English case
// names used in our existing gold sets and grammar_label vocabulary.
// Mirrors cmd/importud/main.go::udCaseToLabel — kept here separately so
// the dict layer doesn't import a cmd/ package.
var udCaseToLegacyLabel = map[string]string{
	"Gen": "genitive",
	"Par": "partitive",
	"Ill": "illative",
	"Ine": "inessive",
	"Ela": "elative",
	"All": "allative",
	"Ade": "adessive",
	"Abl": "ablative",
	"Ess": "essive",
	"Tra": "translative",
	"Ins": "instructive",
	"Abe": "abessive",
	"Com": "comitative",
	"Ter": "terminative",
	"Acc": "accusative",
	"Voc": "vocative",
}

// pickBestFormCandidate ranks candidates for a surface form. Ranking, higher
// to lower priority:
//
//  1. Case-match: a lowercase surface prefers a candidate whose lemma starts
//     lowercase. (Place names that share an inflected form with a common noun
//     should not win when the surface gives no capitalization signal.)
//  2. POS sanity: a lowercase surface demotes PROPN. Same motivation.
//  3. Source priority: higher dictionary `source_priority` wins among
//     candidates that pass the case/POS filters equally.
//  4. Deterministic tiebreak: source name asc, lemma asc, POS asc — same
//     candidate set always returns the same pick.
//
// The heuristic deliberately under-claims: it will not turn `naeris`/NOUN
// (turnip) vs `naerma`/VERB (laughed) into the right choice without context,
// because that's genuinely ambiguous. It does fix the dominant homonym
// regression (lowercase surface → PROPN homonym arbitrarily wins).
func pickBestFormCandidate(surface string, candidates []formCandidate) formCandidate {
	if len(candidates) == 1 {
		return candidates[0]
	}
	scored := make([]formCandidate, len(candidates))
	copy(scored, candidates)
	sort.SliceStable(scored, func(i, j int) bool {
		ci, cj := scored[i], scored[j]
		ciCase, ciPOS := caseMatchScore(surface, ci.Lemma), posSanityScore(surface, ci.POS)
		cjCase, cjPOS := caseMatchScore(surface, cj.Lemma), posSanityScore(surface, cj.POS)
		if ciCase != cjCase {
			return ciCase > cjCase
		}
		if ciPOS != cjPOS {
			return ciPOS > cjPOS
		}
		if ci.SourcePriority != cj.SourcePriority {
			return ci.SourcePriority > cj.SourcePriority
		}
		if ci.Source != cj.Source {
			return ci.Source < cj.Source
		}
		if ci.Lemma != cj.Lemma {
			return ci.Lemma < cj.Lemma
		}
		return ci.POS < cj.POS
	})
	return scored[0]
}

// caseMatchScore returns 1 when the surface and lemma share an uppercase /
// lowercase initial, else 0. Both empty strings count as a match.
func caseMatchScore(surface, lemma string) int {
	if startsUpper(surface) == startsUpper(lemma) {
		return 1
	}
	// Asymmetry: lowercase surface + uppercase lemma is the bad case
	// (PROPN-style homonym beating a common noun). Uppercase surface +
	// lowercase lemma is fine — sentence-initial capitalization is common.
	if !startsUpper(surface) && startsUpper(lemma) {
		return 0
	}
	return 1
}

// posSanityScore demotes PROPN when the surface starts lowercase.
func posSanityScore(surface, pos string) int {
	if !startsUpper(surface) && pos == "PROPN" {
		return 0
	}
	return 1
}

// startsUpper reports whether the first rune of s is uppercase.
func startsUpper(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false
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
// Both halves must exist in the forms table. The right component's POS
// determines the compound's POS (Finnish/Estonian compounds have their
// head on the right).
//
// Two-pass strategy:
//
//	Pass 1: prefer splits where the left side is itself a lemma (a clean
//	        nominative compound boundary, e.g. "pankki|automaatista"). The
//	        right half can be inflected; the resulting compound lemma is
//	        leftLemma + rightLemma.
//	Pass 2: fall back to splits where the right side is itself a lemma
//	        (i.e. the right is the bare head). This handles cases like
//	        ET "rongi|sõit" where the left is a genitive-marked stem and
//	        the right is the head lemma. The compound lemma is the
//	        surface left + rightLemma — gluing on the left's bare lemma
//	        would lose the genitive marker (the et-0032 "Rongisõit" →
//	        "rongõis" bug surfaced this).
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

	// Pass 1: left == leftLemma (clean nominative-boundary compound).
	for i := n - 3; i >= 3; i-- {
		left := string(runes[:i])
		right := string(runes[i:])

		var leftLemma, leftPOS string
		if err := stmtForms.QueryRow(left, lang).Scan(&leftLemma, &leftPOS); err != nil {
			continue
		}
		if left != leftLemma {
			continue
		}
		var rightLemma, rightPOS string
		if err := stmtForms.QueryRow(right, lang).Scan(&rightLemma, &rightPOS); err != nil {
			continue
		}
		return FormResolution{
			Lemma:  leftLemma + rightLemma,
			POS:    rightPOS,
			Source: "compound",
		}, true
	}

	// Pass 2: right == rightLemma (genitive-boundary compound).
	for i := n - 3; i >= 3; i-- {
		left := string(runes[:i])
		right := string(runes[i:])

		var leftLemma, leftPOS string
		if err := stmtForms.QueryRow(left, lang).Scan(&leftLemma, &leftPOS); err != nil {
			continue
		}
		var rightLemma, rightPOS string
		if err := stmtForms.QueryRow(right, lang).Scan(&rightLemma, &rightPOS); err != nil {
			continue
		}
		if right != rightLemma {
			continue
		}
		// Use the surface left (which carries the genitive marker), not
		// leftLemma — otherwise the genitive linker is dropped.
		_ = leftLemma
		_ = leftPOS
		return FormResolution{
			Lemma:  left + rightLemma,
			POS:    rightPOS,
			Source: "compound",
		}, true
	}
	return FormResolution{}, false
}

// ─── Case suffix stripping ──────────────────────────────────────────────────

// attachCaseLabelIfStemMatches augments a successful dict resolution with a
// case label, when the case-suffix matcher independently arrives at the same
// lemma. Stopgap until the FST runtime (PRs #106/#107) lands and emits FEATS
// for direct dict hits.
//
// Returns the input unchanged if no label can be attached. Conservative by
// design: a label is attached only when both
//   - case-suffix-strip succeeds on the surface, and
//   - the suffix-strip lemma exactly matches the dict lemma.
//
// The lemma-equality guard prevents false positives where the suffix happens
// to terminate a different word (e.g. dict says `aas` "meadow", suffix-strip
// would say "inessive of `aa`" — different lemma, no label attached).
func attachCaseLabelIfStemMatches(stmtLemmas *sql.Stmt, dictResolution FormResolution, lower, lang string) FormResolution {
	suffixes := parserules.FinnishCaseSuffixes
	if lang == "ET" {
		suffixes = parserules.EstonianCaseSuffixes
	}
	labeled, ok := tryCaseSuffixStrip(stmtLemmas, lower, lang, suffixes)
	if !ok || labeled.Lemma != dictResolution.Lemma || labeled.GrammarLabel == "" {
		return dictResolution
	}
	dictResolution.GrammarLabel = labeled.GrammarLabel
	dictResolution.Source = dictResolution.Source + "+case_suffix_label"
	return dictResolution
}

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
		for _, candidate := range caseSuffixLemmaCandidates(stem, lang) {
			var lemma, pos string
			if err := stmtLemmas.QueryRow(candidate, lang).Scan(&lemma, &pos); err == nil {
				return FormResolution{Lemma: lemma, POS: pos, GrammarLabel: cs.Label, Source: "case_suffix"}, true
			}
		}
	}
	return FormResolution{}, false
}

func caseSuffixLemmaCandidates(stem, lang string) []string {
	candidates := []string{stem}
	if lang == "ET" {
		candidates = appendEstonianLemmaCandidates(candidates, stem)
	}
	return uniqueNonEmptyStrings(candidates)
}

func appendEstonianLemmaCandidates(candidates []string, stem string) []string {
	if strings.HasSuffix(stem, "mise") {
		candidates = append(candidates, strings.TrimSuffix(stem, "mise")+"mine")
	}
	if strings.HasSuffix(stem, "mis") {
		candidates = append(candidates, strings.TrimSuffix(stem, "mis")+"mine")
	}
	if strings.HasSuffix(stem, "seme") {
		candidates = append(candidates, strings.TrimSuffix(stem, "me"))
	}
	if strings.HasSuffix(stem, "sem") {
		candidates = append(candidates, strings.TrimSuffix(stem, "m"))
	}
	if strings.HasSuffix(stem, "d") {
		candidates = append(candidates, strings.TrimSuffix(stem, "d")+"ne")
	}
	if strings.HasSuffix(stem, "de") {
		candidates = append(candidates, strings.TrimSuffix(stem, "de")+"nne")
	}
	if strings.HasSuffix(stem, "e") {
		withoutE := strings.TrimSuffix(stem, "e")
		candidates = append(candidates, withoutE)
		if strings.HasSuffix(withoutE, "d") {
			candidates = append(candidates, strings.TrimSuffix(withoutE, "d")+"ne")
		}
	}
	return candidates
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// BatchLookupAllForms returns every (lemma, pos) candidate the dictionary has
// for each surface form — used at deck-ingest time to expand homonyms into one
// occurrence row per candidate (e.g. ET "joon" → both joon/NOUN and
// jooma/VERB). Unlike BatchLookupForms this is a direct dictionary lookup
// only; it does not run the possessive / compound / case-suffix fallback
// chain, because those heuristics are designed to commit to a single
// resolution and aren't authoritative for ambiguity.
//
// Forms with no direct dict hit are absent from the result map. Each form's
// slice is non-empty when present.
func (d *DB) BatchLookupAllForms(forms []string, lang string) map[string][]FormResolution {
	result := make(map[string][]FormResolution, len(forms))
	if len(forms) == 0 {
		return result
	}

	stmt, err := d.db.Prepare(`SELECT lemma, pos FROM forms WHERE form = ? AND lang = ?`)
	if err != nil {
		return result
	}
	defer stmt.Close()

	for _, form := range forms {
		lower := strings.ToLower(form)
		rows, err := stmt.Query(lower, lang)
		if err != nil {
			continue
		}
		var candidates []FormResolution
		for rows.Next() {
			var lemma, pos string
			if err := rows.Scan(&lemma, &pos); err != nil {
				rows.Close()
				candidates = nil
				break
			}
			candidates = append(candidates, FormResolution{Lemma: lemma, POS: pos, Source: "dict"})
		}
		rows.Close()
		if len(candidates) > 0 {
			result[form] = candidates
		}
	}
	return result
}

// LemmaKey identifies a unique (lemma, pos) pair for use as a map key.
type LemmaKey struct {
	Lemma string
	POS   string
}

// BatchLookupGlosses resolves a slice of LemmaKeys to their English gloss strings.
// Returns a map from LemmaKey → gloss. Keys with no entry in the lemmas table
// are absent from the map (caller treats absent as empty gloss).
//
// Phase 2 read path: prefer the translations table over lemmas.gloss when
// both exist for the same source. The JOIN on
// (lemma, pos, lang, source) deliberately couples each translation row
// to its co-written lemma row — this lets us rank by lemmas.source_priority
// without adding a denormalized priority column to translations.
//
// Three cases the query handles together:
//
//  1. kaikki-imported entry (lemmas + translations both written by kaikki):
//     JOIN matches, source_priority=10, returns translations.text. Same
//     string as lemmas.gloss for typical entries (sense_idx=0); behavior
//     change only when sense_idx=0 is whitespace and a later sense wins.
//
//  2. custom-override applied via -custom-glosses (lemmas.gloss replaced
//     by source='custom' priority=100, but applyCustomGlosses doesn't
//     write to translations): JOIN finds no translations row matching
//     source='custom', falls through to lemmas.gloss → custom override
//     returned. Preserves the documented custom-glosses contract from
//     README without applyCustomGlosses needing changes here.
//
//  3. multiple sources writing translations for the same (lemma, pos)
//     (forthcoming once Ekilex translations land via PR 4): each row's
//     JOINed lemmas.source_priority decides; deterministic by ORDER BY.
func (d *DB) BatchLookupGlosses(lemmas []LemmaKey, lang string) map[LemmaKey]string {
	result := make(map[LemmaKey]string, len(lemmas))
	if len(lemmas) == 0 {
		return result
	}

	// Single statement queries both tables and falls back deterministically.
	// The translations subquery returns NULL when the JOIN fails (e.g. no
	// matching translation row for the current lemmas.source); COALESCE
	// then falls through to lemmas.gloss, then to '' if neither exists.
	stmt, err := d.db.Prepare(`
		SELECT COALESCE(
		  (SELECT t.text
		   FROM translations t
		   JOIN lemmas l ON l.lemma = t.lemma
		                AND l.pos   = t.pos
		                AND l.lang  = t.lang
		                AND l.source = t.source
		   WHERE t.lemma = ? AND t.pos = ? AND t.lang = ? AND t.target_lang = 'EN'
		   ORDER BY l.source_priority DESC, t.sense_idx ASC
		   LIMIT 1),
		  (SELECT gloss FROM lemmas WHERE lemma = ? AND pos = ? AND lang = ?),
		  ''
		)`,
	)
	if err != nil {
		return result
	}
	defer stmt.Close()

	for _, k := range lemmas {
		var gloss string
		if err := stmt.QueryRow(k.Lemma, k.POS, lang, k.Lemma, k.POS, lang).Scan(&gloss); err == nil && gloss != "" {
			result[k] = gloss
		}
	}
	return result
}
