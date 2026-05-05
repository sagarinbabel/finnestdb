package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("expected argon2id PHC prefix, got %q", hash)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Fatal("VerifyPassword: correct password did not verify")
	}
	if VerifyPassword("wrong password", hash) {
		t.Fatal("VerifyPassword: wrong password verified")
	}
}

func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestHashPasswordProducesDistinctHashes(t *testing.T) {
	a, err := HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("expected distinct salts to produce distinct hashes")
	}
	if !VerifyPassword("same", a) || !VerifyPassword("same", b) {
		t.Fatal("both hashes should verify the same password")
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"plaintext",
		"$bcrypt$xxx",
		"$argon2id$v=19$m=64,t=3,p=2$invalid",
	}
	for _, c := range cases {
		if VerifyPassword("anything", c) {
			t.Errorf("VerifyPassword accepted malformed hash %q", c)
		}
	}
}

func TestSessionTokenAndHashTokenAreDeterministic(t *testing.T) {
	tok, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 32 {
		t.Fatalf("session token too short: %q", tok)
	}
	if HashToken(tok) != HashToken(tok) {
		t.Fatal("HashToken should be deterministic")
	}
	other, _ := NewSessionToken()
	if tok == other {
		t.Fatal("expected distinct session tokens across calls")
	}
	if HashToken(tok) == HashToken(other) {
		t.Fatal("expected distinct hashes for distinct tokens")
	}
}
