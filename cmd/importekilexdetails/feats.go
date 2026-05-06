package main

import "strings"

// ekilexMorphToFeats projects an Ekilex morph_code (the third column in
// data/ekilex/forms/*.tsv) to a UD-FEATS string. Returns "" when the code
// has no productive morphology to encode (idioms, phrases, unknown).
//
// Source distribution we're targeting (see docs/LEARNINGS.md 2026-05-07
// finding 2): 30 nominal + ~50 verbal codes cover ~99% of all rows.
//
// Coverage philosophy:
//
//   - Map every code that has clear UD FEATS equivalents.
//   - Aditive (Adt) folds to Case=Ill — UD-EDT does the same.
//   - The trailing "_" variant on Ind*Pr*Ps_ is a positive-form marker —
//     drop it before lookup; the base code carries the same FEATS.
//   - Negative variants (suffix N or Neg) append Polarity=Neg to the
//     base form's FEATS.
//   - Idiom (ID), gerund (Ger), and unknown (??) return empty — we
//     don't claim FEATS we can't defend.
//
// The mapping vocabulary (Case=Ine|Number=Sing etc.) matches UD-Estonian
// FEATS exactly, so projecting Ekilex morph_codes here lines up with
// our UD-EDT gold rows from PR #113.
func ekilexMorphToFeats(code string) string {
	if code == "" {
		return ""
	}
	// Strip a trailing "_" — Ekilex uses this as a "positive" marker on
	// some codes (IndPrPs_, IndPrIps_) where the bare form already
	// implies the same.
	code = strings.TrimSuffix(code, "_")

	// Negative-polarity variants: <base>N or <base>Neg.
	//   * The "N" suffix is the older Ekilex convention (e.g. IndPrSg1N)
	//     but it never appears in our distribution that way — only
	//     IndPrPsN / IndPrIpsN-style "personal class + N" exist.
	//   * "Neg" is used by participles only: PtsPtIpsNeg, PtsPtPsNeg.
	negPolarity := false
	switch {
	case strings.HasSuffix(code, "Neg"):
		negPolarity = true
		code = strings.TrimSuffix(code, "Neg")
	case strings.HasSuffix(code, "N") && len(code) > 1 && (strings.HasPrefix(code, "Ind") || strings.HasPrefix(code, "Knd") || strings.HasPrefix(code, "Kvt") || strings.HasPrefix(code, "Imp") || strings.HasPrefix(code, "Inf")):
		negPolarity = true
		code = strings.TrimSuffix(code, "N")
	}

	feats, ok := ekilexBaseFeats[code]
	if !ok {
		return ""
	}
	if negPolarity {
		feats = appendFeat(feats, "Polarity=Neg")
	}
	return feats
}

// appendFeat inserts a key=value pair into a UD FEATS string, keeping
// the result alphabetically sorted (UD's canonical encoding). Empty
// existing string is fine.
func appendFeat(feats, kv string) string {
	if feats == "" {
		return kv
	}
	parts := strings.Split(feats, "|")
	parts = append(parts, kv)
	// UD requires alphabetical attribute order. Cheap sort.
	for i := 1; i < len(parts); i++ {
		for j := i; j > 0 && parts[j-1] > parts[j]; j-- {
			parts[j-1], parts[j] = parts[j], parts[j-1]
		}
	}
	return strings.Join(parts, "|")
}

