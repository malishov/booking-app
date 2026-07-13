package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	const pw = "correct horse battery"

	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == pw {
		t.Fatal("hash must not equal the plaintext password")
	}
	if !CheckPassword(hash, pw) {
		t.Fatal("CheckPassword should accept the correct password")
	}
	if CheckPassword(hash, "wrong password") {
		t.Fatal("CheckPassword should reject an incorrect password")
	}
}
