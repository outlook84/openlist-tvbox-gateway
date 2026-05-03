package auth

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const DefaultBcryptCost = 12

var bcryptCost = DefaultBcryptCost

var (
	ErrEmptyPassword = errors.New("password must not be empty")
	ErrInvalidCode   = errors.New("access code must be 4 to 12 digits")
	ErrUnsupported   = errors.New("unsupported password hash")
)

func HashPassword(password string) (string, error) {
	if err := ValidateAccessCode(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func ValidateAccessCode(code string) error {
	if code == "" {
		return ErrEmptyPassword
	}
	if len(code) < 4 || len(code) > 12 {
		return ErrInvalidCode
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return ErrInvalidCode
		}
	}
	return nil
}

func VerifyPassword(hash, password string) error {
	if err := ValidateAccessCode(password); err != nil {
		return err
	}
	hash = strings.TrimSpace(hash)
	hash, err := normalizeHash(hash)
	if err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func ValidateHash(hash string) error {
	hash, err := normalizeHash(hash)
	if err != nil {
		return err
	}
	_, err = bcrypt.Cost([]byte(hash))
	return err
}

func IsSupportedHash(hash string) bool {
	hash = strings.TrimSpace(hash)
	return strings.HasPrefix(hash, "$2a$") ||
		strings.HasPrefix(hash, "$2b$") ||
		strings.HasPrefix(hash, "$2y$")
}

func normalizeHash(hash string) (string, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" || !IsSupportedHash(hash) {
		return "", ErrUnsupported
	}
	if strings.HasPrefix(hash, "$2y$") {
		hash = "$2a$" + strings.TrimPrefix(hash, "$2y$")
	}
	return hash, nil
}
