package main

import (
	"bufio"
	"os"
	"strings"
)

// frequencyRanks loads an OpenSubtitles "form count" list (highest count
// first) and returns a map from lowercased surface form to its 1-based rank.
// Rank 1 is the most common form. Used to compute the mean-frequency-rank and
// rare-form-rate difficulty signals. A missing file yields a nil map and the
// generator falls back to the no-baseline difficulty path.
//
// The list lives under localdata/frequency/<lang>/opensubtitles-2018-<lang>-50k.txt
// which is read-only corpus material. The ranks are baked into the checked-in
// metrics at generation time; production never reads this file.
func frequencyRanks(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ranks := make(map[string]int)
	scanner := bufio.NewScanner(f)
	rank := 0
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		form := strings.ToLower(fields[0])
		if form == "" {
			continue
		}
		rank++
		// First occurrence wins; the list is already frequency-ordered.
		if _, seen := ranks[form]; !seen {
			ranks[form] = rank
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ranks, nil
}
