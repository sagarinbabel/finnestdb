package textfilter

import (
	"strings"
	"testing"
)

func TestShouldSkipEPUBResource(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "OEBPS/nav.xhtml", want: true},
		{name: "OPS/Text/toc01.xhtml", want: true},
		{name: "OPS/frontmatter/titlepage.xhtml", want: true},
		{name: "OPS/Text/chapter-01.xhtml", want: false},
		{name: "OPS/Text/part0001.xhtml", raw: `<html><body><nav epub:type="toc"><ol><li>Chapter</li></ol></nav></body></html>`, want: true},
	}
	for _, tt := range tests {
		if got := ShouldSkipEPUBResource(tt.name, []byte(tt.raw)); got != tt.want {
			t.Fatalf("ShouldSkipEPUBResource(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCleanEPUBTextDropsResidueLines(t *testing.T) {
	in := strings.Join([]string{
		"Sisällys",
		"",
		"Luku 1",
		"",
		"Tämä on oikea esimerkkilause.",
		"ISBN 978-1-234-56789-0",
		"Kiitos.",
	}, "\n")
	got := CleanEPUBText(in)
	if strings.Contains(got, "Sisällys") || strings.Contains(got, "Luku 1") || strings.Contains(got, "ISBN") {
		t.Fatalf("CleanEPUBText kept residue:\n%s", got)
	}
	if !strings.Contains(got, "Tämä on oikea esimerkkilause.") || !strings.Contains(got, "Kiitos.") {
		t.Fatalf("CleanEPUBText dropped valid prose:\n%s", got)
	}
}

func TestIsUserFriendlySentence(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "Tämä on hyvä esimerkkilause.", want: true},
		{text: "Kiitos.", want: true},
		{text: "ISBN 978-1-234-56789-0", want: false},
		{text: "J. R. R. Tolkien", want: false},
		{text: "Harry Potter ja Azkabanin vanki", want: false},
		{text: "Luku 12", want: false},
		{text: "Sisällys", want: false},
		{text: "https://example.test", want: false},
	}
	for _, tt := range tests {
		if got := IsUserFriendlySentence(tt.text); got != tt.want {
			t.Fatalf("IsUserFriendlySentence(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}
