package epub

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// buildEPUB returns a minimal zip archive emulating an EPUB. files maps
// archive paths to raw content.
func buildEPUB(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestExtractTextConcatenatesChaptersInOrder(t *testing.T) {
	data := buildEPUB(t, map[string]string{
		"OEBPS/ch02.xhtml": `<html><body><p>Toinen luku.</p></body></html>`,
		"OEBPS/ch01.xhtml": `<html><body><p>Ensimmäinen luku.</p></body></html>`,
		"OEBPS/styles.css": `body { color: red; }`,
		"META-INF/container.xml": `<container/>`,
	})

	text, err := ExtractTextFromBytes(data)
	if err != nil {
		t.Fatalf("ExtractTextFromBytes: %v", err)
	}

	ch1Idx := strings.Index(text, "Ensimmäinen luku.")
	ch2Idx := strings.Index(text, "Toinen luku.")
	if ch1Idx < 0 || ch2Idx < 0 {
		t.Fatalf("expected both chapters, got:\n%s", text)
	}
	if ch1Idx > ch2Idx {
		t.Fatalf("chapters out of order:\n%s", text)
	}
	if strings.Contains(text, "color: red") {
		t.Fatalf("CSS leaked into extracted text:\n%s", text)
	}
}

func TestExtractTextStripsTagsAndDecodesEntities(t *testing.T) {
	data := buildEPUB(t, map[string]string{
		"a.xhtml": `<html><head><title>x</title></head><body>` +
			`<p>Hello&nbsp;world &amp; goodbye &mdash; ok.</p>` +
			`<script>var x = 1;</script>` +
			`<style>p { color: blue; }</style>` +
			`<!-- a comment --></body></html>`,
	})

	text, err := ExtractTextFromBytes(data)
	if err != nil {
		t.Fatalf("ExtractTextFromBytes: %v", err)
	}
	if !strings.Contains(text, "Hello world & goodbye — ok.") {
		t.Fatalf("entities not decoded:\n%s", text)
	}
	if strings.Contains(text, "var x = 1") || strings.Contains(text, "color: blue") {
		t.Fatalf("script/style leaked:\n%s", text)
	}
	if strings.Contains(text, "a comment") {
		t.Fatalf("comment leaked:\n%s", text)
	}
}

func TestExtractTextRejectsArchiveWithoutHTML(t *testing.T) {
	data := buildEPUB(t, map[string]string{
		"meta.txt": "no html here",
	})
	if _, err := ExtractTextFromBytes(data); err == nil {
		t.Fatalf("expected error on archive without HTML")
	}
}

func TestExtractTextRejectsNonZipBytes(t *testing.T) {
	if _, err := ExtractTextFromBytes([]byte("not a zip")); err == nil {
		t.Fatalf("expected error on non-zip bytes")
	}
}

func TestNaturalLessOrdersNumericSuffixes(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"ch2.xhtml", "ch10.xhtml", true},   // numeric, not lexical
		{"ch10.xhtml", "ch2.xhtml", false},
		{"ch9.xhtml", "ch10.xhtml", true},
		{"a.xhtml", "b.xhtml", true},
		{"ch1.xhtml", "ch1.xhtml", false},
		{"OEBPS/x-2.xhtml", "OEBPS/x-10.xhtml", true},
	}
	for _, c := range cases {
		got := naturalLess(c.a, c.b)
		if got != c.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestExtractMetadataFromOPF(t *testing.T) {
	opf := `<?xml version="1.0"?><package><metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
		<dc:creator/>
		<dc:title>Väike prints</dc:title>
		<dc:creator opf:role="aut">Antoine de Saint-Exupéry</dc:creator>
	</metadata></package>`
	data := buildEPUB(t, map[string]string{
		"OEBPS/content.opf":       opf,
		"OEBPS/Text/ch01.xhtml":   `<html><body><p>Body.</p></body></html>`,
	})

	meta, err := ExtractMetadataFromBytes(data)
	if err != nil {
		t.Fatalf("ExtractMetadataFromBytes: %v", err)
	}
	if meta.Title != "Väike prints" {
		t.Fatalf("Title = %q, want %q", meta.Title, "Väike prints")
	}
	if meta.Author != "Antoine de Saint-Exupéry" {
		t.Fatalf("Author = %q, want %q", meta.Author, "Antoine de Saint-Exupéry")
	}
}

func TestExtractMetadataMissingOPF(t *testing.T) {
	data := buildEPUB(t, map[string]string{
		"OEBPS/Text/ch01.xhtml": `<html><body><p>Body.</p></body></html>`,
	})
	meta, err := ExtractMetadataFromBytes(data)
	if err != nil {
		t.Fatalf("ExtractMetadataFromBytes: %v", err)
	}
	if meta.Title != "" || meta.Author != "" {
		t.Fatalf("expected empty metadata, got %+v", meta)
	}
}

func TestExtractChaptersDropsHeadTitleAndDRMFile(t *testing.T) {
	data := buildEPUB(t, map[string]string{
		"OEBPS/Text/ch1.xhtml": `<html><head><title>filename-leak</title></head><body><h1>Chapter One</h1><p>Body sentence.</p></body></html>`,
		"OEBPS/rrOwnerInfo/rrOwnerInfo.html": `<html><body><p>iBooks DRM marker — should not appear.</p></body></html>`,
	})

	chapters, err := ExtractChaptersFromBytes(data)
	if err != nil {
		t.Fatalf("ExtractChaptersFromBytes: %v", err)
	}
	if len(chapters) != 1 {
		t.Fatalf("got %d chapters, want 1 (DRM file should be skipped)", len(chapters))
	}
	if strings.Contains(chapters[0].Text, "filename-leak") {
		t.Fatalf("chapter text leaked <title>: %q", chapters[0].Text)
	}
	if !strings.Contains(chapters[0].Text, "Body sentence.") {
		t.Fatalf("chapter text missing body: %q", chapters[0].Text)
	}
}

func TestExtractChaptersTitlePreference(t *testing.T) {
	data := buildEPUB(t, map[string]string{
		// Prefers <h1>
		"OEBPS/ch01.xhtml": `<html><head><title>Title-tag</title></head><body><h1>Chapter One</h1><p>Body.</p></body></html>`,
		// Falls back to <title>
		"OEBPS/ch02.xhtml": `<html><head><title>Otsikko Kaksi</title></head><body><p>Vain runkoa.</p></body></html>`,
		// Falls back to filename basename
		"OEBPS/ch03.xhtml": `<html><body><p>Pelkkä leipäteksti.</p></body></html>`,
	})

	chapters, err := ExtractChaptersFromBytes(data)
	if err != nil {
		t.Fatalf("ExtractChaptersFromBytes: %v", err)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(chapters))
	}
	if chapters[0].Title != "Chapter One" {
		t.Fatalf("chapter[0].Title = %q, want %q", chapters[0].Title, "Chapter One")
	}
	if chapters[1].Title != "Otsikko Kaksi" {
		t.Fatalf("chapter[1].Title = %q, want %q", chapters[1].Title, "Otsikko Kaksi")
	}
	if chapters[2].Title != "ch03" {
		t.Fatalf("chapter[2].Title = %q, want %q", chapters[2].Title, "ch03")
	}
	if !strings.Contains(chapters[0].Text, "Body.") {
		t.Fatalf("chapter[0].Text missing body: %q", chapters[0].Text)
	}
}
