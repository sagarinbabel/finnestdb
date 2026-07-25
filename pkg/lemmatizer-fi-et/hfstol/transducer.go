package hfstol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

const (
	hfstMagic                  = "HFST"
	transducerHeaderLen        = 2 + 2 + 4 + 4 + 4 + 4 + 9*4 // 56 bytes
	indexEntrySize             = 6                           // uint16 + uint32
	unweightedTransSize        = 8                           // uint16 + uint16 + uint32
	weightedTransSize          = 12                          // uint16 + uint16 + uint32 + float32
	noSymbol            uint16 = 0xFFFF
	noTableIndex        uint32 = 0xFFFFFFFF
	transTableStart     uint32 = 0x80000000
)

// flag diacritic operations, matching libvoikko / HFST conventions.
type opCode uint8

const (
	opP opCode = iota
	opC
	opU
	opR
	opD
	opN // negation; rare
)

type opFeatureValue struct {
	op      opCode
	feature uint16
	value   uint16
}

const (
	flagValueNeutral uint16 = 0
	flagValueAny     uint16 = 1
)

// Transducer is an in-memory HFST optimised-lookup analyzer.
type Transducer struct {
	data []byte // entire file contents

	weighted bool

	// alphabet
	symbolToString    []string
	stringToSymbol    map[rune]uint16   // single-rune symbols only
	multiCharSymbols  map[string]uint16 // multi-rune surface symbols (rare in analyzers)
	symbolToDiacritic []opFeatureValue  // index → flag op (only meaningful for flag-diacritic symbols)
	flagFeatureCount  uint16
	unknownSymbol     uint16
	defaultSymbol     uint16
	identitySymbol    uint16

	// table layout
	indexTableOffset int    // byte offset in data[]
	transTableOffset int    // byte offset in data[]
	indexTableSize   uint32 // number of entries
	transTableSize   uint32 // number of entries
}

// Open reads an .hfstol file into memory and parses the header,
// alphabet, and table offsets. The Transducer holds a reference to
// the file contents; call Close when done.
func Open(path string) (*Transducer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(data)
}

// Close releases the in-memory file backing.
func (t *Transducer) Close() error {
	t.data = nil
	return nil
}

func parse(data []byte) (*Transducer, error) {
	// HFST3 file preamble:
	//   bytes 0-3: "HFST"
	//   byte 4:    0x00 (separator)
	//   bytes 5-6: remaining_header_len (uint16 LE)
	//   byte 7:    0x00 (second separator) - required by HFST's reader
	//   bytes 8..8+remaining_header_len: property-bag (null-terminated key/value pairs)
	if len(data) < 8 {
		return nil, fmt.Errorf("hfstol: file too short (%d bytes)", len(data))
	}
	if string(data[0:4]) != hfstMagic || data[4] != 0x00 {
		return nil, fmt.Errorf("hfstol: missing HFST magic header")
	}
	headerLen := binary.LittleEndian.Uint16(data[5:7])
	if data[7] != 0x00 {
		return nil, fmt.Errorf("hfstol: expected null separator at byte 7, got %#x", data[7])
	}
	bagStart := 8
	bagEnd := bagStart + int(headerLen)
	if bagEnd > len(data) {
		return nil, fmt.Errorf("hfstol: property bag truncated (need %d bytes from offset %d)", headerLen, bagStart)
	}
	off := bagEnd
	props := readPropertyBag(data[bagStart:bagEnd])
	weighted := props["type"] == "HFST_OLW"

	if off+transducerHeaderLen > len(data) {
		return nil, fmt.Errorf("hfstol: truncated before TransducerHeader")
	}
	t := &Transducer{data: data, weighted: weighted}

	// TransducerHeader: 2+2+4+4+4+4+ 9×4 bytes.
	thdr := data[off : off+transducerHeaderLen]
	numInputSymbols := binary.LittleEndian.Uint16(thdr[0:2])
	numSymbols := binary.LittleEndian.Uint16(thdr[2:4])
	t.indexTableSize = binary.LittleEndian.Uint32(thdr[4:8])
	t.transTableSize = binary.LittleEndian.Uint32(thdr[8:12])
	// thdr[12:20] = number_of_states + number_of_transitions (informational; ignore)
	headerWeighted := binary.LittleEndian.Uint32(thdr[20:24]) != 0
	if headerWeighted != weighted {
		// Trust the property bag's "type" field; the in-header bool can
		// disagree on some converted files. Don't error.
		t.weighted = headerWeighted
	}
	_ = numInputSymbols
	off += transducerHeaderLen

	// Alphabet: numSymbols null-terminated UTF-8 strings.
	alphaEnd, err := t.parseAlphabet(off, numSymbols)
	if err != nil {
		return nil, err
	}
	off = alphaEnd

	t.indexTableOffset = off
	indexBytes := int(t.indexTableSize) * indexEntrySize
	if off+indexBytes > len(data) {
		return nil, fmt.Errorf("hfstol: index table truncated (need %d bytes from offset %d, have %d)", indexBytes, off, len(data)-off)
	}
	off += indexBytes

	t.transTableOffset = off
	transRecSize := unweightedTransSize
	if t.weighted {
		transRecSize = weightedTransSize
	}
	transBytes := int(t.transTableSize) * transRecSize
	if off+transBytes > len(data) {
		return nil, fmt.Errorf("hfstol: transition table truncated (need %d bytes from offset %d, have %d)", transBytes, off, len(data)-off)
	}
	return t, nil
}

