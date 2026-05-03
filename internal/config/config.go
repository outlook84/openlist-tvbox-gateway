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
	ID             string `json:"id" yaml:"id"`
	Server         string `json:"server" yaml:"server"`
	AuthType       string `json:"auth_type" yaml:"auth_type"`
	APIKey         string `json:"api_key,omitempty" yaml:"api_key"`
	APIKeyAction   string `json:"api_key_action,omitempty" yaml:"-"`
	APIKeyEnv      string `json:"api_key_env,omitempty" yaml:"api_key_env"`
	User           string `json:"user,omitempty" yaml:"user"`
	Password       string `json:"password,omitempty" yaml:"password"`
	PasswordAction string `json:"password_action,omitempty" yaml:"-"`
	PasswordEnv    string `json:"password_env,omitempty" yaml:"password_env"`
	Version        string `json:"version" yaml:"version"`
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
	ID                   string  `json:"id" yaml:"id"`
	Path                 string  `json:"path" yaml:"path"`
	AccessCodeHash       string  `json:"access_code_hash,omitempty" yaml:"access_code_hash"`
	AccessCodeHashAction string  `json:"access_code_hash_action,omitempty" yaml:"-"`
	AccessCode           string  `json:"access_code,omitempty" yaml:"access_code"`
	SiteKey              string  `json:"site_key" yaml:"site_key"`
	SiteName             string  `json:"site_name" yaml:"site_name"`
	TVBox                TVBox   `json:"tvbox" yaml:"tvbox"`
	Lives                []Live  `json:"lives" yaml:"lives"`
	Mounts               []Mount `json:"mounts" yaml:"mounts"`
}

type Live struct {
	Name       string `json:"name" yaml:"name"`
	Type       int    `json:"type" yaml:"type"`
	URL        string `json:"url" yaml:"url"`
	PlayerType int    `json:"playerType,omitempty" yaml:"player_type"`
	EPG        string `json:"epg,omitempty" yaml:"epg"`
	Logo       string `json:"logo,omitempty" yaml:"logo"`
	UA         string `json:"ua,omitempty" yaml:"ua"`
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
	var validateErr error
	if IsJSONPath(path) {
		validateErr = cfg.ValidateEditable()
	} else {
		validateErr = cfg.Validate()
	}
	if validateErr != nil {
		return nil, validateErr
	}
	return &cfg, nil
}

