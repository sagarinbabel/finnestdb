// Package starterdeck holds the shared Top-N lemma ranking used to seed the
// cold-start "Top N words" official deck (cmd/seedcolddeck) and to select the
// corpus example sentences that back those deck cards (cmd/pickexamples).
//
// Both tools must rank identical lemmas from the same OpenSubtitles frequency
// baseline so that every deck card has a matching example. Keeping the ranking
// in one place is what guarantees that: change the ranking here and both the
// deck and its examples move together.
package starterdeck

import (
	"bufio"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"finnestdb/internal/store"
)

const (
	// MaxListLines bounds how deep into the frequency list we read before
	// resolution. Resolution loses some forms (names, typos, foreign words),
	// so we read well past the requested top-N to have enough resolved lemmas.
	MaxListLines = 20000
	lookupChunk  = 500
)

// LemmaEntry is one aggregated deck entry: a lemma ranked by the summed token
// count of all its surface forms, with its most frequent form kept as the
// representative example.
type LemmaEntry struct {
	Lemma    string
	POS      string
	ReprForm string
	Count    int64
}

// FormResolver is the dictionary lookup seam, satisfied by *store.DB.
type FormResolver interface {
	BatchLookupForms(forms []string, lang string, parserMode string) map[string]store.FormResolution
}

// TopLemmas reads a "form count" frequency list (highest count first),
// resolves forms through the dictionary in chunks, aggregates counts per
// (lemma, pos), and returns the top N entries by summed count. Returns the
// number of list forms that did not resolve.
//
// This is the single ranking definition shared by seedcolddeck and
// pickexamples; both must call it with the same list, resolver, and N.
func TopLemmas(r io.Reader, resolver FormResolver, lang string, n int) ([]LemmaEntry, int, error) {
	type formCount struct {
		Form  string
		Count int64
	}
	var list []formCount
	scanner := bufio.NewScanner(r)
	for scanner.Scan() && len(list) < MaxListLines {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		count, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || count <= 0 {
			continue
		}
		if !hasLetter(fields[0]) {
			continue
		}
		list = append(list, formCount{Form: fields[0], Count: count})
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	type lemmaKey struct{ Lemma, POS string }
	totals := make(map[lemmaKey]*LemmaEntry)
	skipped := 0
	for start := 0; start < len(list); start += lookupChunk {
		end := start + lookupChunk
		if end > len(list) {
			end = len(list)
		}
		chunk := list[start:end]
		forms := make([]string, len(chunk))
		for i, fc := range chunk {
			forms[i] = fc.Form
		}
		resolved := resolver.BatchLookupForms(forms, lang, "custom")
		// Iterate the chunk in list order (highest count first) so the first
		// form credited to a lemma is its most frequent one.
		for _, fc := range chunk {
			res, ok := resolved[fc.Form]
			if !ok || res.Lemma == "" || res.POS == "" || res.POS == "PUNCT" {
				skipped++
				continue
			}
			// Subtitle corpora are dense with character names; a starter
			// vocabulary deck should teach words, not the cast of 50k films.
			if res.POS == "PROPN" {
				skipped++
				continue
			}
			key := lemmaKey{Lemma: res.Lemma, POS: res.POS}
			if entry, exists := totals[key]; exists {
				entry.Count += fc.Count
			} else {
				totals[key] = &LemmaEntry{Lemma: res.Lemma, POS: res.POS, ReprForm: fc.Form, Count: fc.Count}
			}
		}
	}

	entries := make([]LemmaEntry, 0, len(totals))
	for _, e := range totals {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Lemma < entries[j].Lemma
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries, skipped, nil
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
