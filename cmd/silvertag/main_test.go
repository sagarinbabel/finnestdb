package main

import "testing"

func TestSplitSentences(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single", "Hei.", []string{"Hei."}},
		{"two", "Hei. Mitä kuuluu?", []string{"Hei.", "Mitä kuuluu?"}},
		{"paragraph break", "Eka.\n\nToka.", []string{"Eka.", "Toka."}},
		{"exclamation", "Wow! Tosi siistiä.", []string{"Wow!", "Tosi siistiä."}},
		{"trailing whitespace", "Hei. ", []string{"Hei."}},
		{"no terminal punct", "Sentence without period", []string{"Sentence without period"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSentences(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("len got %d want %d (got=%v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] got %q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTokenizeWords(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"basic", "Hei vaan", []string{"Hei", "vaan"}},
		{"hyphen kept", "B1-tase", []string{"B1-tase"}},
		{"punct stripped", "Hei, vaan!", []string{"Hei", "vaan"}},
		{"quotes stripped", `"Hei"`, []string{"Hei"}},
		{"unicode quotes", "„Hei", []string{"Hei"}},
		{"digits", "vuonna 1990", []string{"vuonna", "1990"}},
		{"numbers with hyphen", "1990-luvulla", []string{"1990-luvulla"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenizeWords(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("len got %d want %d (got=%v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] got %q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTrimQuotesAndPunct(t *testing.T) {
	cases := map[string]string{
		`"Hei"`:    "Hei",
		`-Hei-`:    "Hei",
		`««Hei»»`:  "Hei",
		`Hei`:      "Hei",
		``:         "",
		`'`:        "",
	}
	for in, want := range cases {
		got := trimQuotesAndPunct(in)
		if got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