func EnsureEditableJSON(path string) error {
	if !IsJSONPath(path) {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cfg := Config{
		Backends: []Backend{
			{
				ID:       "local",
				Server:   "http://127.0.0.1:5244",
				AuthType: "anonymous",
				Version:  "v3",
			},
		},
		Subs: []Subscription{},
	}
	if err := cfg.ValidateEditable(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func IsJSONPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".json")
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
	return c.validate(validateOptions{})
}

// ValidateEditable is for the JSON admin editor. Editable JSON stores secrets
// directly and intentionally rejects env-backed secrets; use YAML for env-only
// secret configuration.
func (c *Config) ValidateEditable() error {
	return c.validate(validateOptions{AllowEmptySubs: true, ReservedHTTPPrefixes: []string{"/admin"}, RejectEnvSecrets: true})
}

type validateOptions struct {
	AllowEmptySubs       bool
	ReservedHTTPPrefixes []string
	RejectEnvSecrets     bool
}

func (c *Config) validate(opts validateOptions) error {
	c.PublicBaseURL = strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/")
	if c.PublicBaseURL != "" {
		u, err := url.Parse(c.PublicBaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return CodedErrorf("config.public_base_url.invalid", nil, "public_base_url must be an absolute URL when set")
		}
	}
	c.TVBox = normalizeTVBox(c.TVBox)
	if !validID(c.TVBox.SiteKey) {
		return CodedErrorf("tvbox.site_key.invalid", nil, "tvbox.site_key must contain only letters, digits, underscore or dash")
	}
	if len(c.Backends) == 0 {
		return CodedErrorf("backend.required", nil, "at least one backend is required")
	}
	backendIDs := map[string]struct{}{}
	for i := range c.Backends {
		b := &c.Backends[i]
		if !validID(b.ID) {
			return CodedErrorf("backend.id.invalid", map[string]any{"index": i}, "backend[%d] id must contain only letters, digits, underscore or dash", i)
		}
		if _, ok := backendIDs[b.ID]; ok {
			return CodedErrorf("backend.id.duplicate", map[string]any{"backend_id": b.ID}, "duplicate backend id %q", b.ID)
		}
		backendIDs[b.ID] = struct{}{}
		u, err := url.Parse(b.Server)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return CodedErrorf("backend.server.invalid", map[string]any{"backend_id": b.ID}, "backend %q server must be an absolute URL", b.ID)
		}
		b.Server = strings.TrimRight(b.Server, "/")
		if b.Version == "" {
			b.Version = "v3"
		} else if b.Version != "v3" {
			return CodedErrorf("backend.version.invalid", map[string]any{"backend_id": b.ID, "version": b.Version}, "backend %q version must be v3", b.ID)
		}
		if opts.RejectEnvSecrets {
			b.APIKeyEnv = strings.TrimSpace(b.APIKeyEnv)
			b.PasswordEnv = strings.TrimSpace(b.PasswordEnv)
			if b.APIKeyEnv != "" || b.PasswordEnv != "" {
				return CodedErrorf("backend.env_secret.unsupported", map[string]any{"backend_id": b.ID}, "backend %q env-backed secrets are not supported in editable JSON config", b.ID)
			}
		}
		if err := normalizeBackendAuth(b); err != nil {
			return err
		}
	}
	if len(c.Subs) == 0 && !opts.AllowEmptySubs {
		return CodedErrorf("subscription.required", nil, "at least one sub is required")
	}
	subIDs := map[string]struct{}{}
	subPaths := map[string]struct{}{}
	subSiteKeys := map[string]struct{}{}
	for i := range c.Subs {
		sub := &c.Subs[i]
		if sub.ID == "" {
			sub.ID = "default"
		}
		if !validID(sub.ID) {
			return CodedErrorf("subscription.id.invalid", map[string]any{"index": i}, "subs[%d] id must contain only letters, digits, underscore or dash", i)
		}
		if _, ok := subIDs[sub.ID]; ok {
			return CodedErrorf("subscription.id.duplicate", map[string]any{"sub_id": sub.ID}, "duplicate sub id %q", sub.ID)
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
			return CodedErrorf("subscription.path.invalid", map[string]any{"sub_id": sub.ID}, "sub %q path: %v", sub.ID, err)
		}
		sub.Path = cleanPath
		if reserved, ok := reservedHTTPPrefix(sub.Path, opts.ReservedHTTPPrefixes); ok {
			return CodedErrorf("subscription.path.reserved", map[string]any{"sub_id": sub.ID, "path": sub.Path, "reserved": reserved}, "sub %q path %q conflicts with reserved path prefix %q", sub.ID, sub.Path, reserved)
		}
		if _, ok := subPaths[sub.Path]; ok {
			return CodedErrorf("subscription.path.duplicate", map[string]any{"path": sub.Path}, "duplicate sub path %q", sub.Path)
		}
		subPaths[sub.Path] = struct{}{}
		if strings.TrimSpace(sub.AccessCode) != "" {
			return CodedErrorf("subscription.access_code.plaintext_unsupported", map[string]any{"sub_id": sub.ID}, "sub %q access_code plaintext is not supported; use access_code_hash", sub.ID)
		}
		sub.AccessCodeHash = strings.TrimSpace(sub.AccessCodeHash)
		if sub.AccessCodeHash != "" {
			if err := auth.ValidateHash(sub.AccessCodeHash); err != nil {
				return CodedErrorf("subscription.access_code_hash.invalid", map[string]any{"sub_id": sub.ID}, "sub %q access_code_hash must be a valid bcrypt hash", sub.ID)
			}
		}
		explicitSubSiteKey := strings.TrimSpace(sub.SiteKey) != "" || strings.TrimSpace(sub.TVBox.SiteKey) != ""
		if strings.TrimSpace(sub.SiteKey) != "" {
			sub.TVBox.SiteKey = sub.SiteKey
		}
		if strings.TrimSpace(sub.SiteName) != "" {
			sub.TVBox.SiteName = sub.SiteName
		}
		sub.TVBox = mergeTVBox(c.TVBox, sub.TVBox)
		if !explicitSubSiteKey {
			sub.TVBox.SiteKey = defaultSubSiteKey(c.TVBox.SiteKey, sub.ID)
		}
		sub.SiteKey = sub.TVBox.SiteKey
		sub.SiteName = sub.TVBox.SiteName
		if !validID(sub.TVBox.SiteKey) {
			return CodedErrorf("subscription.site_key.invalid", map[string]any{"sub_id": sub.ID}, "sub %q site_key must contain only letters, digits, underscore or dash", sub.ID)
		}
		if _, ok := subSiteKeys[sub.TVBox.SiteKey]; ok {
			return CodedErrorf("subscription.site_key.duplicate", map[string]any{"site_key": sub.TVBox.SiteKey}, "duplicate sub site_key %q", sub.TVBox.SiteKey)
		}
		subSiteKeys[sub.TVBox.SiteKey] = struct{}{}
		if len(sub.Mounts) == 0 {
			return CodedErrorf("subscription.mount.required", map[string]any{"sub_id": sub.ID}, "sub %q requires at least one mount", sub.ID)
		}
		if err := validateLives(sub.ID, sub.Lives); err != nil {
			return err
		}
		if err := validateMounts(sub.ID, sub.Mounts, backendIDs); err != nil {
			return err
		}
	}
	return nil
}

