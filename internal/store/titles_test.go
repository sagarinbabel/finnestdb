package store

import (
	"strings"
	"testing"
)

// TestDeriveTitle pins the deterministic title-derivation rules described in
// DeriveTitle's doc comment: first clause/sentence, cleaned, cut at a
// sentence/clause boundary under MaxDerivedTitleLen, with degenerate and
// empty-input fallbacks. These examples double as the PR's derivation
// examples table.
func TestDeriveTitle(t *testing.T) {
	cases := []struct {
		name string
		text string
		lang string
		want string
	}{
		{
			name: "normal paragraph cuts at sentence end",
			text: "Kissa istuu ikkunalla. Se katselee ulos pihalle koko päivän.",
			lang: "FI",
			want: "Kissa istuu ikkunalla.",
		},
		{
			name: "normal paragraph with no early sentence end cuts at clause comma",
			text: "Kissa istuu ikkunalla ja katselee ulos, kun aurinko paistaa kirkkaasti taivaalla.",
			lang: "FI",
			want: "Kissa istuu ikkunalla ja katselee ulos…",
		},
		{
			name: "one-worder falls back unchanged",
			text: "Terve",
			lang: "FI",
			want: "Terve",
		},
		{
			name: "URL-only falls back to the URL itself (short enough, no clause structure)",
			text: "https://example.com/artikkel/123",
			lang: "FI",
			want: "https://example.com/artikkel/123",
		},
		{
			name: "long URL-only is hard-truncated as a degenerate single word",
			text: "https://example.com/" + strings.Repeat("a", 80),
			lang: "FI",
			want: "https://example.com/" + strings.Repeat("a", 39) + "…",
		},
		{
			name: "markdown heading marker is stripped",
			text: "# Suomen kielen historia\n\nSuomi on uralilainen kieli.",
			lang: "FI",
			want: "Suomen kielen historia",
		},
		{
			name: "markdown bullet marker is stripped",
			text: "- Ensimmäinen kohta tässä listassa on pitkä.",
			lang: "FI",
			want: "Ensimmäinen kohta tässä listassa on pitkä.",
		},
		{
			name: "surrounding quotes are stripped",
			text: `"Tervetuloa Suomeen" sanoi opas iloisesti kaikille matkustajille.`,
			lang: "FI",
			want: "Tervetuloa Suomeen\" sanoi opas iloisesti kaikille…",
		},
		{
			name: "digits-only falls back to first words",
			text: "1234 5678 9012 3456",
			lang: "FI",
			want: "1234 5678 9012 3456",
		},
		{
			name: "empty text falls back to Finnish default",
			text: "",
			lang: "FI",
			want: "Untitled Finnish text",
		},
		{
			name: "whitespace-only text falls back to Estonian default",
			text: "   \n\t  ",
			lang: "ET",
			want: "Untitled Estonian text",
		},
		{
			name: "Estonian text with diacritics",
			text: "Tere tulemast! See on eestikeelne tekst õ ä ö ü tähtedega.",
			lang: "ET",
			want: "Tere tulemast!",
		},
		{
			name: "exactly 59 chars, no punctuation, passes through unchanged",
			text: strings.Repeat("a", 59),
			lang: "FI",
			want: strings.Repeat("a", 59),
		},
		{
			name: "exactly 60 chars, no punctuation, passes through unchanged",
			text: strings.Repeat("a", 60),
			lang: "FI",
			want: strings.Repeat("a", 60),
		},
		{
			name: "61 chars, no punctuation, hard-truncates with ellipsis and stays <= 60",
			text: strings.Repeat("a", 61),
			lang: "FI",
			want: strings.Repeat("a", 59) + "…",
		},
		{
			name: "period inside a URL is not mistaken for a sentence end",
			text: "Check this out: https://example.com and also read more below.",
			lang: "FI",
			want: "Check this out: https…",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveTitle(tc.text, tc.lang)
			if got != tc.want {
				t.Errorf("DeriveTitle(%q, %q) = %q, want %q", tc.text, tc.lang, got, tc.want)
			}
			if n := len([]rune(got)); n > MaxDerivedTitleLen {
				t.Errorf("DeriveTitle(%q, %q) = %q has %d runes, want <= %d", tc.text, tc.lang, got, n, MaxDerivedTitleLen)
			}
		})
	}
}

// TestDeriveTitleNeverExceedsCap fuzzes rune-length invariants across a range
// of synthetic lengths so a future edit to the truncation logic can't
// silently regress the "always <= 60 chars" contract that the deck save
// modal and history rows both rely on for layout.
func TestDeriveTitleNeverExceedsCap(t *testing.T) {
	for n := 0; n <= 200; n++ {
		text := strings.Repeat("sana ", n) // repeated Finnish word + space
		got := DeriveTitle(text, "FI")
		if l := len([]rune(got)); l > MaxDerivedTitleLen {
			t.Fatalf("n=%d: DeriveTitle produced %q with %d runes, want <= %d", n, got, l, MaxDerivedTitleLen)
		}
	}
}

func TestDefaultTitleForLang(t *testing.T) {
	if got := DefaultTitleForLang("FI"); got != "Untitled Finnish text" {
		t.Errorf("DefaultTitleForLang(FI) = %q", got)
	}
	if got := DefaultTitleForLang("ET"); got != "Untitled Estonian text" {
		t.Errorf("DefaultTitleForLang(ET) = %q", got)
	}
}
