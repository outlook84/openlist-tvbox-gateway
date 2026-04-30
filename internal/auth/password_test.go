package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordVerifies(t *testing.T) {
	hash, err := HashPassword("123456")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword(hash, "123456"); err != nil {
		t.Fatalf("verify generated hash: %v", err)
	}
	if err := VerifyPassword(hash, "bad"); err == nil {
		t.Fatal("expected bad password to fail")
	}
}

func TestHashPasswordRejectsNonNumericCode(t *testing.T) {
	if _, err := HashPassword("abc123"); err == nil {
		t.Fatal("expected non-numeric code to fail")
	}
}

func TestHashPasswordRejectsShortCode(t *testing.T) {
	if _, err := HashPassword("123"); err == nil {
		t.Fatal("expected short code to fail")
	}
}

func TestVerifyPasswordAcceptsHtpasswdBcryptPrefix(t *testing.T) {
	hash, err := HashPassword("123456")
	if err != nil {
		t.Fatal(err)
	}
	htpasswd := "$2y$" + strings.TrimPrefix(hash, "$2a$")
	if err := VerifyPassword(htpasswd, "123456"); err != nil {
		t.Fatalf("verify $2y$ hash: %v", err)
	}
}

func TestVerifyPasswordRejectsUnsupportedHash(t *testing.T) {
	if err := VerifyPassword("sha256:abc", "123456"); err == nil {
		t.Fatal("expected unsupported hash error")
	}
}

func TestValidateHashRejectsMalformedBcrypt(t *testing.T) {
	if err := ValidateHash("$2a$12$replace-with-bcrypt-hash"); err == nil {
		t.Fatal("expected malformed bcrypt hash error")
	}
}
