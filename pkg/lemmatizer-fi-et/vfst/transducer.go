package vfst

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

const (
	cookie1       uint32 = 0x00013A6E
	cookie2       uint32 = 0x000351FA
	headerLen            = 16
	transitionLen        = 8
	finalSymbol          = 0xFFFF
	maxLoopCount         = 100000

	flagValueNeutral uint16 = 0
	flagValueAny     uint16 = 1
)

type operation uint8

const (
	opP operation = iota // positive set: set feature := value
	opC                  // clear:        set feature := neutral
	opU                  // unify:        if neutral, set; else require equal
	opR                  // require:      require feature == value (or != neutral if value is "any")
	opD                  // disallow:     require feature != value (or == neutral if value is "any")
)

type opFeatureValue struct {
	op      operation
	feature uint16
	value   uint16
}

// Transducer is an in-memory unweighted VFST. Read-only and safe for
// concurrent calls to Analyze (each call creates its own traversal
// configuration; no shared mutable state).
type Transducer struct {
	data []byte // entire file, kept resident

	symbolToString    []string         // index -> UTF-8 symbol string
	symbolToDiacritic []opFeatureValue // index -> flag diacritic op (only meaningful for index < firstNormalChar)
	symbolToRune      []rune           // first rune of each symbol; 0 for empty/multi-rune
	stringToSymbol    map[rune]uint16  // single-rune symbols only

	firstNormalChar           uint16
	firstMultiChar            uint16
	flagDiacriticFeatureCount uint16
	unknownSymbol             uint16

	transitionStart int // byte offset in data where the transition table begins
}

// Open reads a VFST file into memory and prepares the symbol table and
// transition pointer. The returned Transducer holds a reference to the
// file contents; call Close when done.
func Open(path string) (*Transducer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(data)
}

// OpenBytes is like Open but takes the pre-loaded VFST contents
// directly. Useful when the .vfst is embedded via go:embed. The
// Transducer aliases the byte slice; do not modify it after the call.
func OpenBytes(data []byte) (*Transducer, error) {
	return parse(data)
}

// Close releases the in-memory file backing. Subsequent calls to
// Analyze are not safe.
func (t *Transducer) Close() error {
	t.data = nil
	return nil
}

