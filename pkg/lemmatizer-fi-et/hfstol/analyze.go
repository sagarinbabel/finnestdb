package hfstol

import "strings"

const (
	maxRecursionDepth = 5000
	maxOutputDepth    = 10000
)

// Analyze returns every morphological analysis of input. Each
// analysis is the concatenation of output symbols along an accepting
// path through the FST. Returns nil if no analysis exists.
func (t *Transducer) Analyze(input string) []string {
	st := newAnalysisState(t, input)
	if !st.symbolsKnown {
		return nil
	}
	st.getAnalyses(0, 0, 0)
	return st.results
}

// analysisState holds per-call traversal state. A new state per
// Analyze call keeps Transducer purely read-only and Analyze
// concurrent-safe.
type analysisState struct {
	t *Transducer

	inputSymbols []uint16 // input string converted to symbol IDs
	outputSymbols []uint16 // output tape: symbols emitted along the current path
	results []string

	// Flag diacritic state. flags[feature] is the current value;
	// applyOperation pushes the previous value onto savedValues so we
	// can restore on return.
	flags         []uint16
	savedFeatures []uint16
	savedValues   []uint16
	flagDepth     int

	recursionLeft int
	symbolsKnown  bool

	// Cycle protection for epsilon transitions: a (state, flag-snapshot)
	// pair we've already entered along the current input position. We
	// use a simple visited-state map keyed by (target, flag-fingerprint).
	visitedEpsilon map[uint64]struct{}
}

func newAnalysisState(t *Transducer, input string) *analysisState {
	st := &analysisState{
		t:              t,
		results:        nil,
		flags:          make([]uint16, t.flagFeatureCount),
		savedFeatures:  make([]uint16, 0, 32),
		savedValues:    make([]uint16, 0, 32),
		recursionLeft:  maxRecursionDepth,
		symbolsKnown:   true,
		visitedEpsilon: map[uint64]struct{}{},
	}
	for _, r := range input {
		sym, ok := t.stringToSymbol[r]
		if !ok {
			// Use unknown / identity if available; otherwise mark unknown
			// and abort cheaply.
			if t.unknownSymbol != noSymbol {
				sym = t.unknownSymbol
			} else if t.identitySymbol != noSymbol {
				sym = t.identitySymbol
			} else {
				st.symbolsKnown = false
				return st
			}
		}
		st.inputSymbols = append(st.inputSymbols, sym)
	}
	st.outputSymbols = make([]uint16, 0, len(st.inputSymbols)*4)
	return st
}

// emit writes the current outputSymbols path to results, dropping
// epsilon and meta symbols.
func (st *analysisState) emit() {
	var b strings.Builder
	for _, s := range st.outputSymbols {
		if s == 0 || s == noSymbol {
			continue
		}
		// Drop flag diacritics from the output (they are control symbols).
		sym := st.t.symbolToString[s]
		if len(sym) >= 5 && sym[0] == '@' && sym[len(sym)-1] == '@' && (sym[2] == '.') {
			continue
		}
		b.WriteString(sym)
	}
	st.results = append(st.results, b.String())
}

// getAnalyses is the recursive DFS. It mirrors HFST's
// Transducer::get_analyses (transducer.cc:580).
func (st *analysisState) getAnalyses(inputPos, outputPos int, i uint32) {
	if st.recursionLeft <= 0 {
		return
	}
	st.recursionLeft--
	defer func() { st.recursionLeft++ }()

	if i >= transTableStart {
		// i indexes into the transition table.
		i -= transTableStart

		// Finality check at end of input.
		if inputPos >= len(st.inputSymbols) {
			if st.t.transitionFinality(i) {
				st.emit()
			}
		}

		// Try epsilon transitions.
		st.tryEpsilonTransitions(inputPos, outputPos, i+1)

		// Try the input symbol if there is one.
		if inputPos < len(st.inputSymbols) {
			st.findTransitions(st.inputSymbols[inputPos], inputPos+1, outputPos+1, i+1)
		}
		return
	}

	// i indexes into the index table.
	if inputPos >= len(st.inputSymbols) {
		if st.t.indexFinality(i) {
			st.emit()
		}
	}

	// Try epsilon indices.
	st.tryEpsilonIndices(inputPos, outputPos, i+1)

	// Try the input symbol via the index.
	if inputPos < len(st.inputSymbols) {
		st.findIndex(st.inputSymbols[inputPos], inputPos+1, outputPos+1, i+1)
	}
}

func (st *analysisState) tryEpsilonTransitions(inputPos, outputPos int, i uint32) {
	for i < st.t.transTableSize {
		in, out, target := st.t.transition(i)
		if in == 0 {
			// epsilon transition. Skip if we've already entered `target`
			// at this input position with the current flag state — this
			// is the cycle protection HFST uses (traversal_states).
			key := st.epsilonVisitKey(target)
			if _, seen := st.visitedEpsilon[key]; seen {
				i++
				continue
			}
			st.visitedEpsilon[key] = struct{}{}
			st.pushOutput(outputPos, out)
			st.getAnalyses(inputPos, outputPos+1, target)
			st.popOutput(outputPos)
			delete(st.visitedEpsilon, key)
			i++
			continue
		}
		if st.isFlagDiacritic(in) {
			savedDepth := st.flagDepth
			if st.applyOperation(in) {
				key := st.epsilonVisitKey(target)
				if _, seen := st.visitedEpsilon[key]; !seen {
					st.visitedEpsilon[key] = struct{}{}
					st.pushOutput(outputPos, out)
					st.getAnalyses(inputPos, outputPos+1, target)
					st.popOutput(outputPos)
					delete(st.visitedEpsilon, key)
				}
			}
			st.unapplyTo(savedDepth)
			i++
			continue
		}
		return
	}
}

