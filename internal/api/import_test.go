package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newMultipartUpload(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.Copy(fw, bytes.NewReader(content)); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

func buildMinimalEPUB(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("OEBPS/ch01.xhtml")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func buildEPUBWithOPF(t *testing.T, opf, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"OEBPS/content.opf":     opf,
		"OEBPS/Text/ch01.xhtml": body,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func postImportExtract(t *testing.T, mux *http.ServeMux, cookies []*http.Cookie, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := newMultipartUpload(t, filename, content)
	req := httptest.NewRequest(http.MethodPost, "/api/import/extract", body)
	req.Header.Set("Content-Type", contentType)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHandleImportExtractRequiresAuth(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	body, contentType := newMultipartUpload(t, "x.txt", []byte("hello"))
	req := httptest.NewRequest(http.MethodPost, "/api/import/extract", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%q", rec.Code, rec.Body.String())
	}
}

func TestHandleImportExtractRejectsGet(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "import-get@example.com")

	req := httptest.NewRequest(http.MethodGet, "/api/import/extract", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandleImportExtractTxt(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "import-txt@example.com")

	rec := postImportExtract(t, mux, cookies, "kissa.txt", []byte("Kissa juoksee.\n"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", rec.Code, rec.Body.String())
	}
	var resp ImportExtractResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.Text, "Kissa juoksee") {
		t.Fatalf("text missing source: %q", resp.Text)
	}
	if resp.Filename != "kissa.txt" {
		t.Fatalf("filename = %q, want kissa.txt", resp.Filename)
	}
	if resp.Truncated {
		t.Fatalf("expected not truncated")
	}
}

func TestHandleImportExtractEPUB(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "import-epub@example.com")

	epubBytes := buildMinimalEPUB(t, `<html><body><h1>Luku 1</h1><p>Kalevala alkaa täältä.</p></body></html>`)
	rec := postImportExtract(t, mux, cookies, "kalevala.epub", epubBytes)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", rec.Code, rec.Body.String())
	}
	var resp ImportExtractResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.Text, "Kalevala alkaa täältä") {
		t.Fatalf("text missing EPUB content: %q", resp.Text)
	}
	if len(resp.Chapters) != 1 {
		t.Fatalf("got %d chapters, want 1; resp=%+v", len(resp.Chapters), resp)
	}
	if resp.Chapters[0].Title != "Luku 1" {
		t.Fatalf("chapter title = %q, want %q", resp.Chapters[0].Title, "Luku 1")
	}
	if !strings.Contains(resp.Chapters[0].Text, "Kalevala alkaa täältä") {
		t.Fatalf("chapter[0].Text missing body: %q", resp.Chapters[0].Text)
	}
}

func TestHandleImportExtractEPUBSurfacesBookMetadata(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "import-epub-meta@example.com")

	opf := `<?xml version="1.0"?><package><metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
		<dc:title>Väike prints</dc:title>
		<dc:creator opf:role="aut">Antoine de Saint-Exupéry</dc:creator>
	</metadata></package>`
	body := `<html><body><p>Body.</p></body></html>`
	rec := postImportExtract(t, mux, cookies, "vp.epub", buildEPUBWithOPF(t, opf, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", rec.Code, rec.Body.String())
	}
	var resp ImportExtractResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.BookTitle != "Väike prints" {
		t.Fatalf("BookTitle = %q, want %q", resp.BookTitle, "Väike prints")
	}
	if resp.BookAuthor != "Antoine de Saint-Exupéry" {
		t.Fatalf("BookAuthor = %q, want %q", resp.BookAuthor, "Antoine de Saint-Exupéry")
	}
}

func TestHandleImportExtractTxtHasNoChapters(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "import-txt-nochap@example.com")

	rec := postImportExtract(t, mux, cookies, "x.txt", []byte("plain text"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", rec.Code, rec.Body.String())
	}
	var resp ImportExtractResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Chapters) != 0 {
		t.Fatalf("plain .txt should have no chapters, got %d", len(resp.Chapters))
	}
}

func TestTruncateImportChaptersKeepsPayloadWithinParseCap(t *testing.T) {
	chapters := []ImportChapter{
		{Title: "One", Text: "12345", CharCount: 5},
		{Title: "Two", Text: "abcdef", CharCount: 6},
		{Title: "Three", Text: "ignored", CharCount: 7},
	}

	got := truncateImportChapters(chapters, 8)
	if len(got) != 2 {
		t.Fatalf("got %d chapters, want 2: %+v", len(got), got)
	}
	if got[0].Text != "12345" || got[0].CharCount != 5 {
		t.Fatalf("first chapter changed unexpectedly: %+v", got[0])
	}
	if got[1].Text != "abc" || got[1].CharCount != 3 {
		t.Fatalf("second chapter = %+v, want truncated text abc with count 3", got[1])
	}
	total := 0
	for _, ch := range got {
		total += ch.CharCount
	}
	if total > 8 {
		t.Fatalf("truncated chapters exceed cap: %d", total)
	}
}

func TestHandleImportExtractRejectsUnsupportedExtension(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "import-bad@example.com")

	rec := postImportExtract(t, mux, cookies, "doc.pdf", []byte("%PDF-1.4\n"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%q", rec.Code, rec.Body.String())
	}
}

func TestHandleImportExtractRejectsInvalidEPUB(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "import-invalid@example.com")

	rec := postImportExtract(t, mux, cookies, "broken.epub", []byte("not a zip"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%q", rec.Code, rec.Body.String())
	}
}
