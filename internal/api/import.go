package api

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"finnestdb/internal/epub"
)

const (
	// Cap upload size at 16 MiB. EPUB books in the FI/ET corpus pipeline have
	// historically come in well under this. Plain-text files are even smaller.
	maxImportUploadBytes = 16 << 20
	// Cap returned text length so the client textarea (MAX_CHARS = 1_500_000)
	// receives at most that many characters. Frontend trims on its end too.
	maxImportTextChars = 1_500_000
)

type ImportExtractResponse struct {
	Text       string          `json:"text"`
	Filename   string          `json:"filename"`
	CharCount  int             `json:"char_count"`
	Truncated  bool            `json:"truncated"`
	BookTitle  string          `json:"book_title,omitempty"`
	BookAuthor string          `json:"book_author,omitempty"`
	Chapters   []ImportChapter `json:"chapters,omitempty"`
}

type ImportChapter struct {
	Title     string `json:"title"`
	Text      string `json:"text"`
	CharCount int    `json:"char_count"`
}

// HandleImportExtract accepts a multipart upload (form field "file") containing
// a .txt, .md, or .epub file and returns the extracted plain text. This lets
// the inspect/parse UI load text from a file the browser cannot read on its
// own (binary EPUB containers) while keeping the existing parse → save-deck
// flow intact.
func (a *API) HandleImportExtract(w http.ResponseWriter, r *http.Request) {
	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if auth == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !allowStateChangingRequest(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportUploadBytes)
	if err := r.ParseMultipartForm(maxImportUploadBytes); err != nil {
		http.Error(w, "Invalid upload (too large or malformed)", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file uploaded (form field 'file' is required)", http.StatusBadRequest)
		return
	}
	defer file.Close()

	text, chapters, meta, err := extractUploadedContent(file, header.Filename, header.Size)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// char_count reflects the source's ORIGINAL length, not the length of
	// the (possibly truncated) returned text. The frontend uses this for
	// the "X / 1,500,000" counter - if the book is bigger than the cap we
	// still want the user to see the real number so the overflow state is
	// honest. `truncated` tells the client the returned text was clipped.
	originalCharCount := utf8.RuneCountInString(text)
	truncated := false
	if originalCharCount > maxImportTextChars {
		text = truncateRunes(text, maxImportTextChars)
		chapters = truncateImportChapters(chapters, maxImportTextChars)
		truncated = true
	}

	writeJSON(w, http.StatusOK, ImportExtractResponse{
		Text:       text,
		Filename:   filepath.Base(header.Filename),
		CharCount:  originalCharCount,
		Truncated:  truncated,
		BookTitle:  meta.Title,
		BookAuthor: meta.Author,
		Chapters:   chapters,
	})
}

// extractUploadedContent reads file content and returns plain text plus, for
// EPUBs, per-chapter structure and book metadata. size is the declared
// content length from the multipart header; we read up to
// maxImportUploadBytes regardless.
func extractUploadedContent(file io.Reader, filename string, size int64) (string, []ImportChapter, epub.BookMetadata, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md":
		data, err := io.ReadAll(file)
		if err != nil {
			return "", nil, epub.BookMetadata{}, errors.New("could not read uploaded file")
		}
		if !utf8.Valid(data) {
			return "", nil, epub.BookMetadata{}, errors.New("file is not valid UTF-8")
		}
		return string(data), nil, epub.BookMetadata{}, nil
	case ".epub":
		// archive/zip needs ReaderAt + size, so buffer the whole upload first.
		data, err := io.ReadAll(file)
		if err != nil {
			return "", nil, epub.BookMetadata{}, errors.New("could not read uploaded EPUB")
		}
		chapters, err := epub.ExtractChaptersFromBytes(data)
		if err != nil {
			return "", nil, epub.BookMetadata{}, err
		}
		// Best-effort metadata read - a malformed OPF shouldn't fail the
		// whole import. zero-value metadata is fine.
		meta, _ := epub.ExtractMetadataFromBytes(data)
		out := make([]ImportChapter, 0, len(chapters))
		var sb strings.Builder
		for _, ch := range chapters {
			out = append(out, ImportChapter{
				Title:     ch.Title,
				Text:      ch.Text,
				CharCount: utf8.RuneCountInString(ch.Text),
			})
			sb.WriteString(ch.Text)
			sb.WriteByte('\n')
			sb.WriteByte('\n')
		}
		return strings.TrimSpace(sb.String()), out, meta, nil
	default:
		return "", nil, epub.BookMetadata{}, errors.New("unsupported file type: only .txt, .md, and .epub are accepted")
	}
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i]
		}
		count++
	}
	return s
}

func truncateImportChapters(chapters []ImportChapter, maxRunes int) []ImportChapter {
	if maxRunes <= 0 || len(chapters) == 0 {
		return nil
	}
	out := make([]ImportChapter, 0, len(chapters))
	remaining := maxRunes
	for _, ch := range chapters {
		if remaining <= 0 {
			break
		}
		chRunes := utf8.RuneCountInString(ch.Text)
		if chRunes > remaining {
			ch.Text = truncateRunes(ch.Text, remaining)
			ch.CharCount = utf8.RuneCountInString(ch.Text)
			out = append(out, ch)
			break
		}
		ch.CharCount = chRunes
		out = append(out, ch)
		remaining -= chRunes
	}
	return out
}
