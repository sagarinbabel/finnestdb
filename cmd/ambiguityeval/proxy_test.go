package main

import "testing"

func TestClassifyProxy(t *testing.T) {
	cases := []struct {
		name           string
		candidateCount int
		pickSource     string
		want           Proxy
	}{
		{"single candidate", 1, "dict", ProxySingle},
		{"zero candidates treated as single (parser pick outside dict, e.g. lex-overlay)", 0, "lex-overlay", ProxySingle},
		{"multi candidates, dict+fst agreement", 2, "dict+fst_feats", ProxyMultiAgree},
		{"multi candidates, dict+fst_label tag", 3, "dict+fst_label", ProxyMultiAgree},
		{"multi candidates, dict only, no fst corroboration", 2, "dict", ProxyMulti},
		{"multi candidates, fst only, no dict corroboration", 2, "fst_feats", ProxyMulti},
		{"multi candidates, unrelated source", 2, "possessive", ProxyMulti},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyProxy(c.candidateCount, c.pickSource); got != c.want {
				t.Errorf("ClassifyProxy(%d, %q) = %s, want %s", c.candidateCount, c.pickSource, got, c.want)
			}
		})
	}
}
