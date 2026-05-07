package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"finnestdb/pkg/lemmatizer-fi-et/vfst"
	"finnestdb/pkg/lemmatizer-fi-et/voikkomap"
)

func main() {
	var (
		lang     = flag.String("lang", "fi", "language (fi only in this command)")
		vfstPath = flag.String("vfst", "", "path to mor.vfst (local-only; do not commit)")
		wordlist = flag.String("wordlist", "", "path to newline-delimited word list")
		outPath  = flag.String("out", "", "output JSON path (default: stdout)")
	)
	flag.Parse()

	if strings.ToLower(*lang) != "fi" {
		fatalf("unsupported -lang %q (this generator supports fi only)", *lang)
	}
	if *vfstPath == "" {
		fatalf("-vfst is required (local path to mor.vfst)")
	}
	if *wordlist == "" {
		fatalf("-wordlist is required")
	}

	tr, err := vfst.Open(*vfstPath)
	if err != nil {
		fatalf("open vfst: %v", err)
	}
	defer tr.Close()

	words, err := readWordlist(*wordlist)
	if err != nil {
		fatalf("read wordlist: %v", err)
	}

	table := map[string][]voikkomap.Analysis{}
	for _, w := range words {
		raw := tr.Analyze(w)
		if len(raw) == 0 {
			continue
		}
		out := make([]voikkomap.Analysis, 0, len(raw))
		for _, line := range raw {
			a := voikkomap.Parse(line)
			if a.Lemma == "" || a.UPOS == "" {
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
