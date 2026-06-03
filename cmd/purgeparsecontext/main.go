package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"finnestdb/internal/store"
)

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	olderThanDays := flag.Int("older-than-days", store.DefaultParseSourceRetentionDays, "Purge parse source text older than this many days")
	dryRun := flag.Bool("dry-run", false, "Count purgeable rows without updating the database")
	flag.Parse()

	if *olderThanDays <= 0 {
		log.Fatalf("-older-than-days must be positive")
	}
	info, err := os.Stat(*dbPath)
	if err != nil {
		log.Fatalf("database %q is not readable: %v", *dbPath, err)
	}
	if info.IsDir() {
		log.Fatalf("database path %q is a directory", *dbPath)
	}

	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	cutoff := time.Now().UTC().AddDate(0, 0, -*olderThanDays)
	if *dryRun {
		n, err := db.CountPurgeableParseSessionSourceText(cutoff)
		if err != nil {
			log.Fatalf("count purgeable parse source text: %v", err)
		}
		fmt.Printf("parse source text rows purgeable before %s UTC: %d\n", cutoff.Format(time.RFC3339), n)
		return
	}

	n, err := db.PurgeParseSessionSourceText(cutoff)
	if err != nil {
		log.Fatalf("purge parse source text: %v", err)
	}
	fmt.Printf("purged parse source text rows before %s UTC: %d\n", cutoff.Format(time.RFC3339), n)
}
