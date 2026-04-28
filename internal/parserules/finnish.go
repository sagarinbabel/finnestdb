package parserules

// FinnishPossessiveSuffixes lists Finnish possessive suffixes the enrichment
// chain tries when a surface form is not found directly in the forms table.
// Ordered longest-first to avoid partial matches (e.g. "nsa" before "si").
//
// Stripping logic (applied by store.tryStripPossessive):
//
//	form → strip suffix → re-look up stripped form in forms table.
//	Accept only if the stripped form IS in the forms table — this gives us
//	dictionary validation. Otherwise reject and keep trying.
//
// Example: "kirjassani"
//
//	strip "ni" → "kirjassa" → found in forms → lemma: kirja ✓
//
// Counter-example: "talo"
//
//	strip "si" → "tal" → not in forms → reject ✗
var FinnishPossessiveSuffixes = []string{
	"nsa", "nsä", // 3rd person (any number): "kirjansa"
	"mme", // 1st person plural: "kirjamme"
	"nne", // 2nd person plural: "kirjanne"
	"ni",  // 1st person singular: "kirjani"
	"si",  // 2nd person singular: "kirjasi"
}

// FinnishCaseSuffixes maps the 15 productive Finnish case endings to their
// grammar labels. Ordered longest-first so the stripping loop in
// store.tryCaseSuffixStrip prefers more-specific matches.
//
// Two entries (-ssaan, -staan) handle the common case+possessive fusions
// that appear in real text and that bare case-suffix stripping would miss.
var FinnishCaseSuffixes = []CaseSuffix{
	{"ssaan", "inessive (3rd poss)"},
	{"staan", "elative (3rd poss)"},
	{"ssa", "inessive"}, {"ssä", "inessive"},
	{"sta", "elative"}, {"stä", "elative"},
	{"lla", "adessive"}, {"llä", "adessive"},
	{"lta", "ablative"}, {"ltä", "ablative"},
	{"lle", "allative"},
	{"ksi", "translative"},
	{"tta", "abessive"}, {"ttä", "abessive"},
	{"ine", "comitative"},
	{"aan", "illative"}, {"ään", "illative"}, {"iin", "illative"},
	{"on", "illative"}, {"un", "illative"}, {"yn", "illative"},
	{"na", "essive"}, {"nä", "essive"},
	{"n", "genitive"},
	{"a", "partitive"}, {"ä", "partitive"},
	{"t", "nominative plural"},
}
