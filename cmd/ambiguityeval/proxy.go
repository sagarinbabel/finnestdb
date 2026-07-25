package main

import "strings"

// ClassifyProxy implements the structural confidence proxy defined in
// docs/PARSER_EVAL_METHODOLOGY.md §"Confidence proxy": there is no numeric
// confidence today, so calibration is measured against signals that already
// exist - the size of the candidate set, and whether the parser's pick was
// corroborated by both a dictionary row and an FST analysis.
//
// candidateCount is the number of distinct (lemma, POS) candidates from
// store.BatchLookupAllForms for the target surface. pickSource is the
// winning token's parsecore.TokenResult.Source string, which is
// "+"-joined (see internal/store/dict.go's appendSourceTag) and can carry
// both a "dict" component and an "fst_*" tag when both corroborate the pick.
func ClassifyProxy(candidateCount int, pickSource string) Proxy {
	if candidateCount <= 1 {
		return ProxySingle
	}
	if strings.Contains(pickSource, "dict") && strings.Contains(pickSource, "fst_") {
		return ProxyMultiAgree
	}
	return ProxyMulti
}
