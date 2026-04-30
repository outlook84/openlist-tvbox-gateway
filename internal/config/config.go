package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"openlist-tvbox/internal/auth"
)

type Config struct {
	PublicBaseURL      string         `json:"public_base_url" yaml:"public_base_url"`
	TrustXForwardedFor bool           `json:"trust_x_forwarded_for" yaml:"trust_x_forwarded_for"`
	TVBox              TVBox          `json:"tvbox" yaml:"tvbox"`
	Backends           []Backend      `json:"backends" yaml:"backends"`
	Subs               []Subscription `json:"subs" yaml:"subs"`
}

type TVBox struct {
	SiteKey     string `json:"site_key" yaml:"site_key"`
	SiteName    string `json:"site_name" yaml:"site_name"`
	Timeout     int    `json:"timeout" yaml:"timeout"`
	Searchable  *int   `json:"searchable" yaml:"searchable"`
	QuickSearch *int   `json:"quick_search" yaml:"quick_search"`
	Changeable  *int   `json:"changeable" yaml:"changeable"`
}

type Backend struct {
	ID          string `json:"id" yaml:"id"`
	Server      string `json:"server" yaml:"server"`
	AuthType    string `json:"auth_type" yaml:"auth_type"`
	APIKey      string `json:"api_key" yaml:"api_key"`
	APIKeyEnv   string `json:"api_key_env" yaml:"api_key_env"`
	User        string `json:"user" yaml:"user"`
	Password    string `json:"password" yaml:"password"`
	PasswordEnv string `json:"password_env" yaml:"password_env"`
	Version     string `json:"version" yaml:"version"`
}

type Mount struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	Backend     string            `json:"backend" yaml:"backend"`
	Path        string            `json:"path" yaml:"path"`
	Params      map[string]string `json:"params" yaml:"params"`
	PlayHeaders map[string]string `json:"play_headers" yaml:"play_headers"`
	Search      *bool             `json:"search" yaml:"search"`
	Refresh     bool              `json:"refresh" yaml:"refresh"`
	Hidden      bool              `json:"hidden" yaml:"hidden"`
}

