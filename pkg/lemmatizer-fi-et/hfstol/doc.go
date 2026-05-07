// Package hfstol is a pure-Go reader for HFST's optimised-lookup
// transducer binary format (.hfstol). The runtime semantics are a
// direct port of HFST's libhfst/src/implementations/optimized-lookup/
// transducer.cc (Helsinki Finite-State Toolkit, GPLv3+); see
// data/<lang>/LICENSE-hfst.txt alongside the vendored .hfstol files
// for the upstream license terms.
//
// The format has two sections beyond the header: an "index table" and
// a "transition target table". Both are arrays of fixed-size records.
// Index entries are 6 bytes (uint16 input symbol + uint32 first
// transition index). Transition entries are 8 bytes for unweighted
// (uint16 input + uint16 output + uint32 target index) or 12 bytes
// for weighted (same plus a float32 weight).
//
// Lookup is hash-based via the index table: given a state s and an
// input symbol c, the matching transitions live at
// transitions[index_table[s + c].first_transition_index]. This is
// faster than VFST's linear scan but the file is bigger because the
// index table is sparse.
//
// File header layout:
//
//	bytes 0-3: "HFST" magic
//	byte 4:    0x00 separator
//	bytes 5-6: header_remaining_length (uint16 LE)
//	next N bytes: pairs of null-terminated key/value strings
//	  - "version" / "3.3"
//	  - "type" / "HFST_OL" (unweighted) or "HFST_OLW" (weighted)
//	  - "name" / "..." (free-form)
//	then a 56-byte TransducerHeader:
//	  uint16 number_of_input_symbols
//	  uint16 number_of_symbols
//	  uint32 size_of_transition_index_table
//	  uint32 size_of_transition_target_table
//	  uint32 number_of_states
//	  uint32 number_of_transitions
//	  9× uint32 boolean flags (weighted, deterministic, etc.)
//	then the alphabet: `number_of_symbols` null-terminated UTF-8
//	  symbol strings; symbol 0 is "@_EPSILON_SYMBOL_@"; flag diacritics
//	  match the libvoikko convention "@<P|C|U|R|D>.FEAT[.VAL]@".
//	then the transition index table.
//	then the transition target table.
//
// Index encoding (TRANSITION_TARGET_TABLE_START = 0x80000000):
//
//	If (i & 0x80000000) != 0, the index points into the transition
//	target table at offset (i - 0x80000000). Otherwise it points into
//	the index table at offset i. Both tables share the same logical
//	address space; the high bit selects which.
package hfstol
