package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/internal/api"
	"finnestdb/internal/store"
)

func main() {
	defaultPort := "8080"
	if envPort := strings.TrimSpace(os.Getenv("PORT")); envPort != "" {
		defaultPort = envPort
	}
	port := flag.String("port", defaultPort, "Port to listen on")
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	flag.Parse()

	// Initialize database
	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Check dictionary status for each language and report clearly.
	missingDicts := []string{}
	for _, lang := range []string{"FI", "ET"} {
		n, err := db.FormsCount(lang)
		if err != nil {
			log.Printf("warn: could not check %s dictionary: %v", lang, err)
			continue
		}
		if n == 0 {
			missingDicts = append(missingDicts, strings.ToLower(lang))
		} else {
			log.Printf("%s dictionary loaded: %d forms", lang, n)
		}
	}
	if len(missingDicts) > 0 {
		log.Printf("WARNING: no dictionary data for [%s]. Definitions will be blank and lemmatization will be approximate.",
			strings.Join(missingDicts, ", "))
		log.Printf("  To fix, run:  make import-dict")
	}

	// Initialize API
	apiHandler := api.NewAPI(db)

	// Setup routes
	mux := http.NewServeMux()
	apiHandler.SetupRoutes(mux)

	// Serve static files from /web directory
	webDir := filepath.Join(".", "web")
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		log.Fatalf("Web directory not found: %s", webDir)
	}

	// Serve static files. During local UI iteration, stale cached CSS/JS is more
	// confusing than helpful, so make the shipped web assets revalidate on load.
	fs := http.FileServer(http.Dir(webDir))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fs.ServeHTTP(w, r)
	}))

	// Start server
	addr := fmt.Sprintf(":%s", *port)
	log.Printf("Starting server on http://localhost%s", addr)
	log.Printf("Database: %s", *dbPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