// ekilexBaseFeats is the lookup table for non-negative variants of every
// Ekilex morph_code we recognise. Negative-polarity variants are derived
// at lookup time by ekilexMorphToFeats.
//
// Categories represented:
//
//	Nominal (number × case): Sg{N,G,P,Ill,Adt,In,El,All,Ad,Abl,Tr,Es,Ab,Kom,Ter}
//	                          Pl{N,G,P,Ill,Adt,In,El,All,Ad,Abl,Tr,Es,Ab,Kom,Ter}
//	Verbal indicative finite: IndPr{Sg1,Sg2,Sg3,Pl1,Pl2,Pl3} present active
//	                          IndIpf{Sg1,Sg2,Sg3,Pl1,Pl2,Pl3} past active
//	                          IndPrPs / IndPrIps                present pass/imp
//	                          IndIpfPs / IndIpfIps              past pass/imp
//	Conditional / quotative / imperative: Knd*, Kvt*, Imp* — same shape
//	Non-finite: Inf (-ma supine), Sup (-da infinitive), Ger
//	Participles: PtsPr{Ps,Ips} present active/passive
//	             PtsPt{Ps,Ips} past active/passive
//	Supine + case: SupAb / SupEl / SupIn / SupTr / SupIps
//	Idiomatic: ID — left empty
var ekilexBaseFeats = map[string]string{
	// ── Nominal ─────────────────────────────────────────────────────────
	"SgN":   "Case=Nom|Number=Sing",
	"SgG":   "Case=Gen|Number=Sing",
	"SgP":   "Case=Par|Number=Sing",
	"SgIll": "Case=Ill|Number=Sing",
	"SgAdt": "Case=Ill|Number=Sing", // aditive — UD folds into illative
	"SgIn":  "Case=Ine|Number=Sing",
	"SgEl":  "Case=Ela|Number=Sing",
	"SgAll": "Case=All|Number=Sing",
	"SgAd":  "Case=Ade|Number=Sing",
	"SgAbl": "Case=Abl|Number=Sing",
	"SgTr":  "Case=Tra|Number=Sing",
	"SgEs":  "Case=Ess|Number=Sing",
	"SgAb":  "Case=Abe|Number=Sing",
	"SgKom": "Case=Com|Number=Sing",
	"SgTer": "Case=Ter|Number=Sing",

	"PlN":   "Case=Nom|Number=Plur",
	"PlG":   "Case=Gen|Number=Plur",
	"PlP":   "Case=Par|Number=Plur",
	"PlIll": "Case=Ill|Number=Plur",
	"PlAdt": "Case=Ill|Number=Plur",
	"PlIn":  "Case=Ine|Number=Plur",
	"PlEl":  "Case=Ela|Number=Plur",
	"PlAll": "Case=All|Number=Plur",
	"PlAd":  "Case=Ade|Number=Plur",
	"PlAbl": "Case=Abl|Number=Plur",
	"PlTr":  "Case=Tra|Number=Plur",
	"PlEs":  "Case=Ess|Number=Plur",
	"PlAb":  "Case=Abe|Number=Plur",
	"PlKom": "Case=Com|Number=Plur",
	"PlTer": "Case=Ter|Number=Plur",

	// ── Indicative finite — present active ──────────────────────────────
	"IndPrSg1": "Mood=Ind|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act",
	"IndPrSg2": "Mood=Ind|Number=Sing|Person=2|Tense=Pres|VerbForm=Fin|Voice=Act",
	"IndPrSg3": "Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin|Voice=Act",
	"IndPrPl1": "Mood=Ind|Number=Plur|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act",
	"IndPrPl2": "Mood=Ind|Number=Plur|Person=2|Tense=Pres|VerbForm=Fin|Voice=Act",
	"IndPrPl3": "Mood=Ind|Number=Plur|Person=3|Tense=Pres|VerbForm=Fin|Voice=Act",

	// ── Indicative finite — past (imperfect) active ─────────────────────
	"IndIpfSg1": "Mood=Ind|Number=Sing|Person=1|Tense=Past|VerbForm=Fin|Voice=Act",
	"IndIpfSg2": "Mood=Ind|Number=Sing|Person=2|Tense=Past|VerbForm=Fin|Voice=Act",
	"IndIpfSg3": "Mood=Ind|Number=Sing|Person=3|Tense=Past|VerbForm=Fin|Voice=Act",
	"IndIpfPl1": "Mood=Ind|Number=Plur|Person=1|Tense=Past|VerbForm=Fin|Voice=Act",
	"IndIpfPl2": "Mood=Ind|Number=Plur|Person=2|Tense=Past|VerbForm=Fin|Voice=Act",
	"IndIpfPl3": "Mood=Ind|Number=Plur|Person=3|Tense=Past|VerbForm=Fin|Voice=Act",

	// ── Indicative — personal/impersonal forms ──────────────────────────
	"IndPrPs":   "Mood=Ind|Tense=Pres|VerbForm=Fin|Voice=Act",
	"IndPrIps":  "Mood=Ind|Tense=Pres|VerbForm=Fin|Voice=Pass",
	"IndIpfPs":  "Mood=Ind|Tense=Past|VerbForm=Fin|Voice=Act",
	"IndIpfIps": "Mood=Ind|Tense=Past|VerbForm=Fin|Voice=Pass",

	// ── Conditional ─────────────────────────────────────────────────────
	"KndPrSg1": "Mood=Cnd|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act",
	"KndPrSg2": "Mood=Cnd|Number=Sing|Person=2|Tense=Pres|VerbForm=Fin|Voice=Act",
	"KndPrPl1": "Mood=Cnd|Number=Plur|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act",
	"KndPrPl2": "Mood=Cnd|Number=Plur|Person=2|Tense=Pres|VerbForm=Fin|Voice=Act",
	"KndPrPl3": "Mood=Cnd|Number=Plur|Person=3|Tense=Pres|VerbForm=Fin|Voice=Act",
	"KndPrPs":  "Mood=Cnd|Tense=Pres|VerbForm=Fin|Voice=Act",
	"KndPrIps": "Mood=Cnd|Tense=Pres|VerbForm=Fin|Voice=Pass",
	"KndPtSg1": "Mood=Cnd|Number=Sing|Person=1|Tense=Past|VerbForm=Fin|Voice=Act",
	"KndPtSg2": "Mood=Cnd|Number=Sing|Person=2|Tense=Past|VerbForm=Fin|Voice=Act",
	"KndPtPl1": "Mood=Cnd|Number=Plur|Person=1|Tense=Past|VerbForm=Fin|Voice=Act",
	"KndPtPl2": "Mood=Cnd|Number=Plur|Person=2|Tense=Past|VerbForm=Fin|Voice=Act",
	"KndPtPl3": "Mood=Cnd|Number=Plur|Person=3|Tense=Past|VerbForm=Fin|Voice=Act",
	"KndPtPs":  "Mood=Cnd|Tense=Past|VerbForm=Fin|Voice=Act",
	"KndPtIps": "Mood=Cnd|Tense=Past|VerbForm=Fin|Voice=Pass",

	// ── Quotative ───────────────────────────────────────────────────────
	"KvtPrPs":  "Mood=Qot|Tense=Pres|VerbForm=Fin|Voice=Act",
	"KvtPrIps": "Mood=Qot|Tense=Pres|VerbForm=Fin|Voice=Pass",
	"KvtPtPs":  "Mood=Qot|Tense=Past|VerbForm=Fin|Voice=Act",
	"KvtPtIps": "Mood=Qot|Tense=Past|VerbForm=Fin|Voice=Pass",

	// ── Imperative ──────────────────────────────────────────────────────
	"ImpPrSg2": "Mood=Imp|Number=Sing|Person=2|Tense=Pres|VerbForm=Fin|Voice=Act",
	"ImpPrPl1": "Mood=Imp|Number=Plur|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act",
	"ImpPrPl2": "Mood=Imp|Number=Plur|Person=2|Tense=Pres|VerbForm=Fin|Voice=Act",
	"ImpPrPs":  "Mood=Imp|Tense=Pres|VerbForm=Fin|Voice=Act",
	"ImpPrIps": "Mood=Imp|Tense=Pres|VerbForm=Fin|Voice=Pass",

	// ── Non-finite ──────────────────────────────────────────────────────
	"Inf": "VerbForm=Inf", // -ma supine (UD treats as VerbForm=Sup but Ekilex calls it Inf)
	"Sup": "VerbForm=Sup", // -da infinitive

	// ── Participles ─────────────────────────────────────────────────────
	"PtsPrPs":  "Tense=Pres|VerbForm=Part|Voice=Act",
	"PtsPrIps": "Tense=Pres|VerbForm=Part|Voice=Pass",
	"PtsPtPs":  "Tense=Past|VerbForm=Part|Voice=Act",
	"PtsPtIps": "Tense=Past|VerbForm=Part|Voice=Pass",

	// ── Supine + case (rare but real for verbal nouns) ──────────────────
	"SupAb":  "Case=Abe|VerbForm=Sup",
	"SupEl":  "Case=Ela|VerbForm=Sup",
	"SupIn":  "Case=Ine|VerbForm=Sup",
	"SupTr":  "Case=Tra|VerbForm=Sup",
	"SupIps": "VerbForm=Sup|Voice=Pass",

	// ── Empty / placeholders ────────────────────────────────────────────
	// "ID" (idiom), "Ger" (gerund), "??" (unknown) deliberately absent —
	// returning "" for these is the right behavior.
}