func parse(data []byte) (*Transducer, error) {
	if len(data) < headerLen+2 {
		return nil, fmt.Errorf("vfst: file too short (%d bytes)", len(data))
	}
	c1 := binary.LittleEndian.Uint32(data[0:4])
	c2 := binary.LittleEndian.Uint32(data[4:8])
	if c1 != cookie1 || c2 != cookie2 {
		// Reverse-byte-order files exist in upstream but are vanishingly rare
		// today (all modern targets are LE). We could byte-swap here later if
		// needed; for now reject.
		return nil, fmt.Errorf("vfst: unrecognized cookie %#x %#x (file not in native byte order?)", c1, c2)
	}
	if data[8] != 0x00 {
		return nil, fmt.Errorf("vfst: weighted transducer (byte 8 = %#x); this package supports unweighted only", data[8])
	}

	t := &Transducer{data: data}
	if err := t.parseSymbolTable(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Transducer) parseSymbolTable() error {
	off := headerLen
	if off+2 > len(t.data) {
		return fmt.Errorf("vfst: truncated before symbol count")
	}
	symbolCount := binary.LittleEndian.Uint16(t.data[off : off+2])
	off += 2

	t.symbolToString = make([]string, symbolCount)
	t.symbolToDiacritic = make([]opFeatureValue, symbolCount)
	t.symbolToRune = make([]rune, symbolCount)
	t.stringToSymbol = make(map[rune]uint16, symbolCount)

	// Flag-diacritic feature/value interning. The empty string and "@" are
	// pre-bound to neutral / any respectively, matching libvoikko.
	features := map[string]uint16{}
	values := map[string]uint16{"": flagValueNeutral, "@": flagValueAny}

	for i := uint16(0); i < symbolCount; i++ {
		if i == 0 {
			// epsilon: just a null byte
			if off >= len(t.data) || t.data[off] != 0 {
				return fmt.Errorf("vfst: symbol 0 must be a single null byte")
			}
			t.symbolToString[0] = ""
			off++
			continue
		}
		end := bytes.IndexByte(t.data[off:], 0)
		if end < 0 {
			return fmt.Errorf("vfst: unterminated symbol %d", i)
		}
		sym := string(t.data[off : off+end])
		off += end + 1
		t.symbolToString[i] = sym

		if t.firstNormalChar == 0 {
			if len(sym) > 0 && sym[0] == '@' {
				ofv, err := parseDiacritic(sym, features, values)
				if err != nil {
					return err
				}
				t.symbolToDiacritic[i] = ofv
			} else {
				t.firstNormalChar = i
			}
		} else if t.firstMultiChar == 0 && len(sym) > 0 && sym[0] == '[' {
			t.firstMultiChar = i
		}

		// Single-rune symbols become character→symbol lookups for input matching.
		// The C++ code uses `ucs4Symbol[0]` (first wide char) when wcslen == 1;
		// in Go we want the symbol to be exactly one rune.
		runes := []rune(sym)
		if len(runes) >= 1 {
			t.symbolToRune[i] = runes[0]
		}
		if t.firstNormalChar > 0 && t.firstMultiChar == 0 && len(runes) == 1 {
			t.stringToSymbol[runes[0]] = i
		}
	}

	t.unknownSymbol = symbolCount
	t.flagDiacriticFeatureCount = uint16(len(features))

	// Pad to 8-byte boundary, then transitions begin.
	if rem := off % transitionLen; rem != 0 {
		off += transitionLen - rem
	}
	t.transitionStart = off
	return nil
}

func parseDiacritic(symbol string, features, values map[string]uint16) (opFeatureValue, error) {
	if len(symbol) <= 4 {
		return opFeatureValue{}, fmt.Errorf("vfst: malformed flag diacritic %q", symbol)
	}
	var op operation
	switch symbol[1] {
	case 'P':
		op = opP
	case 'C':
		op = opC
	case 'U':
		op = opU
	case 'R':
		op = opR
	case 'D':
		op = opD
	default:
		// libvoikko falls back to D on unrecognised opcodes; we match that.
		op = opD
	}
	// Strip leading "@X." and trailing "@".
	body := symbol[3 : len(symbol)-1]
	feature, value := body, "@"
	if dot := bytes.IndexByte([]byte(body), '.'); dot >= 0 {
		feature = body[:dot]
		value = body[dot+1:]
	}
	fid, ok := features[feature]
	if !ok {
		fid = uint16(len(features))
		features[feature] = fid
	}
	vid, ok := values[value]
	if !ok {
		vid = uint16(len(values))
		values[value] = vid
	}
	return opFeatureValue{op: op, feature: fid, value: vid}, nil
}

// transition layout helpers. We keep transitions in raw bytes and decode
// per-access; cheap and avoids allocating a parallel struct table. A 4 MB
// FST contains ~500k transitions, so eager decoding would 2-3x memory.
type transition struct {
	symIn       uint16
	symOut      uint16
	targetState uint32
	moreTrans   uint8
}

func (t *Transducer) decodeTransitionAt(off int) transition {
	// off is the byte offset within t.data
	b := t.data[off : off+transitionLen]
	transInfo := binary.LittleEndian.Uint32(b[4:8])
	return transition{
		symIn:       binary.LittleEndian.Uint16(b[0:2]),
		symOut:      binary.LittleEndian.Uint16(b[2:4]),
		targetState: transInfo & 0x00FFFFFF,
		moreTrans:   uint8(transInfo >> 24),
	}
}

func (t *Transducer) decodeTransitionAtIndex(idx uint32) transition {
	return t.decodeTransitionAt(t.transitionStart + int(idx)*transitionLen)
}

// getMaxTc replicates UnweightedTransducer.cpp's helper: a state's
// transition count comes from its head transition's moreTransitions
// byte; if that byte is 255 the real count is in the OverflowCell that
// follows.
func (t *Transducer) getMaxTc(stateIndex uint32) uint32 {
	head := t.decodeTransitionAtIndex(stateIndex)
	maxTc := uint32(head.moreTrans)
	if maxTc == 255 {
		oc := t.decodeTransitionAt(t.transitionStart + (int(stateIndex)+1)*transitionLen)
		// OverflowCell.moreTransitions occupies bytes 0-3, but we just
		// reuse the transition decoder's 8-byte slice; its symIn+symOut
		// pair is the low 32 bits of the cell's first uint32.
		full := uint32(oc.symIn) | uint32(oc.symOut)<<16
		maxTc = full + 1
	}
	return maxTc
}
