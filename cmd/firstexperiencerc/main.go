// cmd/firstexperiencerc runs the automated portion of the first-experience
// release-candidate pack described in docs/GO_LIVE_CHECKLIST.md
// "First-experience quality check". It loads the single canonical manifest
// at testdata/first-experience-rc/manifest.json and executes every case
// whose "automation" field is "parser": it parses the case's fixture text
// through the real custom-mode parser pipeline (internal/parsecore, the same
// path /api/parse uses) and asserts the case's "expect" block.
//
// Cases with automation "playwright" are exercised separately by
// web/tests/first-experience-rc.spec.ts (see `make first-experience-rc`,
// which runs this binary and then that spec back to back). Cases with
// automation "manual" point at the manual walkthrough instructions in
// docs/GO_LIVE_CHECKLIST.md. Cases with automation "pending" mark a journey
// that does not have automated or manual coverage wired up yet — this is
// expected for a skeleton pack and is not a run failure.
//
// Exit code is nonzero only when an automated ("parser") case fails its
// expectations or errors while parsing. Pending and manual cases never fail
// the run; they are reported so the summary states what's still missing.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

const defaultManifestPath = "testdata/first-experience-rc/manifest.json"

// lemmaPOS is one expected (lemma, POS) attachment the parser must produce
// somewhere in the fixture's word list.
type lemmaPOS struct {
	Lemma string `json:"lemma"`
	POS   string `json:"pos"`
}

// caseExpect is intentionally permissive: most fields are optional so
// pending/manual/playwright cases can carry human-readable notes only,
// while "parser" cases add the fields the runner actually asserts.
type caseExpect struct {
	MinTokenCount int        `json:"min_token_count,omitempty"`
	LemmaPOS      []lemmaPOS `json:"lemma_pos,omitempty"`
	Notes         string     `json:"notes,omitempty"`
}

type rcCase struct {
	ID         string     `json:"id"`
	Language   string     `json:"language"`
	Journey    string     `json:"journey"`
	Automation string     `json:"automation"`
	Fixture    string     `json:"fixture,omitempty"`
	Expect     caseExpect `json:"expect,omitempty"`
	Notes      string     `json:"notes,omitempty"`
}

type manifest struct {
	Cases []rcCase `json:"cases"`
}

func main() {
	manifestPath := flag.String("manifest", defaultManifestPath, "Path to the first-experience RC manifest JSON")
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	flag.Parse()

	m, err := loadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest: %v\n", err)
		os.Exit(1)
	}

	db, err := store.NewDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db %q: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	fixtureDir := filepath.Dir(*manifestPath)

	var failed, passed, skippedPending, manualCount, otherAutomation int
	for _, c := range m.Cases {
		switch c.Automation {
		case "parser":
			ok, reason := runParserCase(db, fixtureDir, c)
			if ok {
				passed++
				fmt.Printf("PASS %s (%s/%s)\n", c.ID, c.Language, c.Journey)
			} else {
				failed++
				fmt.Printf("FAIL %s (%s/%s): %s\n", c.ID, c.Language, c.Journey, reason)
			}
		case "pending":
			skippedPending++
			reason := c.Expect.Notes
			if reason == "" {
				reason = "not yet automated"
			}
			fmt.Printf("SKIP-pending %s (%s/%s): %s\n", c.ID, c.Language, c.Journey, reason)
		case "manual":
			manualCount++
			fmt.Printf("MANUAL %s (%s/%s): see docs/GO_LIVE_CHECKLIST.md 'First-experience quality check'\n", c.ID, c.Language, c.Journey)
		case "playwright":
			// Executed by web/tests/first-experience-rc.spec.ts, not this
			// binary. Reported for visibility so the summary line accounts
			// for every case in the manifest.
			otherAutomation++
			fmt.Printf("SKIP-playwright %s (%s/%s): see web/tests/first-experience-rc.spec.ts\n", c.ID, c.Language, c.Journey)
		default:
			failed++
			fmt.Printf("FAIL %s (%s/%s): unknown automation %q\n", c.ID, c.Language, c.Journey, c.Automation)
		}
	}

	fmt.Println()
	fmt.Printf("Summary: %d passed, %d failed, %d pending, %d manual, %d playwright (of %d total)\n",
		passed, failed, skippedPending, manualCount, otherAutomation, len(m.Cases))

	if failed > 0 {
		os.Exit(1)
	}
}

func loadManifest(path string) (*manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(m.Cases) == 0 {
		return nil, fmt.Errorf("%s: no cases found", path)
	}
	return &m, nil
}

// runParserCase parses the case's fixture through the real custom-mode
// parser pipeline and checks the expect block. Returns (true, "") on pass,
// or (false, reason) describing the first unmet expectation.
func runParserCase(db *store.DB, fixtureDir string, c rcCase) (bool, string) {
	if c.Fixture == "" {
		return false, "case has automation:parser but no fixture path"
	}
	fixturePath := filepath.Join(fixtureDir, c.Fixture)
	text, err := os.ReadFile(fixturePath)
	if err != nil {
		return false, fmt.Sprintf("read fixture %s: %v", fixturePath, err)
	}

	lang, err := udLang(c.Language)
	if err != nil {
		return false, err.Error()
	}

	result, err := parsecore.Analyze(db, lang, string(text), "custom")
	if err != nil {
		return false, fmt.Sprintf("parse %s: %v", fixturePath, err)
	}

	if c.Expect.MinTokenCount > 0 && result.TotalTokens < c.Expect.MinTokenCount {
		return false, fmt.Sprintf("total_tokens=%d, want >= %d", result.TotalTokens, c.Expect.MinTokenCount)
	}

	for _, want := range c.Expect.LemmaPOS {
		if !hasLemmaPOS(result.Words, want.Lemma, want.POS) {
			return false, fmt.Sprintf("missing expected attachment lemma=%q pos=%q in word list %s", want.Lemma, want.POS, wordListSummary(result.Words))
		}
	}

	return true, ""
}

func udLang(lang string) (string, error) {
	switch lang {
	case "fi":
		return "FI", nil
	case "et":
		return "ET", nil
	default:
		return "", fmt.Errorf("unsupported language %q (want fi or et)", lang)
	}
}

func hasLemmaPOS(words []parsecore.WordEntry, lemma, pos string) bool {
	for _, w := range words {
		if w.Lemma == lemma && w.POS == pos {
			return true
		}
	}
	return false
}

// wordListSummary renders a compact "lemma/POS" list for failure messages so
// a FAIL line is diagnosable without re-running with -v or opening the
// fixture by hand.
func wordListSummary(words []parsecore.WordEntry) string {
	pairs := make([]string, 0, len(words))
	for _, w := range words {
		pairs = append(pairs, fmt.Sprintf("%s/%s", w.Lemma, w.POS))
	}
	sort.Strings(pairs)
	return fmt.Sprintf("%v", pairs)
}
