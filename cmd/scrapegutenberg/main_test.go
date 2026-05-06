package main

import "testing"

// TestParseSearchPage exercises the simple regex-based ID extraction. The
// snippet below is a slimmed-down chunk of a real Gutenberg search results
// page (with non-result links interleaved to confirm we don't grab them).
func TestParseSearchPage(t *testing.T) {
	html := `
<html>
<head><a href="/about/">About</a></head>
<li><a href="/ebooks/7000">Kalevala</a></li>
<li><a href="/ebooks/11296">Seitsemän veljestä</a></li>
<a href="/ebooks/search/?query=l.fi&start_index=26">Next</a>
<a href="/ebooks/7000">Kalevala (dup)</a>
<li><a href="/ebooks/22006">Some other</a></li>
`
	got := parseSearchPage(html)
	want := []int{7000, 11296, 22006}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%d, want %d", i, got[i], want[i])
		}
	}
}

// TestParseGutenbergText: header parsing + body extraction between the
// canonical "*** START OF" / "*** END OF" markers. The fixture is a
// trimmed real-format Gutenberg file.
func TestParseGutenbergText(t *testing.T) {
	text := `The Project Gutenberg EBook of Foo, by Bar

Title: Foo
Author: Bar
Language: Finnish

*** START OF THIS PROJECT GUTENBERG EBOOK FOO ***

Body line 1.
Body line 2.

*** END OF THIS PROJECT GUTENBERG EBOOK FOO ***
End matter we should drop.
`
	meta, body, err := parseGutenbergText(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if meta.Title != "Foo" {
		t.Errorf("title: got %q", meta.Title)
	}
	if meta.Author != "Bar" {
		t.Errorf("author: got %q", meta.Author)
	}
	if meta.Language != "Finnish" {
		t.Errorf("language: got %q", meta.Language)
	}
	if !contains(body, "Body line 1.") || !contains(body, "Body line 2.") {
		t.Errorf("body missing expected lines: %q", body)
	}
	if contains(body, "End matter") {
		t.Errorf("body kept post-END text: %q", body)
	}
}

// TestParseGutenbergText_MissingMarkers: bail out cleanly rather than ship
// boilerplate. Many older books have non-standard markers; better to skip
// than to keep junk.
func TestParseGutenbergText_MissingMarkers(t *testing.T) {
	_, _, err := parseGutenbergText("Header but no markers")
	if err == nil {
		t.Error("expected error on missing markers")
	}
}

// TestLooksFinnish: ä/ö frequency + common-particle hits. English text
// fails; sample Finnish prose passes.
func TestLooksFinnish(t *testing.T) {
	en := "The quick brown fox jumps over the lazy dog. " // repeated to length
	for len(en) < 3000 {
		en += en
	}
	if looksFinnish(en) {
		t.Error("English text falsely detected as Finnish")
	}

	fi := "Kerrottiin että hän oli ollut siellä. Tämä on hyvin vanha tarina. "
	for len(fi) < 3000 {
		fi += fi
	}
	if !looksFinnish(fi) {
		t.Error("Finnish text not detected as Finnish")
	}
}

// TestApproxTokenCount: a sanity check that whitespace splitting matches
// what we'd intuit. Exact counts come later from the silver tagger.
func TestApproxTokenCount(t *testing.T) {
	got := approxTokenCount("yksi kaksi kolme   neljä\nviisi")
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

// contains is a small helper to keep the body-match assertions short.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) == 0 || indexOf(s, substr) >= 0))
}

// indexOf is a thin wrapper to avoid importing strings just for the test.
func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
