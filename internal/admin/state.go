package admin

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"openlist-tvbox/internal/auth"
)

type adminState struct {
	Hash          string
	SetupCodePath string
	SetupCode     string
}

func loadAdminState(configPath string) (adminState, error) {
	if code := strings.TrimSpace(os.Getenv(envAdminCode)); code != "" {
		hash, err := hashAdminCode(code)
		if err != nil {
			return adminState{}, err
		}
		return adminState{Hash: hash}, nil
	}
	if hash := strings.TrimSpace(os.Getenv(envAdminCodeHash)); hash != "" {
		if err := auth.ValidateHash(hash); err != nil {
			return adminState{}, fmt.Errorf("%s is not a valid bcrypt hash", envAdminCodeHash)
		}
		return adminState{Hash: hash}, nil
	}
	secretPath := adminSecretPath(configPath)
	data, err := os.ReadFile(secretPath)
	if err == nil {
		hash := strings.TrimSpace(string(data))
		if err := auth.ValidateHash(hash); err != nil {
			return adminState{}, fmt.Errorf("admin secret file contains invalid hash")
		}
		return adminState{Hash: hash}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return adminState{}, fmt.Errorf("read admin secret file: %w", err)
	}
	setupPath := adminSetupCodePath(configPath)
	if data, err := os.ReadFile(setupPath); err == nil {
		code := strings.TrimSpace(string(data))
		if err := validateAdminCode(code); err != nil {
			return adminState{}, fmt.Errorf("admin setup code file contains invalid code")
		}
		return adminState{SetupCodePath: setupPath, SetupCode: code}, nil
	}
	code, err := randomDigits(12)
	if err != nil {
		return adminState{}, err
	}
	if err := os.WriteFile(setupPath, []byte(code+"\n"), 0o600); err != nil {
		return adminState{}, fmt.Errorf("write admin setup code file: %w", err)
	}
	return adminState{SetupCodePath: setupPath, SetupCode: code}, nil
}

func adminSecretPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), secretFileName)
}

func adminSetupCodePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), setupCodeFileName)
}

func randomDigits(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = '0' + (buf[i] % 10)
	}
	return string(buf), nil
}

func hashAdminCode(code string) (string, error) {
	if err := validateAdminCode(code); err != nil {
		return "", newCodedError("admin.access_code.invalid", err.Error(), map[string]any{"min": 8, "max": 64})
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(code), adminBcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyAdminCode(hash, code string) error {
	if err := validateAdminCode(code); err != nil {
		return err
	}
	hash = strings.TrimSpace(hash)
	if strings.HasPrefix(hash, "$2y$") {
		hash = "$2a$" + strings.TrimPrefix(hash, "$2y$")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(code))
}

func verifySetupCode(want, got string) error {
	if err := validateAdminCode(got); err != nil {
		return err
	}
	if subtleConstantTimeCompare(want, got) {
		return nil
	}
	return errors.New("invalid setup code")
}

func subtleConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func validateAdminCode(code string) error {
	if len(code) < 8 || len(code) > 64 {
		return errors.New("admin access code must be 8 to 64 characters")
	}
	for _, r := range code {
		if r <= 32 || r == 127 {
			return errors.New("admin access code must not contain whitespace or control characters")
		}
	}
	return nil
}
