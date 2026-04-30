package config

import (
	"os"
	"path/filepath"
	"testing"

	"openlist-tvbox/internal/auth"
)

func TestValidateRejectsDuplicateMountID(t *testing.T) {
	search := true
	cfg := Config{
		Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []Subscription{{
			Mounts: []Mount{
				{ID: "m1", Backend: "b1", Path: "/", Search: &search},
				{ID: "m1", Backend: "b1", Path: "/other"},
			},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate mount id error")
	}
}

func TestValidateRejectsUnsafeMountPath(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
		Subs:     []Subscription{{Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "../secret"}}}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsafe path error")
	}
}

func TestValidateRejectsSensitivePlayHeader(t *testing.T) {
	for _, header := range []string{"Cookie", "Proxy-Authorization"} {
		cfg := Config{
			Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
			Subs: []Subscription{{
				Mounts: []Mount{{
					ID:          "m1",
					Backend:     "b1",
					Path:        "/",
					PlayHeaders: map[string]string{header: "secret=value"},
				}},
			}},
		}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected sensitive play header error for %q", header)
		}
	}
}

func TestValidateNormalizesPlayHeaders(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []Subscription{{
			Mounts: []Mount{{
				ID:          "m1",
				Backend:     "b1",
				Path:        "/",
				PlayHeaders: map[string]string{"user-agent": " Custom-UA "},
			}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Subs[0].Mounts[0].PlayHeaders["User-Agent"]; got != "Custom-UA" {
		t.Fatalf("User-Agent = %q", got)
	}
}

func TestLoadYAMLWithMultipleSubs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
public_base_url: http://127.0.0.1:18989
trust_x_forwarded_for: true
tvbox:
  site_key: openlist_tvbox
  site_name: OpenList
backends:
  - id: main
    server: https://openlist.example.com
  - id: backup
    server: https://backup.example.com
subs:
  - id: movies
    path: /sub/movies
    site_key: movies
    site_name: Movies
    mounts:
      - id: hd
        name: HD
        backend: main
        path: /Videos/Movies
      - id: archive
        backend: backup
        path: /Archive/Movies
  - id: shows
    path: /tv/shows
    mounts:
      - id: tv
        backend: main
        path: /Videos/Shows
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Subs) != 2 {
		t.Fatalf("subs = %#v", cfg.Subs)
	}
	if !cfg.TrustXForwardedFor {
		t.Fatal("trust_x_forwarded_for was not loaded")
	}
	if cfg.Subs[0].Mounts[0].Path != "/Videos/Movies" {
		t.Fatalf("mount path = %q", cfg.Subs[0].Mounts[0].Path)
	}
	if cfg.Subs[1].Path != "/tv/shows" {
		t.Fatalf("sub path = %q", cfg.Subs[1].Path)
	}
	if cfg.Subs[0].TVBox.SiteKey != "movies" || cfg.Subs[0].TVBox.SiteName != "Movies" {
		t.Fatalf("sub tvbox identity = %#v", cfg.Subs[0].TVBox)
	}
	if cfg.Subs[1].TVBox.SiteKey != "openlist_tvbox" || cfg.Subs[1].TVBox.SiteName != "OpenList" {
		t.Fatalf("sub inherited tvbox identity = %#v", cfg.Subs[1].TVBox)
	}
}

func TestSubTVBoxOverridesGlobalOptions(t *testing.T) {
	cfg := Config{
		TVBox:    TVBox{SiteKey: "global_key", SiteName: "Global", Timeout: 20},
		Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []Subscription{{
			ID:     "movies",
			TVBox:  TVBox{Timeout: 30},
			Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Subs[0].TVBox.SiteKey != "global_key" || cfg.Subs[0].TVBox.SiteName != "Global" {
		t.Fatalf("sub identity = %#v", cfg.Subs[0].TVBox)
	}
	if cfg.Subs[0].TVBox.Timeout != 30 {
		t.Fatalf("sub timeout = %d", cfg.Subs[0].TVBox.Timeout)
	}
}

func TestValidateRejectsDuplicateSubPath(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []Subscription{
			{ID: "a", Path: "/sub/a", Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}}},
			{ID: "b", Path: "/sub/a/", Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate sub path error")
	}
}

func TestValidateRequiresSubs(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing subs error")
	}
}

func TestValidateAcceptsBcryptAccessCodeHash(t *testing.T) {
	hash, err := auth.HashPassword("123456")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []Subscription{{
			ID:             "a",
			AccessCodeHash: hash,
			Mounts:         []Mount{{ID: "m1", Backend: "b1", Path: "/"}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsPlaintextAccessCode(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []Subscription{{
			ID:         "a",
			AccessCode: "123456",
			Mounts:     []Mount{{ID: "m1", Backend: "b1", Path: "/"}},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected plaintext access_code error")
	}
}

func TestValidateRejectsUnsupportedAccessCodeHash(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []Subscription{{
			ID:             "a",
			AccessCodeHash: "sha256:abc",
			Mounts:         []Mount{{ID: "m1", Backend: "b1", Path: "/"}},
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsupported access_code_hash error")
	}
}

func TestValidateDefaultsBackendAuthModes(t *testing.T) {
	cfg := Config{
		Backends: []Backend{
			{ID: "token", Server: "https://example.com", AuthType: "api_key", APIKey: "secret"},
			{ID: "guest", Server: "https://guest.example.com"},
		},
		Subs: []Subscription{{
			Mounts: []Mount{
				{ID: "m1", Backend: "token", Path: "/"},
				{ID: "m2", Backend: "guest", Path: "/"},
			},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Backends[0].APIKey != "secret" {
		t.Fatal("api_key was not preserved")
	}
	if cfg.Backends[1].AuthType != "anonymous" {
		t.Fatalf("guest auth_type = %q", cfg.Backends[1].AuthType)
	}
	if cfg.Backends[1].APIKey != "" || cfg.Backends[1].User != "" || cfg.Backends[1].Password != "" {
		t.Fatalf("guest backend has auth fields: %#v", cfg.Backends[1])
	}
	if cfg.Backends[0].Version != "v3" || cfg.Backends[1].Version != "v3" {
		t.Fatalf("backend versions = %#v", cfg.Backends)
	}
}

func TestValidateRejectsUnsupportedBackendVersion(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{ID: "b1", Server: "https://example.com", Version: "v2"}},
		Subs:     []Subscription{{Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}}}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsupported backend version error")
	}
}

func TestValidateBackendAPIKeyFromEnv(t *testing.T) {
	t.Setenv("OPENLIST_TEST_API_KEY", "secret-token")
	cfg := Config{
		Backends: []Backend{{
			ID:        "b1",
			Server:    "https://example.com",
			AuthType:  "api_key",
			APIKeyEnv: "OPENLIST_TEST_API_KEY",
		}},
		Subs: []Subscription{{Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Backends[0].APIKey != "secret-token" {
		t.Fatal("api_key_env was not resolved")
	}
}

func TestValidateRejectsMixedBackendAPIKey(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{
			ID:        "b1",
			Server:    "https://example.com",
			AuthType:  "api_key",
			APIKey:    "secret",
			APIKeyEnv: "OPENLIST_TEST_API_KEY",
		}},
		Subs: []Subscription{{Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}}}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected mixed api_key error")
	}
}

func TestValidateBackendPasswordAuthFromTopLevelFields(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{
			ID:       "b1",
			Server:   "https://example.com",
			AuthType: "password",
			User:     "admin",
			Password: "password",
		}},
		Subs: []Subscription{{Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Backends[0].User != "admin" {
		t.Fatalf("user = %q", cfg.Backends[0].User)
	}
	if cfg.Backends[0].Password != "password" {
		t.Fatal("password was not preserved")
	}
}

func TestValidateBackendPasswordAuthFromEnv(t *testing.T) {
	t.Setenv("OPENLIST_TEST_PASSWORD", "secret-password")
	cfg := Config{
		Backends: []Backend{{
			ID:          "b1",
			Server:      "https://example.com",
			AuthType:    "password",
			User:        "admin",
			PasswordEnv: "OPENLIST_TEST_PASSWORD",
		}},
		Subs: []Subscription{{Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Backends[0].Password != "secret-password" {
		t.Fatal("password_env was not resolved")
	}
}

func TestValidateMissingBackendAuthTypeDefaultsToAnonymous(t *testing.T) {
	cfg := Config{
		Backends: []Backend{{
			ID:     "b1",
			Server: "https://example.com",
			APIKey: "secret",
		}},
		Subs: []Subscription{{Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}}}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected anonymous auth to reject credential fields")
	}
}

func TestValidateRejectsBackendAuthTypeCredentialConflicts(t *testing.T) {
	tests := []struct {
		name    string
		backend Backend
	}{
		{
			name:    "api key with password fields",
			backend: Backend{ID: "b1", Server: "https://example.com", AuthType: "api_key", APIKey: "secret", User: "admin", Password: "password"},
		},
		{
			name:    "api key with password env",
			backend: Backend{ID: "b1", Server: "https://example.com", AuthType: "api_key", APIKey: "secret", Password: "password", PasswordEnv: "OPENLIST_TEST_PASSWORD"},
		},
		{
			name:    "password with api key",
			backend: Backend{ID: "b1", Server: "https://example.com", AuthType: "password", APIKey: "secret", User: "admin", Password: "password"},
		},
		{
			name:    "anonymous with api key",
			backend: Backend{ID: "b1", Server: "https://example.com", AuthType: "anonymous", APIKey: "secret"},
		},
		{
			name:    "unsupported auth type",
			backend: Backend{ID: "b1", Server: "https://example.com", AuthType: "token"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Backends: []Backend{tt.backend},
				Subs:     []Subscription{{Mounts: []Mount{{ID: "m1", Backend: "b1", Path: "/"}}}},
			}
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected credential conflict error")
			}
		})
	}
}
