package store

import (
	"database/sql"
	"sort"
	"strings"
	"unicode"

	"finnestdb/internal/parserules"
	lemmatizer "finnestdb/pkg/lemmatizer-fi-et"
	"finnestdb/pkg/lemmatizer-fi-et/lexadverbs"
	"finnestdb/pkg/lemmatizer-fi-et/udfeats"
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
// Resolution chain:
//
//	Step 1: Direct dictionary lookup (lowercase form in forms table), merged
//	        with FI/ET FST analyses in custom mode when local tables exist.
//	Step 2: [FI only] Possessive suffix strip → re-lookup in forms table
//	Step 3: [FI + ET] Compound split → both halves in forms table
//	Step 4: [FI + ET] Case suffix strip → lemma-candidate lookup in lemmas table
//	Step 5: [FI + ET] FST morphological analysis from generated local tables
//	        built offline from local analyser resources such as libvoikko VFST
//	        and Giellalt/HFSTOL.
//	Step 6: Not resolved → absent from map → caller falls back to stub
//
// In "custom" mode, step 1 is a candidate merge rather than a strict
// short-circuit. Dictionary rows and FI/ET generated-table FST analyses are
// ranked together. A same (lemma, POS) FST analysis enriches the dictionary
// candidate with FEATS/GrammarLabel; a disagreeing FST analysis can win only
// when the dictionary candidate is weak and the FST candidate has better
// case/POS/morphology evidence. "Weak" is intentionally narrow here: the
// dict candidate must have no source priority and no morphology. If no FST
// table is available, this degrades to the SQLite dictionary path and the
// case-suffix label stopgap.
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

		// Step 0: Lexical-overlay short-circuit. lexadverbs catalogues
		// surfaces where every other resolution path (dict + FST) has
		// a known wrong answer the maintainer has already curated
		// against. The overlay wins outright: skip the dict + FST
		// merge for these surfaces. Gated to custom mode because the
		// overlay is part of the "custom enhancements" suite; basic
		// mode stays dict-only to keep its baselines stable.
		if parserMode == "custom" {
			if res, ok := lookupLexOverlay(lang, lower); ok {
				result[form] = res
				continue
			}
		}

		// Step 1: Direct dictionary lookup. Multi-lemma rows mean a single
		// surface form can have multiple (lemma, pos) candidates; rank them
		// against the original-case surface so PROPN homonyms don't beat
		// common-noun lemmas on lowercase surfaces.
		if dictCandidates, ok := lookupFormCandidates(stmtFormsAll, lower, lang); ok {
			best := formResolutionFromCandidate(lower, pickBestFormCandidate(form, dictCandidates))
			if parserMode == "custom" {
				var analyses []lemmatizer.Analysis
				if lang == "FI" || lang == "ET" {
					if lem := d.fstLemmatizer(); lem != nil {
						analyses = lem.Lemmatize(lang, lower)
					}
				}
				best = mergeAndRankDictFSTCandidates(form, dictCandidates, analyses)
				if strings.HasPrefix(best.Source, "dict") && best.GrammarLabel == "" && best.Feats == "" {
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
// analysis exists. Source ordering (VFST > Giellalt for FI; Giellalt only
// for ET; non-compound > compound) lives in pkg/lemmatizer-fi-et; this helper
// applies the store-level case/POS scoring and projects native FST morphology
// into FEATS for downstream display/eval.
func tryFSTAnalyze(lem *lemmatizer.Lemmatizer, lang, lower string) (FormResolution, bool) {
	a, ok := pickBestFSTAnalysis(lower, lem.Lemmatize(lang, lower))
	if !ok {
		return FormResolution{}, false
	}
	return formResolutionFromFSTAnalysis(lower, a, "fst"), true
}

func pickBestFSTAnalysis(surface string, analyses []lemmatizer.Analysis) (lemmatizer.Analysis, bool) {
	valid := make([]lemmatizer.Analysis, 0, len(analyses))
	for _, a := range analyses {
		if a.Lemma == "" || a.UPOS == "" {
			continue
		}
		valid = append(valid, a)
	}
	if len(valid) == 0 {
		return lemmatizer.Analysis{}, false
	}
	return pickBestVFSTAnalysis(surface, valid), true
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

type resolutionCandidate struct {
	res            FormResolution
	sourcePriority int
	hasDict        bool
	hasFST         bool
	fstOrder       int
	fstLabelCount  int    // how many FST analyses matched this candidate
	firstFSTLabel  string // GrammarLabel from the first FST match
	firstFSTFeats  string // FEATS from the first FST match
	origLabel      string // dict candidate's original GrammarLabel before enrichment
	origFeats      string // dict candidate's original Feats before enrichment
	origSource     string // dict candidate's original Source before enrichment
}

func lookupFormCandidates(stmt *sql.Stmt, lowerSurface, lang string) ([]formCandidate, bool) {
	rows, err := stmt.Query(lowerSurface, lang)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	var candidates []formCandidate
	for rows.Next() {
		var c formCandidate
		if err := rows.Scan(&c.Lemma, &c.POS, &c.Source, &c.SourcePriority, &c.Feats); err != nil {
			return nil, false
		}
		if isBadDictLemma(lang, lowerSurface, c.Lemma) {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return nil, false
	}
	return candidates, true
}

// alwaysBadDictLemmasFI lists Finnish lemmas that are NEVER the right
// answer because the "lemma" is a structural artifact rather than a
// standalone word: short fragments kaikki ships as their own headword
// (`as`, `taa`, `ku`), and compound-clip prefixes that only ever
// appear as the left half of a compound (`sisä-`, `ylä-`). Filtering
// these is safe regardless of which surface is being looked up — a
// learner typing the surface `sisä-` is overwhelmingly likely to
// want the compound it belongs to, not the bare prefix.
//
// Note this set is intentionally narrow. Lemmas like `varsi`
// (a real noun "stalk") or `vuo` (archaic but real "stream") MUST
// NOT live here: dropping them globally would break the legitimate
// `varsi` → `varsi/NOUN` dictionary lookup. Those go in the
// surface-keyed pair blocklist below.
//
// `poli` is a special case: kaikki ships it as the lemma for
// `poliisi` inflected forms (a documented import bug). Treating
// it as always-bad is defensible — no learner asking for the
// surface `poli` is looking for the kaikki "poli" entry.
var alwaysBadDictLemmasFI = map[string]struct{}{
	"as":    {},
	"taa":   {},
	"poli":  {},
	"sisä-": {},
	"ylä-":  {},
	"ku":    {},
}

// badSurfaceLemmaFI is the (surface, lemma) pair blocklist. Each
// entry is a kaikki/analyser mapping that ranks first for the trap
// surface but loses to the correct answer on every learner-facing
// occurrence. The lemma is preserved for OTHER surfaces — `varsi`
// is filtered out only for surface `varsin`, not for the bare
// noun lookup `varsi`.
//
// Keyed by "surface\x00lemma" to avoid the struct-key allocation
// cost on every dict candidate scan.
//
// Seeded from yle_subs/build_surface_target_decks.py's
// SUSPICIOUS_SURFACE_LEMMAS.
var badSurfaceLemmaFI = map[string]struct{}{
	"varsin\x00varsi":     {},
	"vuotta\x00vuo":       {},
	"siitä\x00siittää":    {},
	"muuta\x00muuttaa":    {},
	"juuri\x00juuria":     {},
	"mulla\x00mullah":     {},
	"sulla\x00sullah":     {},
	"kuulla\x00kuu":       {},
	"paljon\x00paljo":     {},
}

func isBadDictLemma(lang, lowerSurface, lemma string) bool {
	if lang != "FI" || lemma == "" {
		return false
	}
	if _, blocked := alwaysBadDictLemmasFI[lemma]; blocked {
		return true
	}
	if lowerSurface == "" {
		return false
	}
	_, blocked := badSurfaceLemmaFI[lowerSurface+"\x00"+lemma]
	return blocked
}

func formResolutionFromCandidate(surface string, c formCandidate) FormResolution {
	// MA-infinitive normalisation runs on dict candidates too. Kaikki
	// ships forms like `lähtemään` keyed under `lähteä`/VERB but with
	// stale FEATS (Case=Ill|Person=3 instead of
	// Case=Ill|InfForm=Ma|VerbForm=Inf). The same surface-tagged
	// agreement rule that catches the FST noun-cousin trap rewrites
	// the dict FEATS too, so the resolution returned to callers is
	// consistent regardless of which source supplied the lemma.
	feats := udfeats.NormalizeMaInfinitive(surface, c.POS, c.Feats)
	res := FormResolution{Lemma: c.Lemma, POS: c.POS, Feats: feats, Source: "dict"}
	// Project Case= from Feats to GrammarLabel for back-compat with the
	// case-only metric. New code should consume Feats directly; the
	// projection lets the existing eval keep working unchanged.
	if feats != "" {
		res.GrammarLabel = caseFromFeats(feats)
	}
	return res
}

func mergeAndRankDictFSTCandidates(surface string, dictCandidates []formCandidate, analyses []lemmatizer.Analysis) FormResolution {
	candidates := make([]resolutionCandidate, 0, len(dictCandidates)+len(analyses))
	byKey := make(map[string]int, len(dictCandidates))
	for _, c := range dictCandidates {
		idx := len(candidates)
		candidates = append(candidates, resolutionCandidate{
			res:            formResolutionFromCandidate(surface, c),
			sourcePriority: c.SourcePriority,
			hasDict:        true,
			fstOrder:       len(analyses) + idx,
		})
		byKey[lemmaPOSKey(c.Lemma, c.POS)] = idx
	}

	for order, a := range analyses {
		if a.Lemma == "" || a.UPOS == "" {
			continue
		}
		fstRes := formResolutionFromFSTAnalysis(surface, a, "fst")
		key := lemmaPOSKey(fstRes.Lemma, fstRes.POS)
		if idx, ok := byKey[key]; ok {
			candidates[idx].hasFST = true
			candidates[idx].fstOrder = min(candidates[idx].fstOrder, order)
			if candidates[idx].fstLabelCount == 0 {
				candidates[idx].origLabel = candidates[idx].res.GrammarLabel
				candidates[idx].origFeats = candidates[idx].res.Feats
				candidates[idx].origSource = candidates[idx].res.Source
				candidates[idx].res = enrichResolutionWithFST(candidates[idx].res, fstRes)
				candidates[idx].firstFSTLabel = fstRes.GrammarLabel
				candidates[idx].firstFSTFeats = fstRes.Feats
			} else if candidates[idx].firstFSTLabel != fstRes.GrammarLabel || candidates[idx].firstFSTFeats != fstRes.Feats {
				candidates[idx].res.GrammarLabel = candidates[idx].origLabel
				candidates[idx].res.Feats = candidates[idx].origFeats
				candidates[idx].res.Source = candidates[idx].origSource
			}
			candidates[idx].fstLabelCount++
			continue
		}
		candidates = append(candidates, resolutionCandidate{
			res:      fstRes,
			hasFST:   true,
			fstOrder: order,
		})
	}

	if len(candidates) == 0 {
		return FormResolution{}
	}
	return pickBestResolutionCandidate(surface, candidates).res
}

func lemmaPOSKey(lemma, pos string) string {
	return lemma + "\x00" + pos
}

func enrichResolutionWithFST(dictRes, fstRes FormResolution) FormResolution {
	if dictRes.Feats == "" && fstRes.Feats != "" {
		dictRes.Feats = fstRes.Feats
		dictRes.GrammarLabel = caseFromFeats(fstRes.Feats)
		if dictRes.GrammarLabel == "" && fstRes.GrammarLabel != "" {
			dictRes.GrammarLabel = fstRes.GrammarLabel
		}
		dictRes.Source = appendSourceTag(dictRes.Source, "fst_feats")
		return dictRes
	}
	if dictRes.GrammarLabel == "" && fstRes.GrammarLabel != "" {
		dictRes.GrammarLabel = fstRes.GrammarLabel
		dictRes.Source = appendSourceTag(dictRes.Source, "fst_label")
	}
	return dictRes
}

func appendSourceTag(source, tag string) string {
	if source == "" {
		return tag
	}
	for _, part := range strings.Split(source, "+") {
		if part == tag {
			return source
		}
	}
	return source + "+" + tag
}

func pickBestResolutionCandidate(surface string, candidates []resolutionCandidate) resolutionCandidate {
	if len(candidates) == 1 {
		return candidates[0]
	}
	scored := make([]resolutionCandidate, len(candidates))
	copy(scored, candidates)
	sort.SliceStable(scored, func(i, j int) bool {
		ci, cj := scored[i], scored[j]
		ciCase, ciPOS := caseMatchScore(surface, ci.res.Lemma), posSanityScore(surface, ci.res.POS)
		cjCase, cjPOS := caseMatchScore(surface, cj.res.Lemma), posSanityScore(surface, cj.res.POS)
		if ciCase != cjCase {
			return ciCase > cjCase
		}
		if ciPOS != cjPOS {
			return ciPOS > cjPOS
		}
		// MA-infinitive bias: on a surface that matches the
		// MA-infinitive suffix pattern, prefer a VERB reading with
		// InfForm=Ma over a NOUN noun-cousin reading. Kaikki ships
		// many MA-surfaces (lähtemään, juomassa, tarjoamaan, ...) as
		// forms of derived nouns (lähtemä, juoma, tarjoama) with
		// Case=Ill|Person=3|Number=Sing; the FST returns the
		// correct verb reading via NormalizeMaInfinitive but loses
		// the supportScore tiebreak against dict. This bias overrides
		// that for MA-surfaces only.
		if mi, mj := maInfinitiveBias(surface, ci.res), maInfinitiveBias(surface, cj.res); mi != mj {
			return mi > mj
		}
		if fstBeatsWeakDict(ci, cj) {
			return true
		}
		if fstBeatsWeakDict(cj, ci) {
			return false
		}
		if ci.sourcePriority != cj.sourcePriority {
			return ci.sourcePriority > cj.sourcePriority
		}
		if supportScore(ci) != supportScore(cj) {
			return supportScore(ci) > supportScore(cj)
		}
		if morphologyScore(ci.res) != morphologyScore(cj.res) {
			return morphologyScore(ci.res) > morphologyScore(cj.res)
		}
		if ci.fstOrder != cj.fstOrder {
			return ci.fstOrder < cj.fstOrder
		}
		if ci.res.Source != cj.res.Source {
			return ci.res.Source < cj.res.Source
		}
		if ci.res.Lemma != cj.res.Lemma {
			return ci.res.Lemma < cj.res.Lemma
		}
		return ci.res.POS < cj.res.POS
	})
	return scored[0]
}

func supportScore(c resolutionCandidate) int {
	switch {
	case c.hasDict && c.hasFST:
		return 4
	case c.hasDict:
		return 3
	case c.hasFST:
		return 2
	default:
		return 0
	}
}

// maInfinitiveBias returns +1 for a VERB reading carrying InfForm=Ma
// on a MA-infinitive-shaped surface, -1 for a NOUN reading whose
// lemma ends in -ma/-mä on the same surface (the noun-cousin trap),
// and 0 otherwise. Surfaces that don't match the MA-suffix pattern
// always return 0, so this bias is a no-op for the common case.
//
// Why two bias levels and not just "verb wins": there ARE legitimate
// noun readings of MA-suffix surfaces in compounds (`tarjouspakkaus`
// shares no overlap, but uncovered edge cases would emerge if the
// bias were unbounded). Capping at ±1 lets surface morphology
// signal preference without overwhelming other ranking dimensions.
func maInfinitiveBias(surface string, res FormResolution) int {
	if !udfeats.IsMaInfinitiveSurface(surface) {
		return 0
	}
	// Noun-cousin shape FIRST. Lemma ends in -ma/-mä on a MA-suffix
	// surface is the trap regardless of POS or whether the FEATS
	// already carry InfForm=Ma. The dict-side NormalizeMaInfinitive
	// will have rewritten the feats to look like a real MA-infinitive
	// (Case=X|InfForm=Ma|VerbForm=Inf|Voice=Act) for these noun-cousin
	// candidates too — but the lemma is the part that's wrong. Demote
	// on the lemma signal before granting the verb promotion below.
	//
	// Finnish verb 1st-infinitives end in -a/-ä (with -da/-ta/-la/-na
	// preceding); a lemma ending in -ma/-mä on this surface is almost
	// always the derived-noun reading. Counter-examples would be
	// loanwords or proper nouns; surface this if/when we find one.
	if strings.HasSuffix(res.Lemma, "ma") || strings.HasSuffix(res.Lemma, "mä") {
		return -1
	}
	if res.POS == "VERB" && strings.Contains(res.Feats, "InfForm=Ma") {
		return 1
	}
	return 0
}

func fstBeatsWeakDict(candidate, other resolutionCandidate) bool {
	// Conservative displacement: an FST-only candidate may beat a dict-only
	// candidate only when the dict row is legacy/weak (no source priority and
	// no FEATS or projected label). Dict morphology, even partial, remains
	// authoritative until production eval shows a broader threshold is safe.
	return candidate.hasFST &&
		!candidate.hasDict &&
		morphologyScore(candidate.res) > 0 &&
		other.hasDict &&
		!other.hasFST &&
		other.sourcePriority <= 0 &&
		morphologyScore(other.res) == 0
}

// morphologyScore is intentionally binary: 1 if the resolution carries any
// morphology (FEATS or a projected GrammarLabel), 0 otherwise.
//
// A scaled version (e.g. one point per FEATS attribute) is tempting but
// systematically biases POS comparisons: Estonian VERBs carry ~6 FEATS
// dimensions (Mood/Number/Person/Tense/VerbForm/Voice) while NOUNs carry ~2
// (Case/Number), so a richer-wins tiebreak makes any verb homonym beat any
// noun reading regardless of which one is actually correct in context. After
// the 2026-05-07 Ekilex bulk landed dense FEATS on every form, this caused
// observable lemma/POS regressions on et-grammar (arstiks→arstima/VERB,
// teed→tegema/VERB, koolis→koolma/VERB, kirja→kirjama/VERB, mees→mesi/NOUN).
//
// Binary scoring keeps morphology meaningful where it actually signals
// confidence (a FEATS-bearing candidate beating a bare one, e.g. an
// FST-enriched dict candidate vs. a legacy entry with no morphology) without
// turning into a POS-density vote between cross-POS homonyms.
func morphologyScore(res FormResolution) int {
	if res.GrammarLabel != "" || res.Feats != "" {
		return 1
	}
	return 0
}

// lookupLexOverlay returns the curated lexadverbs analysis as a
// FormResolution when one exists for (lang, lower). The overlay
// catalogues surfaces where the productive dict/FST analysis is a
// known bug — `tuskin` (kaikki ships as form of `tuska`), `asiaan`
// (form of bad lemma `as`), `vuotta` (form of bad lemma `vuo`),
// `peale` (allative of `pea`), and so on. Each entry is asserted as
// the right answer; this short-circuit ensures it actually wins the
// resolution, instead of being merged with and outranked by the
// dict candidate.
//
// Source tag is "lex-overlay" so eval reports can attribute hits to
// this layer distinctly from dict/fst/dict+fst.
func lookupLexOverlay(lang, lower string) (FormResolution, bool) {
	var analyses []lemmatizer.Analysis
	var ok bool
	switch lang {
	case "FI":
		analyses, ok = lexadverbs.LookupFI(lower)
	case "ET":
		analyses, ok = lexadverbs.LookupET(lower)
	default:
		return FormResolution{}, false
	}
	if !ok || len(analyses) == 0 {
		return FormResolution{}, false
	}
	return formResolutionFromFSTAnalysis(lower, analyses[0], "lex-overlay"), true
}

func formResolutionFromFSTAnalysis(surface string, a lemmatizer.Analysis, source string) FormResolution {
	feats := featsFromFSTAnalysis(a)
	// Rewrite MA-infinitive noun-cousin traps. This catches forms like
	// `tarjoamaan` where the analyzer ranks the noun `tarjoama` first
	// and emits Case=Ill|Number=Sing|Person=3 even though the form is
	// the MA-infinitive illative of the verb `tarjota`. Surfaced-tagged
	// agreement (suffix harmony + Case feature) lets udfeats rewrite
	// the FEATS deterministically.
	feats = udfeats.NormalizeMaInfinitive(surface, a.UPOS, feats)
	label := ""
	if feats != "" {
		label = caseFromFeats(feats)
	}
	if label == "" && a.GrammarLabel != "nominative" {
		label = a.GrammarLabel
	}
	return FormResolution{
		Lemma:        a.Lemma,
		POS:          a.UPOS,
		GrammarLabel: label,
		Feats:        feats,
		Source:       source,
	}
}

func featsFromFSTAnalysis(a lemmatizer.Analysis) string {
	// Prefer the FEATS string the analyzer baked in at Parse() time.
	// Falls through for legacy table files that pre-date the Feats field.
	if a.Feats != "" {
		return a.Feats
	}
	pairs := map[string]string{
		"Number":       a.Number,
		"Tense":        a.Tense,
		"Mood":         a.Mood,
		"Person":       a.Person,
		"Voice":        a.Voice,
		"VerbForm":     a.VerbForm,
		"Degree":       a.Degree,
		"PronType":     a.PronType,
		"PartForm":     a.PartForm,
		"InfForm":      a.InfForm,
		"Person[psor]": a.PersonPsor,
		"Number[psor]": a.NumberPsor,
		"Clitic":       a.Clitic,
		"NumType":      a.NumType,
		"Connegative":  a.Connegative,
		"AdpType":      a.AdpType,
	}
	if udCase, ok := udfeats.LegacyLabelToUDCase[a.GrammarLabel]; ok && udCase != "Nom" {
		pairs["Case"] = udCase
	}
	return udfeats.ComposeMap(pairs)
}

// featsFromCaseLabel projects a lowercase English case name (the
// grammar_label vocabulary) into a minimal UD FEATS string. Used by the
// case-suffix stripping path so a token resolved purely from suffix
// removal still carries Case= in its FEATS — without this, the FEATS
// comparison silently scores those tokens as missing.
//
// Returns "" for "" / "nominative" (Nom is implicit per UD convention)
// or for unknown labels. Number= is deliberately NOT projected here:
// a stripped suffix ("ssa" → inessive) doesn't disambiguate Sing vs
// Plur. The dict layer doesn't claim what the suffix can't tell us.
func featsFromCaseLabel(label string) string {
	if label == "" {
		return ""
	}
	udCase, ok := udfeats.LegacyLabelToUDCase[label]
	if !ok || udCase == "Nom" {
		return ""
	}
	return "Case=" + udCase
}

// caseFromFeats extracts the Case= attribute from a UD FEATS string and
// returns the lowercase English case name used by our existing
// grammar_label vocabulary. Returns "" when FEATS has no Case= attribute
// or when the value is Nominative (left implicit per existing convention,
// matching cmd/importud/main.go). Thin alias for udfeats.CaseFromFeats —
// retained at this name because it's the dict layer's vocabulary anchor.
func caseFromFeats(feats string) string {
	return udfeats.CaseFromFeats(feats)
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
				return FormResolution{
					Lemma:        lemma,
					POS:          pos,
					GrammarLabel: cs.Label,
					Feats:        featsFromCaseLabel(cs.Label),
					Source:       "case_suffix",
				}, true
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

// BatchLookupAllForms returns every learner-facing (lemma, pos) candidate for
// each surface form — used at deck-ingest time to expand homonyms into one
// occurrence row per candidate (e.g. ET "joon" → both joon/NOUN and
// jooma/VERB). In custom mode, lexical-overlay surfaces short-circuit to the
// curated single reading because those entries are known raw-dict/analyzer
// traps, not real ambiguity. Basic mode remains direct dictionary lookup to
// match BatchLookupForms' baseline contract. Otherwise this does not run the
// possessive / compound / case-suffix fallback chain, because those heuristics
// are designed to commit to a single resolution and aren't authoritative for
// ambiguity.
//
// Forms with no direct dict hit are absent from the result map. Each form's
// slice is non-empty when present.
func (d *DB) BatchLookupAllForms(forms []string, lang string, parserMode string) map[string][]FormResolution {
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
		if parserMode == "custom" {
			if res, ok := lookupLexOverlay(lang, lower); ok {
				result[form] = []FormResolution{res}
				continue
			}
		}

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

// Sense is one ranked gloss reading for a (lemma, pos) pair. Senses
// from the translations table arrive in priority+order: higher
// SourcePriority comes first across sources, then lower SenseIdx
// within the same source. The first element of the slice returned by
// BatchLookupSenses is therefore the same string BatchLookupGlosses
// returns — multi-sense callers can drop the first to get
// "alternative meanings" without re-querying.
type Sense struct {
	Text           string // the gloss text (already trimmed; never empty)
	SenseIdx       int    // sense_idx as written by importdict (flat counter across senses[].glosses[])
	Source         string // source key (e.g. "kaikki", "ekilex", "custom")
	SourcePriority int    // lemmas.source_priority for the source this sense came from
}

// BatchLookupSenses resolves a slice of LemmaKeys to their full ranked
// list of glosses. Returns a map from LemmaKey → []Sense. Keys with no
// translations row are absent from the map; the existing
// lemmas.gloss fallback in BatchLookupGlosses does NOT participate
// here because that field is a denormalised cache of the primary
// translations row, not a separate authority worth ranking.
//
// Ranking order matches BatchLookupGlosses: source_priority DESC
// (kaikki=10, EKI=20, custom=100), then sense_idx ASC (first sense
// wins inside a source). Callers that want a richer ranking — e.g.
// length-aware tiebreaks, or filtering by POS in context — should
// compose that on top of this slice rather than mutate the storage
// query.
//
// Target lang is hard-coded to 'EN' because both kaikki dumps for FI
// and ET are en.wiktionary extractions. If we add FI-Wiktionary
// monolingual glosses (target_lang='FI') the call signature would
// gain a target lang parameter; today there's only one shape.
//
// Seeded by the yle_subs TODO at yle_subs/TODO.md (FinEstDB lemma-gloss
// lookup), where the downstream deck builder wants multi-sense data
// to render context-appropriate front cues for polysemous lemmas
// (`pää` → "head" generally, "on top of" in `kirjoituspöydän päällä`).
// The lookup itself is fast — sub-millisecond per key on an indexed
// translations table — so callers are free to fetch many at once.
func (d *DB) BatchLookupSenses(lemmas []LemmaKey, lang string) map[LemmaKey][]Sense {
	result := make(map[LemmaKey][]Sense, len(lemmas))
	if len(lemmas) == 0 {
		return result
	}

	stmt, err := d.db.Prepare(`
		SELECT t.text, t.sense_idx, t.source, l.source_priority
		FROM translations t
		JOIN lemmas l ON l.lemma = t.lemma
		             AND l.pos   = t.pos
		             AND l.lang  = t.lang
		             AND l.source = t.source
		WHERE t.lemma = ? AND t.pos = ? AND t.lang = ? AND t.target_lang = 'EN'
		ORDER BY l.source_priority DESC, t.sense_idx ASC, t.source ASC
	`)
	if err != nil {
		return result
	}
	defer stmt.Close()

	for _, k := range lemmas {
		rows, err := stmt.Query(k.Lemma, k.POS, lang)
		if err != nil {
			continue
		}
		var senses []Sense
		for rows.Next() {
			var s Sense
			if err := rows.Scan(&s.Text, &s.SenseIdx, &s.Source, &s.SourcePriority); err != nil {
				rows.Close()
				senses = nil
				break
			}
			if s.Text == "" {
				continue
			}
			senses = append(senses, s)
		}
		rows.Close()
		if len(senses) > 0 {
			result[k] = senses
		}
	}
	return result
}
