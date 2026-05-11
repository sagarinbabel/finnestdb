package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ── Byte-size flag parsing ──────────────────────────────────────────────────

// parseByteSize parses values like "6GB", "10MiB", "1024", "0".
// Empty string and "0" mean "no budget" and return 0.
//
// Suffixes (binary forms preferred for engineering, decimal forms accepted
// because users say "6 GB" and don't care about base-2 vs base-10):
//
//	KiB / MiB / GiB / TiB  → 1024-based
//	KB  / MB  / GB  / TB   → 1000-based
//	B  / no suffix         → bytes
//
// Returns (bytes, error). Error messages quote the bad input so the
// operator can spot a typo at the flag boundary instead of getting a
// silent zero.
func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	type unit struct {
		suffix string
		mult   float64
	}
	// Order matters: longer suffixes first so "MiB" doesn't match as "M".
	units := []unit{
		{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
		{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"KB", 1e3},
		{"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			num := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			n, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
			}
			if n < 0 {
				return 0, fmt.Errorf("negative byte size: %q", s)
			}
			return int64(n * u.mult), nil
		}
	}
	// No suffix → plain bytes
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q (use suffix KB/MB/GB/KiB/MiB/GiB or plain integer): %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("negative byte size: %q", s)
	}
	return n, nil
}

// formatBytes returns "1.23 GB" / "456 MB" / "789 KB" / "12 B" — always
// 1000-based to match the user-facing flag suffixes.
func formatBytes(n int64) string {
	switch {
	case n >= 1e12:
		return fmt.Sprintf("%.2f TB", float64(n)/1e12)
	case n >= 1e9:
		return fmt.Sprintf("%.2f GB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.2f MB", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.2f KB", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ── Capped TSV writer ───────────────────────────────────────────────────────
//
// `cappedTSVWriter` is a TSV writer that stops accepting rows once the
// cumulative byte budget would be exceeded. Used for the user-friendly
// sentence + wordlist exports where the operator hands us a learner-facing
// budget (e.g. "6GB of sentences_user_friendly.tsv max").
//
// **Encode-then-measure** (no truncation):
//
// Each candidate row is first serialized through a private csv.Writer
// into a `bytes.Buffer`. We then know the exact byte length the row
// would add to the file. If that fits under the budget, we copy the
// encoded bytes verbatim to the underlying buffered writer and update
// the running counter. If it doesn't fit, we drop the row and stop
// accepting more. No file truncation — the file always ends on a row
// boundary. The previous "estimate + os.File.Truncate" path could cut
// mid-row when the estimate underran (csv quoting on tab-bearing
// fields), corrupting the final TSV; that's gone.
//
// Per-row encoding cost: ~one extra small alloc per row plus a memcpy.
// On 70 M ET sentences this is in the seconds, not minutes — well
// worth the corruption-proofing.

type cappedTSVWriter struct {
	f              *os.File
	buf            *bufio.Writer
	probeBuf       bytes.Buffer
	probeCSV       *csv.Writer
	budget         int64
	bytesAccepted  int64 // exact file bytes written (header + accepted rows)
	rowsWritten    int64
	rowsRejected   int64
	capHit         bool
	closed         bool
}

func newCappedTSVWriter(path string, header []string, budget int64) (*cappedTSVWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := &cappedTSVWriter{
		f:      f,
		buf:    bufio.NewWriterSize(f, 1<<20),
		budget: budget,
	}
	w.probeCSV = csv.NewWriter(&w.probeBuf)
	w.probeCSV.Comma = '\t'

	// Header counts toward the budget. If the header alone overshoots,
	// it still gets written (writers without a header are useless), but
	// the writer marks capHit so every subsequent row is rejected.
	headerBytes, ok := w.encodeRow(header)
	if !ok {
		f.Close()
		return nil, fmt.Errorf("encode header: %w", w.probeCSV.Error())
	}
	if _, err := w.buf.Write(headerBytes); err != nil {
		f.Close()
		return nil, err
	}
	w.bytesAccepted = int64(len(headerBytes))
	if budget > 0 && w.bytesAccepted >= budget {
		w.capHit = true
	}
	return w, nil
}

// encodeRow runs the row through the private csv.Writer and returns the
// encoded bytes (a slice into the probe buffer — must be consumed before
// the next encode). Returns false on csv.Writer failure.
func (w *cappedTSVWriter) encodeRow(row []string) ([]byte, bool) {
	w.probeBuf.Reset()
	if err := w.probeCSV.Write(row); err != nil {
		return nil, false
	}
	w.probeCSV.Flush()
	if err := w.probeCSV.Error(); err != nil {
		return nil, false
	}
	return w.probeBuf.Bytes(), true
}

// Write appends a row if the encoded length fits under the budget.
// Returns true on accept, false on reject (budget hit or csv error).
func (w *cappedTSVWriter) Write(row []string) bool {
	if w.capHit {
		w.rowsRejected++
		return false
	}
	enc, ok := w.encodeRow(row)
	if !ok {
		w.capHit = true
		w.rowsRejected++
		return false
	}
	rowBytes := int64(len(enc))
	if w.budget > 0 && w.bytesAccepted+rowBytes > w.budget {
		w.capHit = true
		w.rowsRejected++
		return false
	}
	if _, err := w.buf.Write(enc); err != nil {
		w.capHit = true
		w.rowsRejected++
		return false
	}
	w.bytesAccepted += rowBytes
	w.rowsWritten++
	return true
}

// CapHit reports whether the writer stopped accepting rows.
func (w *cappedTSVWriter) CapHit() bool { return w.capHit }

// EstimatedBytes returns the running byte total (exact, since we
// encode-then-measure). Name preserved for backward compatibility.
func (w *cappedTSVWriter) EstimatedBytes() int64 { return w.bytesAccepted }

// Close flushes the writer and returns (actualFileBytes, capHit, err).
// No truncation: the file always ends on a row boundary because we
// never wrote a row that overshot.
func (w *cappedTSVWriter) Close() (int64, bool, error) {
	if w.closed {
		return w.bytesAccepted, w.capHit, nil
	}
	w.closed = true
	if err := w.buf.Flush(); err != nil {
		_ = w.f.Close()
		return w.bytesAccepted, w.capHit, err
	}
	if err := w.f.Close(); err != nil {
		return w.bytesAccepted, w.capHit, err
	}
	return w.bytesAccepted, w.capHit, nil
}
