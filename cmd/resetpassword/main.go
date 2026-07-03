// Command resetpassword is the operator-side password tool for the alpha,
// where no self-service reset flow exists yet. It sets (or generates) a new
// password for an account, revokes the account's active sessions, and can
// pre-register accounts before public launch — which is how FINNESTDB_ADMIN_EMAILS
// addresses must be claimed, since that env var grants admin to whoever
// registers the email first.
//
// Usage:
//
//	go run ./cmd/resetpassword -email user@example.com                 # generate + print a new password
//	go run ./cmd/resetpassword -email user@example.com -password '...' # set a specific password
//	go run ./cmd/resetpassword -email admin@example.com -create        # pre-register (admin if in FINNESTDB_ADMIN_EMAILS)
//	go run ./cmd/resetpassword -email user@example.com -admin          # also grant is_admin
package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"

	"finnestdb/internal/auth"
	"finnestdb/internal/store"
)

const minPasswordLength = 8

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	email := flag.String("email", "", "Account email (required)")
	password := flag.String("password", "", "New password; generated and printed when empty")
	create := flag.Bool("create", false, "Create the account if it does not exist (pre-registration)")
	admin := flag.Bool("admin", false, "Grant is_admin to the account")
	keepSessions := flag.Bool("keep-sessions", false, "Do not revoke the account's active sessions")
	flag.Parse()

	if *email == "" {
		log.Fatal("-email is required")
	}
	info, err := os.Stat(*dbPath)
	if err != nil {
		log.Fatalf("database %q is not readable: %v", *dbPath, err)
	}
	if info.IsDir() {
		log.Fatalf("database path %q is a directory", *dbPath)
	}

	pw := *password
	generated := false
	if pw == "" {
		pw, err = generatePassword()
		if err != nil {
			log.Fatalf("generate password: %v", err)
		}
		generated = true
	}
	if len(pw) < minPasswordLength {
		log.Fatalf("password must be at least %d characters", minPasswordLength)
	}

	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	hash, err := auth.HashPassword(pw)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	user, err := db.GetUserByEmail(*email)
	switch {
	case err == sql.ErrNoRows:
		if !*create {
			log.Fatalf("no account for %q (use -create to pre-register it)", *email)
		}
		user, err = db.CreateUser(*email, hash)
		if err != nil {
			log.Fatalf("create user: %v", err)
		}
		fmt.Printf("created account %s (id %d, admin=%v)\n", user.Email, user.ID, user.IsAdmin)
	case err != nil:
		log.Fatalf("look up %q: %v", *email, err)
	default:
		if err := db.SetUserPassword(user.ID, hash); err != nil {
			log.Fatalf("set password: %v", err)
		}
		fmt.Printf("password updated for %s (id %d)\n", user.Email, user.ID)
	}

	if *admin && !user.IsAdmin {
		if err := db.SetUserAdmin(user.ID, true); err != nil {
			log.Fatalf("set admin: %v", err)
		}
		fmt.Printf("granted admin to %s\n", user.Email)
	}

	if !*keepSessions {
		n, err := db.RevokeAllSessionsForUser(user.ID)
		if err != nil {
			log.Fatalf("revoke sessions: %v", err)
		}
		fmt.Printf("revoked %d active session(s)\n", n)
	}

	if generated {
		fmt.Printf("new password: %s\n", pw)
		fmt.Println("share it over a trusted channel and ask the user to change it after signing in")
	}
}

// generatePassword returns a URL-safe random password well above the minimum
// length policy.
func generatePassword() (string, error) {
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
