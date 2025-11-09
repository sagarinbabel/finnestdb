package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"finnestdb/internal/api"
	"finnestdb/internal/store"
)

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	flag.Parse()

	// Initialize database
	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

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

	// Serve static files
	fs := http.FileServer(http.Dir(webDir))
	mux.Handle("/", fs)

	// Start server
	addr := fmt.Sprintf(":%s", *port)
	log.Printf("Starting server on http://localhost%s", addr)
	log.Printf("Database: %s", *dbPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

