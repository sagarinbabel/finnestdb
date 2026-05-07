package vfst

import (
	"testing"
)

// Minimal, synthetic VFST file bytes for unit tests. This avoids requiring
// any vendored upstream transducer blobs in-tree.
func minimalVFST(t *testing.T) []byte {
	t.Helper()
	// Header: two cookies + unweighted marker + reserved bytes.
	// Symbol table: 4 symbols: epsilon, "a", "ä", "[Ln]".
	// No real transition table is provided; unit tests below only verify
	// symbol-table parsing invariants.
	b := make([]byte, 0, 64)
	// header (16)
	b = append(b,
		0x6e, 0x3a, 0x01, 0x00, // cookie1 LE 0x00013A6E
		0xfa, 0x51, 0x03, 0x00, // cookie2 LE 0x000351FA
		0x00,                   // unweighted
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // reserved (7)
	)
	// symbolCount = 4 (uint16 LE)
	b = append(b, 0x04, 0x00)
	// symbol 0: epsilon (null)
	b = append(b, 0x00)
	// symbol 1: "a"
	b = append(b, 'a', 0x00)
	// symbol 2: "ä" (UTF-8 0xc3 0xa4)
	b = append(b, 0xc3, 0xa4, 0x00)
	// symbol 3: "[Ln]"
	b = append(b, '[', 'L', 'n', ']', 0x00)
	// pad to 8-byte boundary (transitionLen)
	for len(b)%8 != 0 {
		b = append(b, 0x00)
	}
	// Add 8 bytes so transitionStart points inside the slice.
	b = append(b, make([]byte, 8)...)
	return b
}

func TestOpen_ParsesHeaderAndSymbolTable(t *testing.T) {
	tr, err := OpenBytes(minimalVFST(t))
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	if tr.firstNormalChar == 0 {
		t.Fatal("firstNormalChar is 0; symbol table did not parse normal-symbol region")
	}
	if tr.firstMultiChar == 0 {
		t.Fatal("firstMultiChar is 0; symbol table did not detect any multi-char tag (looking for '[...]' entries)")
	}
	if tr.firstNormalChar >= tr.firstMultiChar {
		t.Fatalf("expected firstNormalChar (%d) < firstMultiChar (%d)", tr.firstNormalChar, tr.firstMultiChar)
	}
	if tr.transitionStart == 0 {
		t.Fatal("transitionStart is 0; symbol table parsing didn't advance the cursor")
	}
	// The Finnish FST should have a Latin letter and the morphology bracket
	// vocabulary at minimum.
	if _, ok := tr.stringToSymbol['a']; !ok {
		t.Error("expected 'a' to be in the input alphabet")
	}
	if _, ok := tr.stringToSymbol['ä']; !ok {
		t.Error("expected 'ä' (front vowel) to be in the input alphabet")
	}
}

func TestAnalyze_UnknownInputReturnsEmpty(t *testing.T) {
	tr, err := OpenBytes(minimalVFST(t))
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	if got := tr.Analyze("xyzzy123"); len(got) != 0 {
		t.Errorf("expected no analyses for non-Finnish input, got %d: %v", len(got), got)
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	tr, err := OpenBytes(minimalVFST(t))
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	// Empty input: behavior is implementation-defined. We just want it not
	// to panic. Either no analyses or one degenerate analysis is fine.
	_ = tr.Analyze("")
}
