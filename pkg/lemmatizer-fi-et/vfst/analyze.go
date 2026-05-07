package vfst

import (
	"log"
	"strings"
)

// configuration holds the state of one Analyze traversal. A fresh
// configuration is allocated per Analyze call, so concurrent Analyze
// calls on a single Transducer are safe (read-only aliasing of t.data).
type configuration struct {
	inputSymbols []uint16
	inputLength  int
	inputDepth   int

	stateIndexStack        []uint32
	currentTransitionStack []uint32
	outputSymbolStack      []uint16
	stackDepth             int

	flagDepth          int
	currentFlagValues  []uint16
	updatedFlagValue   []uint16
	updatedFlagFeature []uint16
}

const initialStackDepth = 256

func newConfiguration(t *Transducer, inputLen int) *configuration {
	return &configuration{
		inputSymbols:           make([]uint16, 0, inputLen),
		stateIndexStack:        make([]uint32, initialStackDepth),
		currentTransitionStack: make([]uint32, initialStackDepth),
		outputSymbolStack:      make([]uint16, initialStackDepth),
		currentFlagValues:      make([]uint16, t.flagDiacriticFeatureCount),
		updatedFlagValue:       make([]uint16, initialStackDepth),
		updatedFlagFeature:     make([]uint16, initialStackDepth),
	}
}

func (cfg *configuration) growStacksIfNeeded(needed int) {
	if needed < len(cfg.stateIndexStack) {
		return
	}
	newSize := len(cfg.stateIndexStack) * 2
	for newSize <= needed {
		newSize *= 2
	}
	grow := func(s []uint32, n int) []uint32 { return append(s, make([]uint32, n-len(s))...) }
	grow16 := func(s []uint16, n int) []uint16 { return append(s, make([]uint16, n-len(s))...) }
	cfg.stateIndexStack = grow(cfg.stateIndexStack, newSize)
	cfg.currentTransitionStack = grow(cfg.currentTransitionStack, newSize)
	cfg.outputSymbolStack = grow16(cfg.outputSymbolStack, newSize)
	cfg.updatedFlagValue = grow16(cfg.updatedFlagValue, newSize)
	cfg.updatedFlagFeature = grow16(cfg.updatedFlagFeature, newSize)
}

// Analyze returns every morphological analysis of input that the FST
// recognises. Each analysis is the concatenation of the output symbols
// along one accepting path — for the libvoikko Finnish FST that's a
// string like "[Ln][Ica][Xp]talo[X]talo[Sn][Ny]". Returns nil if the
// input is not in the alphabet or has no accepted analysis.
func (t *Transducer) Analyze(input string) []string {
	runes := []rune(input)
	cfg := newConfiguration(t, len(runes))
	if !t.prepare(cfg, runes) {
		// The C++ runtime still walks the FST in this case; the result is
		// always empty because no transition can match the unknown-symbol
		// sentinel. We short-circuit for clarity and a small speedup.
		return nil
	}
	var out []string
	for {
		s, ok, truncated := t.next(cfg)
		if truncated {
			log.Printf("vfst: analysis traversal hit maxLoopCount=%d for %q; results may be incomplete", maxLoopCount, input)
			break
		}
		if !ok {
			break
		}
		out = append(out, s)
	}
	return out
}

func (t *Transducer) prepare(cfg *configuration, input []rune) bool {
	cfg.stackDepth = 0
	cfg.flagDepth = 0
	cfg.inputDepth = 0
	cfg.inputLength = 0
	cfg.stateIndexStack[0] = 0
	cfg.currentTransitionStack[0] = 0
	for i := range cfg.currentFlagValues {
		cfg.currentFlagValues[i] = flagValueNeutral
	}
	allKnown := true
	for _, r := range input {
		sym, ok := t.stringToSymbol[r]
		if !ok {
			cfg.inputSymbols = append(cfg.inputSymbols, t.unknownSymbol)
			allKnown = false
		} else {
			cfg.inputSymbols = append(cfg.inputSymbols, sym)
		}
		cfg.inputLength++
	}
	return allKnown
}

