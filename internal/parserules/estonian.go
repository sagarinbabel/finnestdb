package parserules

// EstonianCaseSuffixes maps the 14 productive Estonian case endings to their
// grammar labels. Ordered longest-first so the stripping loop in
// store.tryCaseSuffixStrip prefers more-specific matches.
//
// Estonian has the same broad case inventory as Finnish but with shorter
// endings (no doubled consonants), so longest-first ordering matters even
// more here — "sse" must come before "s" or it'd be stripped as inessive.
var EstonianCaseSuffixes = []CaseSuffix{
	{"sse", "illative"},
	{"st", "elative"},
	{"le", "allative"},
	{"lt", "ablative"},
	{"ks", "translative"},
	{"ta", "abessive"},
	{"ga", "comitative"},
	{"ni", "terminative"},
	{"na", "essive"},
	{"s", "inessive"},
	{"l", "adessive"},
	{"d", "nominative plural"},
	{"t", "partitive"},
}
