package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"finnestdb/pkg/lemmatizer-fi-et/giellaltmap"
	"finnestdb/pkg/lemmatizer-fi-et/hfstol"
	"finnestdb/pkg/lemmatizer-fi-et/vfst"
	"finnestdb/pkg/lemmatizer-fi-et/voikkomap"
)

// analyzer is the minimal surface needed from either FI (vfst) or ET
// (hfstol) backends. Both expose Analyze(word) returning every raw
// reading as a string.
type analyzer interface {
	Analyze(word string) []string
	Close() error
}

// parseLine maps one raw analyser output to the runtime's shared
// Analysis shape. Each language has its own tag normaliser.
type parseLine func(raw string) (voikkomap.Analysis, bool)

func main() {
	var (
		lang       = flag.String("lang", "fi", "language: fi or et")
		vfstPath   = flag.String("vfst", "", "path to mor.vfst (FI only; local-only, do not commit)")
		hfstolPath = flag.String("hfstol", "", "path to .hfstol analyser (ET only; local-only, do not commit)")
		wordlist   = flag.String("wordlist", "", "path to newline-delimited word list")
		outPath    = flag.String("out", "", "output JSON path (default: stdout)")
	)
	flag.Parse()

	if *wordlist == "" {
		fatalf("-wordlist is required")
	}

	ana, parse, err := openBackend(*lang, *vfstPath, *hfstolPath)
	if err != nil {
		fatalf("%v", err)
	}
	defer ana.Close()

	words, err := readWordlist(*wordlist)
	if err != nil {
		fatalf("read wordlist: %v", err)
	}

	table := map[string][]voikkomap.Analysis{}
	for _, w := range words {
		raw := ana.Analyze(w)
		if len(raw) == 0 {
			continue
		}
		out := make([]voikkomap.Analysis, 0, len(raw))
		for _, line := range raw {
			a, ok := parse(line)
			if !ok {
				continue
			}
			out = append(out, a)
		}
		if len(out) == 0 {
			continue
		}
		// Stable output.
		sort.Slice(out, func(i, j int) bool {
			if out[i].Lemma != out[j].Lemma {
				return out[i].Lemma < out[j].Lemma
			}
			if out[i].UPOS != out[j].UPOS {
				return out[i].UPOS < out[j].UPOS
			}
			if out[i].GrammarLabel != out[j].GrammarLabel {
				return out[i].GrammarLabel < out[j].GrammarLabel
			}
			return out[i].Raw < out[j].Raw
		})
		table[w] = out
	}

	var keys []string
	for k := range table {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string][]voikkomap.Analysis, len(keys))
	for _, k := range keys {
		ordered[k] = table[k]
	}

	b, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		fatalf("marshal: %v", err)
	}
	b = append(b, '\n')

	if *outPath == "" {
		_, _ = os.Stdout.Write(b)
		return
	}
	if err := os.WriteFile(*outPath, b, 0o644); err != nil {
		fatalf("write %s: %v", *outPath, err)
	}
}

// openBackend resolves the language-specific analyser and parser. ET
// uses Giellalt's HFST optimised-lookup analyser and giellaltmap; FI
// uses Voikko's mor.vfst and voikkomap. The two backends are mutually
// exclusive — supplying the wrong flag for the chosen language is a
// hard error so a misconfigured run fails fast instead of silently
// emitting an empty table.
func openBackend(lang, vfstPath, hfstolPath string) (analyzer, parseLine, error) {
	switch strings.ToLower(lang) {
	case "fi":
		if vfstPath == "" {
			return nil, nil, fmt.Errorf("-vfst is required for -lang fi (local path to mor.vfst)")
		}
		if hfstolPath != "" {
			return nil, nil, fmt.Errorf("-hfstol must not be set for -lang fi")
		}
		tr, err := vfst.Open(vfstPath)
		if err != nil {
			return nil, nil, fmt.Errorf("open vfst: %w", err)
		}
		parse := func(raw string) (voikkomap.Analysis, bool) {
			a := voikkomap.Parse(raw)
			if a.Lemma == "" || a.UPOS == "" {
				return voikkomap.Analysis{}, false
			}
			return a, true
		}
		return tr, parse, nil
	case "et":
		if hfstolPath == "" {
			return nil, nil, fmt.Errorf("-hfstol is required for -lang et (local path to analyser-gt-desc.hfstol)")
		}
		if vfstPath != "" {
			return nil, nil, fmt.Errorf("-vfst must not be set for -lang et")
		}
		tr, err := hfstol.Open(hfstolPath)
		if err != nil {
			return nil, nil, fmt.Errorf("open hfstol: %w", err)
		}
		parse := func(raw string) (voikkomap.Analysis, bool) {
			g := giellaltmap.Parse(raw)
			if g.Lemma == "" || g.UPOS == "" {
				return voikkomap.Analysis{}, false
			}
			// giellaltmap.Analysis carries an extra IsCompound bit that
			// voikkomap.Analysis doesn't. Project it down to the runtime's
			// shared shape so the JSON written here matches what
			// pkg/lemmatizer-fi-et reads on load.
			return voikkomap.Analysis{
				Lemma:        g.Lemma,
				UPOS:         g.UPOS,
				GrammarLabel: g.GrammarLabel,
				Number:       g.Number,
				Tense:        g.Tense,
				Mood:         g.Mood,
				Person:       g.Person,
				Feats:        g.Feats,
				Raw:          g.Raw,
			}, true
		}
		return tr, parse, nil
	default:
		return nil, nil, fmt.Errorf("unsupported -lang %q (want fi or et)", lang)
	}
}

func readWordlist(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	seen := map[string]bool{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		w := strings.TrimSpace(s.Text())
		if w == "" || strings.HasPrefix(w, "#") {
			continue
		}
		w = strings.ToLower(w)
		if seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out, s.Err()
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