func reservedHTTPPrefix(path string, prefixes []string) (string, bool) {
	for _, prefix := range prefixes {
		cleanPrefix, err := CleanHTTPPath(prefix)
		if err != nil {
			continue
		}
		if path == cleanPrefix || strings.HasPrefix(path, cleanPrefix+"/") {
			return cleanPrefix, true
		}
	}
	return "", false
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
			return CodedErrorf("backend.auth.credentials_for_anonymous", map[string]any{"backend_id": b.ID}, "backend %q anonymous auth must not set credential fields", b.ID)
		}
	case "api_key":
		if b.User != "" || b.Password != "" || b.PasswordEnv != "" {
			return CodedErrorf("backend.auth.api_key_password_conflict", map[string]any{"backend_id": b.ID}, "backend %q api_key auth must not set password auth fields", b.ID)
		}
		if b.APIKey != "" && b.APIKeyEnv != "" {
			return CodedErrorf("backend.secret.multiple_sources", map[string]any{"backend_id": b.ID, "secret": "api_key"}, "backend %q must set only one of api_key or api_key_env", b.ID)
		}
		if b.APIKeyEnv != "" {
			value, ok := os.LookupEnv(b.APIKeyEnv)
			if !ok {
				return CodedErrorf("backend.env_secret.missing", map[string]any{"backend_id": b.ID, "env": b.APIKeyEnv}, "backend %q api_key_env %q is not set", b.ID, b.APIKeyEnv)
			}
			b.APIKey = strings.TrimSpace(value)
			if b.APIKey == "" {
				return CodedErrorf("backend.env_secret.empty", map[string]any{"backend_id": b.ID, "env": b.APIKeyEnv}, "backend %q api_key_env %q is empty", b.ID, b.APIKeyEnv)
			}
		}
		if b.APIKey == "" {
			return CodedErrorf("backend.auth.api_key_required", map[string]any{"backend_id": b.ID}, "backend %q api_key auth requires api_key or api_key_env", b.ID)
		}
	case "password":
		if b.APIKey != "" || b.APIKeyEnv != "" {
			return CodedErrorf("backend.auth.password_api_key_conflict", map[string]any{"backend_id": b.ID}, "backend %q password auth must not set api_key or api_key_env", b.ID)
		}
		if b.User == "" {
			return CodedErrorf("backend.auth.user_required", map[string]any{"backend_id": b.ID}, "backend %q password auth requires user", b.ID)
		}
		if b.Password != "" && b.PasswordEnv != "" {
			return CodedErrorf("backend.secret.multiple_sources", map[string]any{"backend_id": b.ID, "secret": "password"}, "backend %q must set only one of password or password_env", b.ID)
		}
		if b.PasswordEnv != "" {
			value, ok := os.LookupEnv(b.PasswordEnv)
			if !ok {
				return CodedErrorf("backend.env_secret.missing", map[string]any{"backend_id": b.ID, "env": b.PasswordEnv}, "backend %q password_env %q is not set", b.ID, b.PasswordEnv)
			}
			b.Password = value
		}
		if b.Password == "" {
			return CodedErrorf("backend.auth.password_required", map[string]any{"backend_id": b.ID}, "backend %q password auth requires password or password_env", b.ID)
		}
	default:
		return CodedErrorf("backend.auth_type.invalid", map[string]any{"backend_id": b.ID, "auth_type": b.AuthType}, "backend %q auth_type must be one of anonymous, api_key or password", b.ID)
	}
	return nil
}

