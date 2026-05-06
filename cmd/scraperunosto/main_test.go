package main

import "testing"

func TestStripHTML_ParagraphAndBreaks(t *testing.T) {
	in := `<p>Sanos mitä Pohjolaan<br />
kevät tuopi tullessaan?<br />
Kukkia, kukkia,<br />
paljon kukkia!</p>
<p>Syys kun sitten saapuvi,<br />
mitäs hän tuo lahjaksi?</p>`
	got := stripHTML(in)
	if !contains(got, "Sanos") || !contains(got, "Kukkia") || !contains(got, "Syys") {
		t.Errorf("missing content lines: %q", got)
	}
	if contains(got, "<p>") || contains(got, "<br") || contains(got, "</p>") {
		t.Errorf("HTML not fully stripped: %q", got)
	}
	// Paragraph break preserved (allows the silvertag sentence splitter to fire).
	if !contains(got, "\n\n") {
		t.Errorf("paragraph break not preserved: %q", got)
	}
}

func TestDecodeEntity(t *testing.T) {
	cases := map[string]string{
		"&amp;":   "&",
		"&lt;":    "<",
		"&quot;":  "\"",
		"&nbsp;":  " ",
		"&#228;":  "ä", // numeric entity for ä
		"&#xE4;":  "ä", // hex entity for ä
		"&unknown;": "&unknown;", // pass through
	}
	for in, want := range cases {
		got := decodeEntity(in)
		if got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestStripHTML_HandlesEntities(t *testing.T) {
	in := `M&auml;k&auml;l&auml;n maja, &amp; sis&auml;ll&auml;`
	got := stripHTML(in)
	// Common entities should be normalized; named ones we don't handle
	// pass through (no crash).
	if !contains(got, "&") {
		t.Errorf("&amp; should decode to &: %q", got)
	}
}

func TestApproxTokenCount(t *testing.T) {
	if approxTokenCount("yksi kaksi   kolme\n\nneljä") != 4 {
		t.Errorf("token count")
	}
	if approxTokenCount("") != 0 {
		t.Errorf("empty token count")
	}
}

// contains is the same helper used in the gutenberg scraper test.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
