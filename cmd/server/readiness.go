package main

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const (
	appEnvVar                = "APP_ENV"
	allowDegradedDBEnvVar    = "FINNESTDB_ALLOW_DEGRADED_DB"
	minFormsFIEnvVar         = "FINNESTDB_PRODUCTION_MIN_FORMS_FI"
	minFormsETEnvVar         = "FINNESTDB_PRODUCTION_MIN_FORMS_ET"
	defaultProductionFIForms = 20_000_000
	defaultProductionETForms = 6_000_000
)

type dictionaryRequirement struct {
	Lang     string
	MinForms int64
}

type dictionaryReadiness struct {
	Counts map[string]int64
}

func productionMode() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(appEnvVar)), "production")
}

func allowDegradedDB() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(allowDegradedDBEnvVar))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func requireProductionDBReady(dbPath string) (*dictionaryReadiness, error) {
	if !productionMode() || allowDegradedDB() {
		return nil, nil
	}
	reqs, err := productionDictionaryRequirements()
	if err != nil {
		return nil, err
	}
	return checkDictionaryDBReady(dbPath, reqs)
}

func productionDictionaryRequirements() ([]dictionaryRequirement, error) {
	fi, err := envInt64(minFormsFIEnvVar, defaultProductionFIForms)
	if err != nil {
		return nil, err
	}
	et, err := envInt64(minFormsETEnvVar, defaultProductionETForms)
	if err != nil {
		return nil, err
	}
	return []dictionaryRequirement{
		{Lang: "FI", MinForms: fi},
		{Lang: "ET", MinForms: et},
	}, nil
}

func envInt64(name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return n, nil
}

func checkDictionaryDBReady(dbPath string, reqs []dictionaryRequirement) (*dictionaryReadiness, error) {
	info, err := os.Stat(dbPath)
	if err != nil {
		return nil, fmt.Errorf("database %q is required in production: %w", dbPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("database %q is a directory", dbPath)
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("database %q is empty", dbPath)
	}

	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open database read-only: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("open database read-only: %w", err)
	}

	var hasForms int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='forms'`).Scan(&hasForms); err != nil {
		return nil, fmt.Errorf("check forms table: %w", err)
	}
	if hasForms == 0 {
		return nil, fmt.Errorf("database %q is missing forms table", dbPath)
	}

	ready := &dictionaryReadiness{Counts: map[string]int64{}}
	for _, req := range reqs {
		var count int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM forms WHERE lang = ?`, req.Lang).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s forms: %w", req.Lang, err)
		}
		ready.Counts[req.Lang] = count
		if count < req.MinForms {
			return nil, fmt.Errorf("database %q has %d %s forms; production requires at least %d", dbPath, count, req.Lang, req.MinForms)
		}
	}
	return ready, nil
}