func validateLives(subID string, lives []Live) error {
	for i := range lives {
		live := &lives[i]
		live.Name = strings.TrimSpace(live.Name)
		live.URL = strings.TrimSpace(live.URL)
		live.EPG = strings.TrimSpace(live.EPG)
		live.Logo = strings.TrimSpace(live.Logo)
		live.UA = strings.TrimSpace(live.UA)
		if live.Name == "" {
			live.Name = "Live"
		}
		if live.URL == "" {
			return CodedErrorf("subscription.live.url_required", map[string]any{"sub_id": subID, "index": i}, "sub %q live[%d] url is required", subID, i)
		}
		if live.Type != 0 {
			return CodedErrorf("subscription.live.type_invalid", map[string]any{"sub_id": subID, "index": i}, "sub %q live[%d] type must be 0", subID, i)
		}
		u, err := url.Parse(live.URL)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return CodedErrorf("subscription.live.url_invalid", map[string]any{"sub_id": subID, "index": i}, "sub %q live[%d] url must be an absolute http(s) URL", subID, i)
		}
		if live.EPG != "" {
			if u, err := url.Parse(live.EPG); err != nil || u.Scheme == "" || u.Host == "" {
				return CodedErrorf("subscription.live.epg_invalid", map[string]any{"sub_id": subID, "index": i}, "sub %q live[%d] epg must be an absolute URL", subID, i)
			}
		}
		if live.Logo != "" {
			if u, err := url.Parse(live.Logo); err != nil || u.Scheme == "" || u.Host == "" {
				return CodedErrorf("subscription.live.logo_invalid", map[string]any{"sub_id": subID, "index": i}, "sub %q live[%d] logo must be an absolute URL", subID, i)
			}
		}
	}
	return nil
}

func validateMounts(subID string, mounts []Mount, backendIDs map[string]struct{}) error {
	mountIDs := map[string]struct{}{}
	for i := range mounts {
		m := &mounts[i]
		if !validID(m.ID) {
			return CodedErrorf("mount.id.invalid", map[string]any{"sub_id": subID, "index": i}, "sub %q mount[%d] id must contain only letters, digits, underscore or dash", subID, i)
		}
		if _, ok := mountIDs[m.ID]; ok {
			return CodedErrorf("mount.id.duplicate", map[string]any{"sub_id": subID, "mount_id": m.ID}, "sub %q duplicate mount id %q", subID, m.ID)
		}
		mountIDs[m.ID] = struct{}{}
		if _, ok := backendIDs[m.Backend]; !ok {
			return CodedErrorf("mount.backend.unknown", map[string]any{"sub_id": subID, "mount_id": m.ID, "backend_id": m.Backend}, "sub %q mount %q references unknown backend %q", subID, m.ID, m.Backend)
		}
		if m.Name == "" {
			m.Name = m.ID
		}
		clean, err := CleanMountRoot(m.Path)
		if err != nil {
			return CodedErrorf("mount.path.invalid", map[string]any{"sub_id": subID, "mount_id": m.ID}, "sub %q mount %q path: %v", subID, m.ID, err)
		}
		m.Path = clean
		headers, err := NormalizePlayHeaders(m.PlayHeaders)
		if err != nil {
			return CodedErrorf("mount.play_headers.invalid", map[string]any{"sub_id": subID, "mount_id": m.ID}, "sub %q mount %q play_headers: %v", subID, m.ID, err)
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

func defaultSubSiteKey(baseKey, subID string) string {
	if subID == "default" {
		return baseKey
	}
	return baseKey + "_" + subID
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
