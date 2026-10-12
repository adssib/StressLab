package api

import (
	"strings"
	"testing"
	"time"
)

var testKey = []byte(strings.Repeat("k", MinKeyLen))

func TestTokenRoundTrip(t *testing.T) {
	tok, _ := mustTokens(t).Issue(42)
	id, err := mustTokens(t).Verify(tok)
	if err != nil || id != 42 {
		t.Fatalf("want user 42, got %d, %v", id, err)
	}
}

func TestTokenRejects(t *testing.T) {
	tokens := mustTokens(t)
	valid, _ := tokens.Issue(42)
	other, err := NewTokens([]byte(strings.Repeat("x", MinKeyLen)), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, _ := other.Issue(42)

	cases := map[string]string{
		"empty":            "",
		"garbage":          "not-a-token",
		"other key":        otherKey,
		"tampered user id": strings.Replace(valid, "v1.42.", "v1.43.", 1),
		"tampered sig":     valid[:len(valid)-1] + flip(valid[len(valid)-1]),
		"expired":          tokens.Sign(42, time.Now().Add(-time.Second)),
		"wrong version":    "v2" + valid[2:],
	}
	for name, tok := range cases {
		if _, err := tokens.Verify(tok); err == nil {
			t.Errorf("%s: want rejected, got accepted", name)
		}
	}
}

func TestShortKeyRefused(t *testing.T) {
	if _, err := NewTokens([]byte("short"), time.Hour); err == nil {
		t.Fatal("a short signing key must be refused")
	}
}

func mustTokens(t *testing.T) *Tokens {
	t.Helper()
	tokens, err := NewTokens(testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

func flip(c byte) string {
	if c == 'A' {
		return "B"
	}
	return "A"
}
