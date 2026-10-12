package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// A token is "v1.<user id>.<expiry unix seconds>.<signature>", where the
// signature is HMAC-SHA256 over everything before it. Checking one needs only
// the key — never the store (SPEC, Authentication).

// MinKeyLen is the shortest signing key accepted.
const MinKeyLen = 32

var errBadToken = errors.New("invalid token")

// Tokens signs and verifies bearer tokens with one key.
type Tokens struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// NewTokens returns a signer; the key must be at least MinKeyLen bytes.
func NewTokens(key []byte, ttl time.Duration) (*Tokens, error) {
	if len(key) < MinKeyLen {
		return nil, fmt.Errorf("token key must be at least %d bytes, got %d", MinKeyLen, len(key))
	}
	return &Tokens{key: key, ttl: ttl, now: time.Now}, nil
}

// Issue returns a token for userID and when it expires.
func (t *Tokens) Issue(userID int64) (string, time.Time) {
	exp := t.now().Add(t.ttl).Truncate(time.Second)
	return t.Sign(userID, exp), exp
}

// Sign builds a token with an explicit expiry. Issue is the normal path; the
// conformance suite uses Sign to forge expired tokens.
func (t *Tokens) Sign(userID int64, exp time.Time) string {
	payload := "v1." + strconv.FormatInt(userID, 10) + "." + strconv.FormatInt(exp.Unix(), 10)
	return payload + "." + t.sign(payload)
}

// Verify returns the user ID in a valid, unexpired token.
func (t *Tokens) Verify(token string) (int64, error) {
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return 0, errBadToken
	}
	payload, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(t.sign(payload))) {
		return 0, errBadToken
	}
	parts := strings.Split(payload, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return 0, errBadToken
	}
	userID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, errBadToken
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || !t.now().Before(time.Unix(exp, 0)) {
		return 0, errBadToken
	}
	return userID, nil
}

func (t *Tokens) sign(payload string) string {
	mac := hmac.New(sha256.New, t.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