type Subscription struct {
	ID             string  `json:"id" yaml:"id"`
	Path           string  `json:"path" yaml:"path"`
	AccessCodeHash string  `json:"access_code_hash" yaml:"access_code_hash"`
	AccessCode     string  `json:"access_code" yaml:"access_code"`
	SiteKey        string  `json:"site_key" yaml:"site_key"`
	SiteName       string  `json:"site_name" yaml:"site_name"`
	TVBox          TVBox   `json:"tvbox" yaml:"tvbox"`
	Mounts         []Mount `json:"mounts" yaml:"mounts"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := unmarshalConfig(path, data, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func unmarshalConfig(path string, data []byte, cfg *Config) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return yaml.Unmarshal(data, cfg)
	default:
		return json.Unmarshal(data, cfg)
	}
}

func (c *Config) Validate() error {
	c.PublicBaseURL = strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/")
	if c.PublicBaseURL != "" {
		u, err := url.Parse(c.PublicBaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("public_base_url must be an absolute URL when set")
		}
	}
	c.TVBox = normalizeTVBox(c.TVBox)
	if !validID(c.TVBox.SiteKey) {
		return errors.New("tvbox.site_key must contain only letters, digits, underscore or dash")
	}
	if len(c.Backends) == 0 {
		return errors.New("at least one backend is required")
	}
	backendIDs := map[string]struct{}{}
	for i := range c.Backends {
		b := &c.Backends[i]
		if !validID(b.ID) {
			return fmt.Errorf("backend[%d] id must contain only letters, digits, underscore or dash", i)
		}
		if _, ok := backendIDs[b.ID]; ok {
			return fmt.Errorf("duplicate backend id %q", b.ID)
		}
		backendIDs[b.ID] = struct{}{}
		u, err := url.Parse(b.Server)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("backend %q server must be an absolute URL", b.ID)
		}
		b.Server = strings.TrimRight(b.Server, "/")
		if b.Version == "" {
			b.Version = "v3"
		} else if b.Version != "v3" {
			return fmt.Errorf("backend %q version must be v3", b.ID)
		}
		if err := normalizeBackendAuth(b); err != nil {
			return err
		}
	}
	if len(c.Subs) == 0 {
		return errors.New("at least one sub is required")
	}
	subIDs := map[string]struct{}{}
	subPaths := map[string]struct{}{}
	for i := range c.Subs {
		sub := &c.Subs[i]
		if sub.ID == "" {
			sub.ID = "default"
		}
		if !validID(sub.ID) {
			return fmt.Errorf("subs[%d] id must contain only letters, digits, underscore or dash", i)
		}
		if _, ok := subIDs[sub.ID]; ok {
			return fmt.Errorf("duplicate sub id %q", sub.ID)
		}
		subIDs[sub.ID] = struct{}{}
		if sub.Path == "" {
			if sub.ID == "default" {
				sub.Path = "/sub"
			} else {
				sub.Path = "/sub/" + sub.ID
			}
		}
		cleanPath, err := CleanHTTPPath(sub.Path)
		if err != nil {
			return fmt.Errorf("sub %q path: %w", sub.ID, err)
		}
		sub.Path = cleanPath
		if _, ok := subPaths[sub.Path]; ok {
			return fmt.Errorf("duplicate sub path %q", sub.Path)
		}
		subPaths[sub.Path] = struct{}{}
		if strings.TrimSpace(sub.AccessCode) != "" {
			return fmt.Errorf("sub %q access_code plaintext is not supported; use access_code_hash", sub.ID)
		}
		sub.AccessCodeHash = strings.TrimSpace(sub.AccessCodeHash)
		if sub.AccessCodeHash != "" {
			if err := auth.ValidateHash(sub.AccessCodeHash); err != nil {
				return fmt.Errorf("sub %q access_code_hash must be a valid bcrypt hash", sub.ID)
			}
		}
		sub.TVBox.SiteKey = sub.SiteKey
		sub.TVBox.SiteName = sub.SiteName
		sub.TVBox = mergeTVBox(c.TVBox, sub.TVBox)
		sub.SiteKey = sub.TVBox.SiteKey
		sub.SiteName = sub.TVBox.SiteName
		if !validID(sub.TVBox.SiteKey) {
			return fmt.Errorf("sub %q site_key must contain only letters, digits, underscore or dash", sub.ID)
		}
		if len(sub.Mounts) == 0 {
			return fmt.Errorf("sub %q requires at least one mount", sub.ID)
		}
		if err := validateMounts(sub.ID, sub.Mounts, backendIDs); err != nil {
			return err
		}
	}
	return nil
}

func normalizeBackendAuth(b *Backend) error {
	b.AuthType = strings.TrimSpace(b.AuthType)
	if b.AuthType == "" {
		b.AuthType = "anonymous"
	}
	b.APIKey = strings.TrimSpace(b.APIKey)
	b.APIKeyEnv = strings.TrimSpace(b.APIKeyEnv)
	b.User = strings.TrimSpace(b.User)
	b.PasswordEnv = strings.TrimSpace(b.PasswordEnv)

	switch b.AuthType {
	case "anonymous":
		if b.APIKey != "" || b.APIKeyEnv != "" || b.User != "" || b.Password != "" || b.PasswordEnv != "" {
			return fmt.Errorf("backend %q anonymous auth must not set credential fields", b.ID)
		}
	case "api_key":
		if b.User != "" || b.Password != "" || b.PasswordEnv != "" {
			return fmt.Errorf("backend %q api_key auth must not set password auth fields", b.ID)
		}
		if b.APIKey != "" && b.APIKeyEnv != "" {
			return fmt.Errorf("backend %q must set only one of api_key or api_key_env", b.ID)
		}
		if b.APIKeyEnv != "" {
			value, ok := os.LookupEnv(b.APIKeyEnv)
			if !ok {
				return fmt.Errorf("backend %q api_key_env %q is not set", b.ID, b.APIKeyEnv)
			}
			b.APIKey = strings.TrimSpace(value)
			if b.APIKey == "" {
				return fmt.Errorf("backend %q api_key_env %q is empty", b.ID, b.APIKeyEnv)
			}
		}
		if b.APIKey == "" {
			return fmt.Errorf("backend %q api_key auth requires api_key or api_key_env", b.ID)
		}
	case "password":
		if b.APIKey != "" || b.APIKeyEnv != "" {
			return fmt.Errorf("backend %q password auth must not set api_key or api_key_env", b.ID)
		}
		if b.User == "" {
			return fmt.Errorf("backend %q password auth requires user", b.ID)
		}
		if b.Password != "" && b.PasswordEnv != "" {
			return fmt.Errorf("backend %q must set only one of password or password_env", b.ID)
		}
		if b.PasswordEnv != "" {
			value, ok := os.LookupEnv(b.PasswordEnv)
			if !ok {
				return fmt.Errorf("backend %q password_env %q is not set", b.ID, b.PasswordEnv)
			}
			b.Password = value
		}
		if b.Password == "" {
			return fmt.Errorf("backend %q password auth requires password or password_env", b.ID)
		}
	default:
		return fmt.Errorf("backend %q auth_type must be one of anonymous, api_key or password", b.ID)
	}
	return nil
}

func validateMounts(subID string, mounts []Mount, backendIDs map[string]struct{}) error {
	mountIDs := map[string]struct{}{}
	for i := range mounts {
		m := &mounts[i]
		if !validID(m.ID) {
			return fmt.Errorf("sub %q mount[%d] id must contain only letters, digits, underscore or dash", subID, i)
		}
		if _, ok := mountIDs[m.ID]; ok {
			return fmt.Errorf("sub %q duplicate mount id %q", subID, m.ID)
		}
		mountIDs[m.ID] = struct{}{}
		if _, ok := backendIDs[m.Backend]; !ok {
			return fmt.Errorf("sub %q mount %q references unknown backend %q", subID, m.ID, m.Backend)
		}
		if m.Name == "" {
			m.Name = m.ID
		}
		clean, err := CleanMountRoot(m.Path)
		if err != nil {
			return fmt.Errorf("sub %q mount %q path: %w", subID, m.ID, err)
		}
		m.Path = clean
		headers, err := NormalizePlayHeaders(m.PlayHeaders)
		if err != nil {
			return fmt.Errorf("sub %q mount %q play_headers: %w", subID, m.ID, err)
		}
		m.PlayHeaders = headers
	}
	return nil
}

func normalizeTVBox(tv TVBox) TVBox {
	if tv.SiteKey == "" {
		tv.SiteKey = "openlist_tvbox"
	}
	if tv.SiteName == "" {
		tv.SiteName = "OpenList"
	}
	if tv.Timeout <= 0 {
		tv.Timeout = 15
	}
	return tv
}

func mergeTVBox(base, override TVBox) TVBox {
	if override.SiteKey == "" {
		override.SiteKey = base.SiteKey
	}
	if override.SiteName == "" {
		override.SiteName = base.SiteName
	}
	if override.Timeout <= 0 {
		override.Timeout = base.Timeout
	}
	if override.Searchable == nil {
		override.Searchable = base.Searchable
	}
	if override.QuickSearch == nil {
		override.QuickSearch = base.QuickSearch
	}
	if override.Changeable == nil {
		override.Changeable = base.Changeable
	}
	return normalizeTVBox(override)
}

func CleanMountRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	if strings.Contains(path, "\\") || strings.Contains(path, "\x00") {
		return "", errors.New("invalid path")
	}
	if strings.Contains(path, "..") {
		return "", errors.New("path traversal is not allowed")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path, nil
}

func CleanHTTPPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("empty path")
	}
	if strings.Contains(path, "\\") || strings.Contains(path, "\x00") || strings.Contains(path, "?") || strings.Contains(path, "#") {
		return "", errors.New("invalid path")
	}
	if strings.Contains(path, "..") {
		return "", errors.New("path traversal is not allowed")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if path == "/" {
		return path, nil
	}
	return strings.TrimRight(path, "/"), nil
}

func (m Mount) SearchEnabled() bool {
	return m.Search == nil || *m.Search
}

func NormalizePlayHeaders(headers map[string]string) (map[string]string, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		name := http.CanonicalHeaderKey(strings.TrimSpace(key))
		if name == "" || !validHeaderName(name) {
			return nil, fmt.Errorf("invalid header name %q", key)
		}
		if sensitiveHeader(name) {
			return nil, fmt.Errorf("sensitive header %q is not allowed", name)
		}
		if _, ok := out[name]; ok {
			return nil, fmt.Errorf("duplicate header %q", name)
		}
		value = strings.TrimSpace(value)
		if strings.ContainsAny(value, "\r\n\x00") {
			return nil, fmt.Errorf("invalid value for header %q", name)
		}
		out[name] = value
	}
	return out, nil
}

func sensitiveHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "api-key", "token", "x-token":
		return true
	default:
		return false
	}
}

func validHeaderName(name string) bool {
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		}
		return false
	}
	return true
}

func validID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