// epsilonVisitKey fingerprints (target_state, flag_state) so we can
// short-circuit revisits during epsilon-only walks. HFST uses an
// std::set<TraversalState>; we use a map to a uint64 hash that mixes
// target with the flag-stack contents.
func (st *analysisState) epsilonVisitKey(target uint32) uint64 {
	h := uint64(target)
	for i := 0; i < st.flagDepth; i++ {
		h = h*1099511628211 ^ uint64(st.savedFeatures[i])
		h = h*1099511628211 ^ uint64(st.flags[st.savedFeatures[i]])
	}
	return h
}

func (st *analysisState) tryEpsilonIndices(inputPos, outputPos int, i uint32) {
	if i >= st.t.indexTableSize {
		return
	}
	in, target := st.t.indexEntry(i)
	if in != 0 {
		return
	}
	// target is in transition-target-table address space.
	st.tryEpsilonTransitions(inputPos, outputPos, target-transTableStart)
}

func (st *analysisState) findTransitions(input uint16, inputPos, outputPos int, i uint32) {
	// Consuming a real input symbol terminates any in-flight epsilon loop,
	// so we can drop the visited set: there's no way to re-enter the same
	// (state, flag) pair without consuming further input.
	st.visitedEpsilon = map[uint64]struct{}{}
	for i < st.t.transTableSize {
		in, out, target := st.t.transition(i)
		if in == noSymbol {
			return
		}
		if in == input {
			realOut := out
			if realOut == st.t.identitySymbol || realOut == st.t.unknownSymbol {
				// "match anything" output: emit the input we just consumed.
				realOut = st.inputSymbols[inputPos-1]
			}
			st.pushOutput(outputPos, realOut)
			st.getAnalyses(inputPos, outputPos+1, target)
			st.popOutput(outputPos)
			i++
			continue
		}
		return
	}
}

func (st *analysisState) findIndex(input uint16, inputPos, outputPos int, i uint32) {
	at := i + uint32(input)
	if at >= st.t.indexTableSize {
		return
	}
	in, target := st.t.indexEntry(at)
	if in != input {
		return
	}
	st.findTransitions(input, inputPos, outputPos, target-transTableStart)
}

// pushOutput appends to outputSymbols, growing if needed.
func (st *analysisState) pushOutput(pos int, sym uint16) {
	for len(st.outputSymbols) <= pos {
		st.outputSymbols = append(st.outputSymbols, 0)
	}
	st.outputSymbols[pos] = sym
}

func (st *analysisState) popOutput(pos int) {
	if pos < len(st.outputSymbols) {
		st.outputSymbols[pos] = 0
	}
}

func (st *analysisState) isFlagDiacritic(sym uint16) bool {
	if int(sym) >= len(st.t.symbolToDiacritic) {
		return false
	}
	s := st.t.symbolToString[sym]
	return len(s) >= 5 && s[0] == '@' && s[len(s)-1] == '@' && s[2] == '.'
}

// applyOperation runs the flag diacritic state machine for `sym`.
// Returns true if the path is permitted; false if the flag forbids
// continuing. Modifies st.flags and pushes a save onto savedFeatures /
// savedValues.
func (st *analysisState) applyOperation(sym uint16) bool {
	ofv := st.t.symbolToDiacritic[sym]
	cur := st.flags[ofv.feature]
	val := ofv.value
	update := false

	switch ofv.op {
	case opP:
		update = true
	case opC:
		val = flagValueNeutral
		update = true
	case opU:
		if cur != flagValueNeutral {
			if cur != ofv.value {
				return false
			}
		} else {
			update = true
		}
	case opR:
		if ofv.value == flagValueAny && cur == flagValueNeutral {
			return false
		}
		if ofv.value != flagValueAny && cur != ofv.value {
			return false
		}
	case opD:
		if (ofv.value == flagValueAny && cur != flagValueNeutral) || cur == ofv.value {
			return false
		}
	case opN:
		// Negation: rare; treat as a no-op for now.
	}

	st.savedFeatures = append(st.savedFeatures, ofv.feature)
	st.savedValues = append(st.savedValues, cur)
	st.flagDepth++
	if update {
		st.flags[ofv.feature] = val
	}
	return true
}

// unapplyTo restores st.flags to whatever it was when flagDepth ==
// targetDepth. Used during backtracking.
func (st *analysisState) unapplyTo(targetDepth int) {
	for st.flagDepth > targetDepth {
		st.flagDepth--
		feat := st.savedFeatures[st.flagDepth]
		val := st.savedValues[st.flagDepth]
		st.flags[feat] = val
		st.savedFeatures = st.savedFeatures[:st.flagDepth]
		st.savedValues = st.savedValues[:st.flagDepth]
	}
}
