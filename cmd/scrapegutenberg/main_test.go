package main

import (
	"strings"
	"testing"
)

// TestParseSearchPage exercises the simple regex-based ID extraction. The
// snippet below is a slimmed-down chunk of a real Gutenberg search results
// page (with non-result links interleaved to confirm we don't grab them).
func TestParseSearchPage(t *testing.T) {
	html := `
<html>
<head><a href="/about/">About</a></head>
<li><a href="/ebooks/22006">Some other</a></li>
<li><a href="/ebooks/11296">Seitsemän veljestä</a></li>
<a href="/ebooks/search/?query=l.fi&start_index=26">Next</a>
<a href="/ebooks/22006">Some other (dup)</a>
<li><a href="/ebooks/7000">Kalevala</a></li>
`
	got := parseSearchPage(html)
	want := []int{22006, 11296, 7000}
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

func TestParseGutenbergText_StripsPostStartMetadata(t *testing.T) {
	text := `The Project Gutenberg EBook of Foo, by Bar

Title: Foo
Author: Bar
Language: Finnish

*** START OF THIS PROJECT GUTENBERG EBOOK FOO ***

Produced by Somebody and PG Distributed Proofreaders.
Continuation credit line.
Credits: Another credit
Language: Finnish

Ensimmäinen oikea rivi.

*** END OF THIS PROJECT GUTENBERG EBOOK FOO ***
`
	_, body, err := parseGutenbergText(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.HasPrefix(body, "Ensimmäinen oikea rivi.") {
		t.Fatalf("body kept leading metadata: %q", body)
	}
	if strings.Contains(body, "Produced by") || strings.Contains(body, "Credits:") || strings.Contains(body, "Language:") {
		t.Fatalf("body kept Gutenberg metadata: %q", body)
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

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
