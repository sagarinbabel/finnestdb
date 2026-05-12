// doctor reports the health of a finnestdb working copy: which third-party
// data has been fetched, which derived tables have been generated, and which
// analyzer venvs and source analyzers are wired up. It is the single command
// a newcomer can run to find out why the parser is slower or less accurate
// than they expect.
//
// Exit code:
//   - 0   the dictionary is present and provenance-tagged; everything else
//     is informational
//   - 1   a critical piece is missing (no DB, no FI/ET dictionary)
//
// The intent is to surface degraded modes, not to fail the build. A run
// without FST tables, without an analyzer venv, or without Ekilex shards
// is *valid* — the parser works degraded — and the report says so out loud
// so the user makes that call deliberately rather than discovering it from
// surprise eval numbers.
package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const (
	// ANSI color codes — disabled with NO_COLOR or when stdout is not a tty.
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cRed    = "\033[31m"
	cYellow = "\033[33m"
	cGreen  = "\033[32m"
	cDim    = "\033[2m"
)

type level int

const (
	levelOK level = iota
	levelWarn
	levelFail
)

type check struct {
	name   string
	level  level
	detail string
	hint   string
}

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	flag.Parse()

	colors := termColors()

	checks := []check{}
	criticalFail := false

	dbCheck, sourceRows, sourceWarn := checkDB(*dbPath)
	checks = append(checks, dbCheck)
	if dbCheck.level == levelFail {
		criticalFail = true
	}
	if sourceWarn != nil {
		checks = append(checks, *sourceWarn)
	}

	checks = append(checks, checkFSTTables())
	checks = append(checks, checkFIAnalyser()...)
	checks = append(checks, checkETAnalyser()...)
	checks = append(checks, checkOmorfi())
	checks = append(checks, checkEstNLTK())
	checks = append(checks, checkEkilexShards())
	checks = append(checks, checkUDCache())
	checks = append(checks, checkFrequencyBaselines())
	checks = append(checks, checkRustParser())

	fmt.Println()
	fmt.Printf("%sfinnestdb doctor%s — checking %s\n", colors.bold, colors.reset, *dbPath)
	fmt.Println()

	for _, c := range checks {
		fmt.Printf("  %s  %s\n", statusBadge(c.level, colors), c.name)
		if c.detail != "" {
			for _, line := range strings.Split(c.detail, "\n") {
				fmt.Printf("       %s%s%s\n", colors.dim, line, colors.reset)
			}
		}
		if c.hint != "" {
			fmt.Printf("       %s→ %s%s\n", colors.dim, c.hint, colors.reset)
		}
	}

	if len(sourceRows) > 0 {
		fmt.Println()
		fmt.Printf("  %sDictionary source breakdown:%s\n", colors.bold, colors.reset)
		for _, row := range sourceRows {
			fmt.Printf("       %s\n", row)
		}
	}

	fmt.Println()
	mode := decideQualityMode(checks)
	fmt.Printf("  %sExpected parser quality mode:%s %s\n", colors.bold, colors.reset, mode)
	fmt.Println()

	if criticalFail {
		os.Exit(1)
	}
}

// ─── individual checks ────────────────────────────────────────────────────────