func readPropertyBag(buf []byte) map[string]string {
	out := map[string]string{}
	off := 0
	for off < len(buf) {
		end := bytes.IndexByte(buf[off:], 0)
		if end < 0 {
			break
		}
		key := string(buf[off : off+end])
		off += end + 1
		if off >= len(buf) {
			break
		}
		end = bytes.IndexByte(buf[off:], 0)
		if end < 0 {
			break
		}
		val := string(buf[off : off+end])
		off += end + 1
		out[key] = val
	}
	return out
}

func (t *Transducer) parseAlphabet(start int, numSymbols uint16) (int, error) {
	t.symbolToString = make([]string, numSymbols)
	t.symbolToDiacritic = make([]opFeatureValue, numSymbols)
	t.stringToSymbol = make(map[rune]uint16, numSymbols)
	t.multiCharSymbols = make(map[string]uint16)
	t.unknownSymbol = noSymbol
	t.defaultSymbol = noSymbol
	t.identitySymbol = noSymbol

	features := map[string]uint16{}
	values := map[string]uint16{"": flagValueNeutral, "@": flagValueAny}

	off := start
	for i := uint16(0); i < numSymbols; i++ {
		if off >= len(t.data) {
			return 0, fmt.Errorf("hfstol: alphabet truncated at symbol %d", i)
		}
		end := bytes.IndexByte(t.data[off:], 0)
		if end < 0 {
			return 0, fmt.Errorf("hfstol: unterminated symbol %d", i)
		}
		sym := string(t.data[off : off+end])
		off += end + 1
		t.symbolToString[i] = sym

		// Special meta-symbols recognised by HFST OL.
		switch sym {
		case "@_EPSILON_SYMBOL_@":
			// symbol 0; nothing to map
			continue
		case "@_UNKNOWN_SYMBOL_@":
			t.unknownSymbol = i
			continue
		case "@_DEFAULT_SYMBOL_@":
			t.defaultSymbol = i
			continue
		case "@_IDENTITY_SYMBOL_@":
			t.identitySymbol = i
			continue
		}

		// Flag diacritic? Format: "@OP.FEAT[.VAL]@", at least 5 chars.
		if len(sym) >= 5 && sym[0] == '@' && sym[len(sym)-1] == '@' && sym[2] == '.' {
			ofv, ok := parseDiacritic(sym, features, values)
			if ok {
				t.symbolToDiacritic[i] = ofv
				continue
			}
			// Not a recognised flag - fall through and treat as opaque tag.
		}

		runes := []rune(sym)
		switch len(runes) {
		case 0:
			// shouldn't happen; defensive
		case 1:
			if _, exists := t.stringToSymbol[runes[0]]; !exists {
				t.stringToSymbol[runes[0]] = i
			}
		default:
			// Multi-character symbols: morphology tags ("[Ln]", "+N", "Nom"),
			// derivation markers, etc. These never match input runes during
			// analysis (input is the surface form, character by character)
			// but they appear as outputs we emit.
			t.multiCharSymbols[sym] = i
		}
	}

	t.flagFeatureCount = uint16(len(features))
	return off, nil
}

func parseDiacritic(symbol string, features, values map[string]uint16) (opFeatureValue, bool) {
	var op opCode
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
	case 'N':
		op = opN
	default:
		return opFeatureValue{}, false
	}
	body := symbol[3 : len(symbol)-1]
	feature, value := body, "@"
	if dot := strings.IndexByte(body, '.'); dot >= 0 {
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
	return opFeatureValue{op: op, feature: fid, value: vid}, true
}

// Index entry decoders. Index entries are 6 bytes: uint16 input + uint32 first.
func (t *Transducer) indexEntry(idx uint32) (input uint16, first uint32) {
	off := t.indexTableOffset + int(idx)*indexEntrySize
	input = binary.LittleEndian.Uint16(t.data[off : off+2])
	first = binary.LittleEndian.Uint32(t.data[off+2 : off+6])
	return
}

// Transition decoders. Unweighted: 8 bytes (uint16 in + uint16 out + uint32 target).
// Weighted: 12 bytes (same + float32 weight, which we currently discard).
func (t *Transducer) transition(idx uint32) (input, output uint16, target uint32) {
	recSize := unweightedTransSize
	if t.weighted {
		recSize = weightedTransSize
	}
	off := t.transTableOffset + int(idx)*recSize
	input = binary.LittleEndian.Uint16(t.data[off : off+2])
	output = binary.LittleEndian.Uint16(t.data[off+2 : off+4])
	target = binary.LittleEndian.Uint32(t.data[off+4 : off+8])
	return
}

// transitionFinality matches HFST's Transition::final():
// input == NO_SYMBOL && output == NO_SYMBOL && target_index == 1.
// (transducer.cc, "bool Transition::final() const".)
func (t *Transducer) transitionFinality(idx uint32) bool {
	in, out, target := t.transition(idx)
	return in == noSymbol && out == noSymbol && target == 1
}

// indexFinality matches HFST's TransitionIndex::final():
// input == NO_SYMBOL && first_transition_index != NO_TABLE_INDEX.
// (transducer.cc, "bool TransitionIndex::final() const".) For the
// weighted variant, first_transition_index is overloaded as a float
// final-weight bit pattern; non-final entries still carry NO_TABLE_INDEX
// (0xFFFFFFFF), so the same predicate works for both flavours.
func (t *Transducer) indexFinality(idx uint32) bool {
	in, first := t.indexEntry(idx)
	return in == noSymbol && first != noTableIndex
}
