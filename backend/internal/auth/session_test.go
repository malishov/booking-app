package auth

import (
	"encoding/base64"
	"testing"
)

func TestSessionTokens(t *testing.T) {
	first, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil || len(decoded) != 32 || first == second {
		t.Fatal("invalid or repeated random token")
	}
	hash := SessionTokenHash(first)
	if len(hash) != 64 || hash == first || hash != SessionTokenHash(first) || hash == SessionTokenHash(second) {
		t.Fatal("invalid token hash")
	}
}