func checkDB(dbPath string) (check, []string, *check) {
	if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
		return check{
			name:   "SQLite database",
			level:  levelFail,
			detail: dbPath + " does not exist",
			hint:   "run scripts/setup-local.sh (or the more targeted `make import-dict-fi-recommended import-dict-et-recommended`)",
		}, nil, nil
	}

	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return check{
			name:   "SQLite database",
			level:  levelFail,
			detail: fmt.Sprintf("open: %v", err),
		}, nil, nil
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT lang, source, source_priority, COUNT(*) AS rows
		  FROM forms
		 GROUP BY lang, source, source_priority
		 ORDER BY lang, source_priority DESC, source`)
	if err != nil {
		return check{
			name:   "SQLite database",
			level:  levelFail,
			detail: fmt.Sprintf("query forms: %v", err),
			hint:   "schema looks unexpected; check finnestdb.db is from a recent build",
		}, nil, nil
	}
	defer rows.Close()

	var lines []string
	totalFI, totalET := 0, 0
	emptyFI, emptyET := 0, 0
	for rows.Next() {
		var lang, source string
		var prio, count int
		if err := rows.Scan(&lang, &source, &prio, &count); err != nil {
			break
		}
		switch lang {
		case "FI":
			totalFI += count
		case "ET":
			totalET += count
		}
		if source == "" && prio == 0 {
			switch lang {
			case "FI":
				emptyFI += count
			case "ET":
				emptyET += count
			}
		}
		displaySource := source
		if displaySource == "" {
			displaySource = "(empty)"
		}
		lines = append(lines, fmt.Sprintf("forms   %s  source=%-12s  priority=%-3d  %d rows",
			lang, displaySource, prio, count))
	}

	if totalFI == 0 && totalET == 0 {
		return check{
			name:   "SQLite database",
			level:  levelFail,
			detail: "no FI or ET form rows present",
			hint:   "run `make import-dict-fi-recommended import-dict-et-recommended`",
		}, lines, nil
	}

	mainCheck := check{
		name:   "SQLite database",
		level:  levelOK,
		detail: fmt.Sprintf("FI: %d forms, ET: %d forms", totalFI, totalET),
	}

	if emptyFI > 0 || emptyET > 0 {
		// Backfill runs on server / importer startup, so this state should
		// only persist on a DB that hasn't been touched since the
		// EnsureDictionarySourceColumns chain was extended. Either way,
		// surfacing the count tells the user how many rows are mislabelled.
		warn := &check{
			name:  "Dictionary provenance",
			level: levelWarn,
			detail: fmt.Sprintf("%d FI + %d ET rows have empty source / priority=0",
				emptyFI, emptyET),
			hint: "start the server once (auto-backfills to source='kaikki') or run `make verify-dict` after",
		}
		return mainCheck, lines, warn
	}
	return mainCheck, lines, nil
}

func checkFSTTables() check {
	dir := "localdata/lemmatizer-fi-et/tables"
	fi := filepath.Join(dir, "fi_min.json")
	et := filepath.Join(dir, "et_min.json")
	hasFI := fileExists(fi)
	hasET := fileExists(et)

	switch {
	case hasFI && hasET:
		return check{
			name:   "FST lemmatizer tables",
			level:  levelOK,
			detail: fmt.Sprintf("FI: %s\nET: %s", humanSize(fi), humanSize(et)),
		}
	case hasFI:
		return check{
			name:   "FST lemmatizer tables",
			level:  levelWarn,
			detail: fmt.Sprintf("FI: %s\nET: missing", humanSize(fi)),
			hint:   "ET FST generator not yet wired; FI step 5 enabled, ET runs dict-only — see docs/FST_LEMMATIZER_ROADMAP.md",
		}
	case hasET:
		return check{
			name:   "FST lemmatizer tables",
			level:  levelWarn,
			detail: fmt.Sprintf("FI: missing\nET: %s", humanSize(et)),
			hint:   "regenerate FI: `make gen-lemmatizer-tables-fi VFST_PATH=...`",
		}
	default:
		return check{
			name:   "FST lemmatizer tables",
			level:  levelWarn,
			detail: "neither fi_min.json nor et_min.json present under " + dir,
			hint:   "FST step 5 disabled; runtime degrades to dict-only. To enable FI: install libvoikko, then `make gen-lemmatizer-tables-fi VFST_PATH=/path/to/mor.vfst`",
		}
	}
}

// checkFIAnalyser reports whether libvoikko and its mor.vfst transducer
// are available locally — the prerequisites for `make gen-lemmatizer-tables-fi`.
func checkFIAnalyser() []check {
	voikko, _ := exec.LookPath("voikkospell")
	if voikko == "" {
		hint := "`brew install libvoikko` (macOS)"
		if runtime.GOOS == "linux" {
			hint = "`apt install libvoikko-dev voikko-fi` or equivalent"
		}
		return []check{{
			name:   "FI morphological analyser (libvoikko)",
			level:  levelWarn,
			detail: "voikkospell not found on PATH — FI table regeneration is unavailable",
			hint:   hint + ", then `make gen-lemmatizer-tables-fi VFST_PATH=/path/to/mor.vfst`",
		}}
	}

	vfstPath := findVFST()
	if vfstPath == "" {
		return []check{{
			name:   "FI morphological analyser (libvoikko)",
			level:  levelWarn,
			detail: "voikkospell on PATH but mor.vfst not found in standard locations",
			hint:   "locate mor.vfst manually, then `make gen-lemmatizer-tables-fi VFST_PATH=/path/to/mor.vfst`",
		}}
	}

	return []check{{
		name:   "FI morphological analyser (libvoikko)",
		level:  levelOK,
		detail: "voikkospell on PATH; mor.vfst at " + vfstPath,
		hint:   "`make gen-lemmatizer-tables-fi VFST_PATH=" + vfstPath + "`",
	}}
}

// findVFST searches standard locations for Voikko's mor.vfst.
func findVFST() string {
	candidates := []string{
		"/opt/homebrew/lib/voikko/5/mor-standard/mor.vfst",
		"/usr/local/lib/voikko/5/mor-standard/mor.vfst",
		"/usr/lib/voikko/5/mor-standard/mor.vfst",
		"/usr/share/voikko/5/mor-standard/mor.vfst",
	}
	// Also check the brew cellar dynamically.
	if brewPrefix, err := exec.Command("brew", "--prefix", "libvoikko").Output(); err == nil {
		p := filepath.Join(strings.TrimSpace(string(brewPrefix)), "lib/voikko/5/mor-standard/mor.vfst")
		candidates = append([]string{p}, candidates...)
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// checkETAnalyser reports whether the Giellalt hfstol transducer is
// present locally — the prerequisite for `make gen-lemmatizer-tables-et`.
func checkETAnalyser() []check {
	hfstolPath := findHFSTOL()
	if hfstolPath == "" {
		return []check{{
			name:   "ET morphological analyser (Giellalt hfstol)",
			level:  levelWarn,
			detail: "analyser-gt-desc.hfstol not found in canonical or known legacy local locations",
			hint:   "place it at localdata/lemmatizer-fi-et/analyser-gt-desc.hfstol, then `make gen-lemmatizer-tables-et`; see docs/LOCAL_TOOLING.md before assuming it is absent",
		}}
	}

	canonical := "localdata/lemmatizer-fi-et/analyser-gt-desc.hfstol"
	if hfstolPath != canonical {
		return []check{{
			name:   "ET morphological analyser (Giellalt hfstol)",
			level:  levelOK,
			detail: "analyser-gt-desc.hfstol found at legacy/local path: " + hfstolPath + "\ncanonical path is " + canonical,
			hint:   "`make gen-lemmatizer-tables-et HFSTOL_PATH=" + hfstolPath + "` or symlink/copy it into localdata/lemmatizer-fi-et/",
		}}
	}

	return []check{{
		name:   "ET morphological analyser (Giellalt hfstol)",
		level:  levelOK,
		detail: "analyser-gt-desc.hfstol at " + hfstolPath,
		hint:   "`make gen-lemmatizer-tables-et`",
	}}
}

// findHFSTOL searches canonical and known legacy locations for the ET hfstol
// transducer. Canonical localdata wins, but old agent worktrees may still
// contain local-only package-data copies that can be passed as HFSTOL_PATH.
func findHFSTOL() string {
	candidates := findHFSTOLCandidates()
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func findHFSTOLCandidates() []string {
	ordered := []string{
		"localdata/lemmatizer-fi-et/analyser-gt-desc.hfstol",
		"pkg/lemmatizer-fi-et/data/et/analyser-gt-desc.hfstol",
		"data/lemmatizer-fi-et/analyser-gt-desc.hfstol",
	}

	if matches, err := filepath.Glob(".claude/worktrees/*/pkg/lemmatizer-fi-et/data/et/analyser-gt-desc.hfstol"); err == nil {
		sort.Strings(matches)
		ordered = append(ordered, matches...)
	}

	seen := map[string]bool{}
	found := []string{}
	for _, p := range ordered {
		if !seen[p] && fileExists(p) {
			found = append(found, p)
			seen[p] = true
		}
	}
	return found
}

func checkOmorfi() check {
	adapter := "scripts/omorfi_adapter_example.py"
	model := filepath.Join(homeDir(), ".cache/omorfi/omorfi.analyse.hfst")

	// Prefer the unified .venv, fall back to legacy .venv-omorfi.
	hasVenv := fileExists(".venv/bin/python") || fileExists(".venv-omorfi/bin/python")
	hasAdapter := fileExists(adapter)
	hasModel := fileExists(model) || fileExists(".cache/omorfi/omorfi.analyse.hfst")

	if hasVenv && hasAdapter && hasModel {
		return check{
			name:   "Omorfi analyzer (FI baseline)",
			level:  levelOK,
			detail: ".venv + adapter + model present; parser-comparison.sh auto-wires FINNESTDB_OMORFI_CMD",
		}
	}
	missing := []string{}
	if !hasVenv {
		missing = append(missing, ".venv/")
	}
	if !hasAdapter {
		missing = append(missing, adapter)
	}
	if !hasModel {
		missing = append(missing, "~/.cache/omorfi/omorfi.analyse.hfst")
	}
	return check{
		name:   "Omorfi analyzer (FI baseline)",
		level:  levelWarn,
		detail: "missing: " + strings.Join(missing, ", "),
		hint:   "`make setup-nlp` (without it, `make compare-parsers` refuses to run unless --allow-missing-baseline)",
	}
}

func checkEstNLTK() check {
	adapter := "scripts/estnltk_adapter_example.py"

	// Prefer the unified .venv, fall back to legacy .venv-estnltk.
	hasVenv := fileExists(".venv/bin/python") || fileExists(".venv-estnltk/bin/python")
	hasAdapter := fileExists(adapter)

	if hasVenv && hasAdapter {
		return check{
			name:   "EstNLTK analyzer (ET baseline)",
			level:  levelOK,
			detail: ".venv + adapter present; parser-comparison-et.sh auto-detects",
		}
	}
	missing := []string{}
	if !hasVenv {
		missing = append(missing, ".venv/")
	}
	if !hasAdapter {
		missing = append(missing, adapter)
	}
	return check{
		name:   "EstNLTK analyzer (ET baseline)",
		level:  levelWarn,
		detail: "missing: " + strings.Join(missing, ", "),
		hint:   "`make setup-nlp`",
	}
}

func checkEkilexShards() check {
	defs := "localdata/ekilex/definitions"
	forms := "localdata/ekilex/forms"
	hasDefs := dirNonEmpty(defs)
	hasForms := dirNonEmpty(forms)
	switch {
	case hasDefs && hasForms:
		return check{
			name:   "Ekilex reduced shards (ET enrichment)",
			level:  levelOK,
			detail: defs + " + " + forms + " populated",
		}
	case hasDefs || hasForms:
		return check{
			name:   "Ekilex reduced shards (ET enrichment)",
			level:  levelWarn,
			detail: fmt.Sprintf("definitions=%v forms=%v — partial", hasDefs, hasForms),
			hint:   "rerun `make reduce-ekilex` after a full `make fetch-ekilex`",
		}
	default:
		return check{
			name:   "Ekilex reduced shards (ET enrichment)",
			level:  levelWarn,
			detail: "no shards under " + defs + " or " + forms,
			hint:   "ET enrichment is degraded without these. Set EKILEX_API_KEY and rerun scripts/setup-local.sh, or fetch+reduce manually",
		}
	}
}

func checkUDCache() check {
	cache := "localdata/ud-cache"
	if !dirNonEmpty(cache) {
		return check{
			name:   "UD treebank cache",
			level:  levelWarn,
			detail: cache + " missing or empty",
			hint:   "`make import-ud-gold` (FI test/dev/PUD/OOD/FTB committed; ET test/dev local-only because of CC BY-NC-SA)",
		}
	}
	return check{
		name:   "UD treebank cache",
		level:  levelOK,
		detail: cache + " present",
	}
}

func checkFrequencyBaselines() check {
	osFi := "localdata/frequency/fi/opensubtitles-2018-fi-50k.txt"
	osEt := "localdata/frequency/et/opensubtitles-2018-et-50k.txt"
	hasFi := fileExists(osFi)
	hasEt := fileExists(osEt)
	switch {
	case hasFi && hasEt:
		return check{
			name:  "Frequency baselines (calibration)",
			level: levelOK,
		}
	case hasFi || hasEt:
		return check{
			name:   "Frequency baselines (calibration)",
			level:  levelWarn,
			detail: fmt.Sprintf("fi=%v et=%v — partial", hasFi, hasEt),
			hint:   "`make fetch-frequency-baselines`",
		}
	default:
		return check{
			name:   "Frequency baselines (calibration)",
			level:  levelWarn,
			detail: "no opensubtitles or UD-derived frequency lists present",
			hint:   "`make fetch-frequency-baselines` (~10 MB)",
		}
	}
}

func checkRustParser() check {
	candidates := []string{
		"parser/target/release/libparser.dylib",
		"parser/target/release/libparser.so",
		"parser/target/release/parser.dll",
	}
	for _, p := range candidates {
		if fileExists(p) {
			return check{
				name:   "Rust parser shared library",
				level:  levelOK,
				detail: p,
			}
		}
	}
	return check{
		name:   "Rust parser shared library",
		level:  levelWarn,
		detail: "release build not found under parser/target/release/",
		hint:   "`make parser` (cargo build --release)",
	}
}

// ─── output helpers ───────────────────────────────────────────────────────────

type colorScheme struct {
	bold, dim, reset, red, yellow, green string
}

func termColors() colorScheme {
	if os.Getenv("NO_COLOR") != "" {
		return colorScheme{}
	}
	if fi, err := os.Stdout.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
		return colorScheme{}
	}
	return colorScheme{
		bold: cBold, dim: cDim, reset: cReset,
		red: cRed, yellow: cYellow, green: cGreen,
	}
}

func statusBadge(l level, c colorScheme) string {
	switch l {
	case levelOK:
		return c.green + "OK   " + c.reset
	case levelWarn:
		return c.yellow + "WARN " + c.reset
	case levelFail:
		return c.red + "FAIL " + c.reset
	}
	return "     "
}

func decideQualityMode(cs []check) string {
	// Walk the checks looking for the analyzer + FST signals. The parser
	// runs in three coarse modes that map directly onto these:
	hasFI := false
	hasET := false
	hasOmorfi := false
	hasEstNLTK := false
	hasFSTFI := false
	hasFSTET := false
	hasEkilex := false

	for _, c := range cs {
		switch c.name {
		case "SQLite database":
			if c.level != levelFail {
				hasFI = !strings.Contains(c.detail, "FI: 0")
				hasET = !strings.Contains(c.detail, "ET: 0")
			}
		case "FST lemmatizer tables":
			if c.level == levelOK {
				hasFSTFI = true
				hasFSTET = true
			} else {
				hasFSTFI = strings.Contains(c.detail, "FI: ") && !strings.Contains(c.detail, "FI: missing")
				hasFSTET = strings.Contains(c.detail, "ET: ") && !strings.Contains(c.detail, "ET: missing")
			}
		case "Omorfi analyzer (FI baseline)":
			hasOmorfi = c.level == levelOK
		case "EstNLTK analyzer (ET baseline)":
			hasEstNLTK = c.level == levelOK
		case "Ekilex reduced shards (ET enrichment)":
			hasEkilex = c.level == levelOK
		}
	}

	parts := []string{}
	if hasFI {
		fi := "FI dict-only"
		if hasFSTFI {
			fi = "FI dict + FST"
		}
		if hasOmorfi {
			fi += " (+ omorfi baseline available)"
		}
		parts = append(parts, fi)
	}
	if hasET {
		et := "ET dict-only"
		if hasEkilex && hasFSTET {
			et = "ET dict + Ekilex + FST"
		} else if hasEkilex {
			et = "ET dict + Ekilex enrichment"
		} else if hasFSTET {
			et = "ET dict + FST"
		}
		if hasEstNLTK {
			et += " (+ estnltk baseline available)"
		}
		parts = append(parts, et)
	}
	if len(parts) == 0 {
		return "no dictionary present"
	}
	return strings.Join(parts, " | ")
}

// ─── filesystem helpers ───────────────────────────────────────────────────────

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func dirNonEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		// Hidden files don't count as content.
		if !strings.HasPrefix(e.Name(), ".") {
			return true
		}
	}
	return false
}

func humanSize(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return path
	}
	n := fi.Size()
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%s (%.1f GB)", path, float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%s (%.1f MB)", path, float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%s (%.1f KB)", path, float64(n)/(1<<10))
	}
	return fmt.Sprintf("%s (%d B)", path, n)
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}
