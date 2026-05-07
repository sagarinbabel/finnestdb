// Package vfst is a pure-Go reader for libvoikko's VFST binary
// finite-state transducer format. It implements the unweighted
// transducer subset, which is what the Finnish morphology analyzer
// (mor.vfst, shipped with libvoikko) uses.
//
// The algorithm is a direct port of libvoikko's
// src/fst/UnweightedTransducer.cpp and src/fst/Transducer.cpp
// (Harri Pitkänen, 2012-2015), tri-licensed under MPL 1.1 / GPLv2 /
// LGPLv2.1. See data/fi/LICENSE-libvoikko.txt alongside the vendored
// mor.vfst data file for the original copyright and license terms.
//
// File format (16-byte header + symbol table + transition table):
//
//   bytes 0-3:   cookie1 = 0x00013A6E (LE)
//   bytes 4-7:   cookie2 = 0x000351FA (LE)
//   byte 8:      0x00 = unweighted (this package), 0x01 = weighted (not supported)
//   bytes 9-15:  reserved
//   bytes 16-17: symbolCount (uint16 LE)
//   bytes 18-:   symbolCount null-terminated UTF-8 symbol strings
//                  (symbol 0 is just a single null byte = epsilon)
//   padding to 8-byte boundary
//   transition table: each transition is 8 bytes:
//     bytes 0-1: symIn  (uint16 LE; 0xFFFF = final-state marker)
//     bytes 2-3: symOut (uint16 LE)
//     bytes 4-7: transInfo (uint32 LE; low 24 bits = targetState,
//                           high 8 bits = moreTransitions count for the source state)
//   if a state has moreTransitions == 0xFF, an 8-byte OverflowCell
//   follows the state's first transition with the real count + 1 in
//   bytes 0-3 and 4 bytes of padding.
//
// Symbols are partitioned into three regions by index:
//   - 0:                                epsilon
//   - 1..firstNormalChar-1:             flag diacritics (e.g. "@P.NeedNoun.ON@")
//   - firstNormalChar..firstMultiChar-1: single-character symbols (input alphabet)
//   - firstMultiChar..symbolCount-1:    multi-character morphology tags (e.g. "[Ln]", "[Sn]")
//
// Final-state marker: a transition with symIn == 0xFFFF.
package vfst
