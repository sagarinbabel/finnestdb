// cmd/corpuspromote runs the smoke → pilot → full ladder, stopping on
// the first hard-gate failure and writing error_report.txt + updating
// promotion-state.json.
//
// v1 smoke implementation: chains extract → aggregate → verify for the
// requested profile. Pilot/full layer in fetcher driving + bytes limits.
//
// Usage:
//
//	go run ./cmd/corpuspromote -lang fi [-profile smoke|pilot|full]
//	go run ./cmd/corpuspromote -lang both
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"finnestdb/corpus_pipeline/internal/cli"
	"finnestdb/corpus_pipeline/internal/sources"
)

type stage struct {
	Profile         string `json:"profile"`
	Status          string `json:"status"` // "passed", "failed"
	RunStartUTC     string `json:"run_start_utc"`
	RunEndUTC       string `json:"run_end_utc"`
	ErrorReport     string `json:"error_report,omitempty"`
}

type promotionState struct {
	Lang                string  `json:"lang"`
	LastCompletedUTC    string  `json:"last_completed_utc,omitempty"`
	LastCompletedProfile string `json:"last_completed_profile,omitempty"`
	NextProfile         string  `json:"next_profile,omitempty"`
	Stages              []stage `json:"stages"`
}

func main() {
	var (
		dataRoot = flag.String("data-root", "../localdata", "")
		lang     = flag.String("lang", "fi", "fi | et | both")
		profile  = flag.String("profile", "smoke", "smoke | pilot | full")
	)
	flag.Parse()

	roots, err := cli.Resolve(*dataRoot, "")
	if err != nil {
		log.Fatalf("resolve: %v", err)
	}

	langs := []string{*lang}
	if *lang == "both" {
		langs = []string{"fi", "et"}
	}

	for _, l := range langs {
		if err := promote(roots, l, *profile); err != nil {
			log.Fatalf("promote %s: %v", l, err)
		}
	}
}

func promote(roots cli.Roots, lang, profile string) error {
	langLower, _, err := cli.LangCodes(lang)
	if err != nil {
		return err
	}
	derived := sources.DerivedDir(roots.DataRoot, langLower)
	if err := os.MkdirAll(derived, 0o755); err != nil {
		return err
	}

	// v1 smoke ladder: just run smoke. Pilot/full runs would here add
	// fetch step with -limit-bytes. Smoke uses the fixture source only.
	profiles := []string{"smoke"}
	if profile == "pilot" || profile == "full" {
		profiles = append(profiles, profile)
	}

	state := readPromotionState(derived, langLower)

	for _, p := range profiles {
		fmt.Fprintf(os.Stderr, "[promote %s] running profile=%s\n", langLower, p)
		st := stage{
			Profile:     p,
			RunStartUTC: time.Now().UTC().Format(time.RFC3339),
		}
		if err := runProfile(roots, langLower, p); err != nil {
			st.Status = "failed"
			st.RunEndUTC = time.Now().UTC().Format(time.RFC3339)
			st.ErrorReport = filepath.Join(derived, "errors", "error_report.txt")
			state.Stages = append(state.Stages, st)
			state.NextProfile = p
			writePromotionState(derived, state)
			return fmt.Errorf("profile %s failed: %w", p, err)
		}
		st.Status = "passed"
		st.RunEndUTC = time.Now().UTC().Format(time.RFC3339)
		state.Stages = append(state.Stages, st)
		state.LastCompletedProfile = p
		state.LastCompletedUTC = st.RunEndUTC
	}
	state.NextProfile = ""
	writePromotionState(derived, state)
	fmt.Fprintf(os.Stderr, "[promote %s] done — last completed: %s\n", langLower, state.LastCompletedProfile)
	return nil
}

func runProfile(roots cli.Roots, lang, profile string) error {
	// Run extract + aggregate + verify in sequence.
	cmds := [][]string{
		{"go", "run", "./cmd/extractcorpus", "-lang", lang, "-data-root", roots.DataRoot},
		{"go", "run", "./cmd/aggregatecorpus", "-lang", lang, "-data-root", roots.DataRoot},
		{"go", "run", "./cmd/corpusverify", "-lang", lang, "-data-root", roots.DataRoot, "-profile", profile},
	}
	for _, c := range cmds {
		fmt.Fprintf(os.Stderr, "[promote %s] $ %v\n", lang, c)
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = filepath.Join(roots.RepoRoot, "corpus_pipeline")
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%v: %w", c, err)
		}
	}
	return nil
}

func readPromotionState(derived, lang string) promotionState {
	path := filepath.Join(derived, "promotion-state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return promotionState{Lang: lang}
	}
	var s promotionState
	_ = json.Unmarshal(data, &s)
	return s
}

func writePromotionState(derived string, s promotionState) {
	path := filepath.Join(derived, "promotion-state.json")
	data, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(path, append(data, '\n'), 0o644)
}