func (t *Transducer) next(cfg *configuration) (string, bool, bool) {
	for loop := 0; loop < maxLoopCount; loop++ {
		stateIdx := cfg.stateIndexStack[cfg.stackDepth]
		currentIdx := cfg.currentTransitionStack[cfg.stackDepth]
		startIdx := currentIdx - stateIdx
		maxTc := t.getMaxTc(stateIdx)

		descended := false
		for tc := startIdx; tc <= maxTc; tc++ {
			if tc == 1 && maxTc >= 255 {
				// the slot at relative index 1 is an OverflowCell, not a transition
				tc++
				currentIdx++
			}
			cur := t.decodeTransitionAtIndex(currentIdx)

			if cur.symIn == finalSymbol {
				if cfg.inputDepth == cfg.inputLength {
					out := t.emitOutput(cfg)
					cfg.currentTransitionStack[cfg.stackDepth] = currentIdx + 1
					return out, true, false
				}
				// not all input consumed; keep searching
				currentIdx++
				continue
			}

			inputMatch := cfg.inputDepth < cfg.inputLength &&
				cfg.inputSymbols[cfg.inputDepth] == cur.symIn
			flagOk := cur.symIn < t.firstNormalChar &&
				t.flagDiacriticCheck(cfg, cur.symIn)

			if inputMatch || flagOk {
				cfg.growStacksIfNeeded(cfg.stackDepth + 2)
				outSym := uint16(0)
				if cur.symOut >= t.firstNormalChar {
					outSym = cur.symOut
				}
				cfg.outputSymbolStack[cfg.stackDepth] = outSym
				cfg.currentTransitionStack[cfg.stackDepth] = currentIdx
				cfg.stackDepth++
				cfg.stateIndexStack[cfg.stackDepth] = cur.targetState
				cfg.currentTransitionStack[cfg.stackDepth] = cur.targetState
				if cur.symIn >= t.firstNormalChar {
					cfg.inputDepth++
				}
				descended = true
				break
			}
			currentIdx++
		}
		if descended {
			continue
		}

		// no further transitions in this state — backtrack
		if cfg.stackDepth == 0 {
			return "", false, false
		}
		cfg.stackDepth--
		prev := t.decodeTransitionAtIndex(cfg.currentTransitionStack[cfg.stackDepth])
		if prev.symIn >= t.firstNormalChar {
			cfg.inputDepth--
		} else if t.flagDiacriticFeatureCount > 0 && prev.symIn != 0 {
			cfg.flagDepth--
			feat := cfg.updatedFlagFeature[cfg.flagDepth]
			cfg.currentFlagValues[feat] = cfg.updatedFlagValue[cfg.flagDepth]
		}
		cfg.currentTransitionStack[cfg.stackDepth]++
	}
	return "", false, true
}

func (t *Transducer) emitOutput(cfg *configuration) string {
	var b strings.Builder
	for i := 0; i < cfg.stackDepth; i++ {
		sym := cfg.outputSymbolStack[i]
		if sym == 0 {
			continue
		}
		b.WriteString(t.symbolToString[sym])
	}
	return b.String()
}

// flagDiacriticCheck mirrors libvoikko's flagDiacriticCheck. Returns
// true if the flag operation is permitted given the current flag state
// (and updates that state on the configuration), false if the path
// must be abandoned.
func (t *Transducer) flagDiacriticCheck(cfg *configuration, sym uint16) bool {
	if t.flagDiacriticFeatureCount == 0 || sym == 0 {
		return true
	}
	ofv := t.symbolToDiacritic[sym]
	cur := cfg.currentFlagValues[ofv.feature]
	update := false
	val := ofv.value

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
	default:
		return false
	}

	cfg.growStacksIfNeeded(cfg.flagDepth + 1)
	cfg.updatedFlagFeature[cfg.flagDepth] = ofv.feature
	cfg.updatedFlagValue[cfg.flagDepth] = cur
	if update {
		cfg.currentFlagValues[ofv.feature] = val
	}
	cfg.flagDepth++
	return true
}
