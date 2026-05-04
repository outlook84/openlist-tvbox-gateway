package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/config"
)

func (s *Server) decodeEditableConfig(r *http.Request) (*config.Config, error) {
	var editable editableConfig
	dec := json.NewDecoder(io.LimitReader(r.Body, maxConfigBodySize+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&editable); err != nil {
		return nil, errors.New("invalid config json")
	}
	cfg := editable.Config()
	return &cfg, nil
}

func applyBackendSecretActions(next *config.Config, current *config.Config) error {
	currentByID := map[string]config.Backend{}
	for _, b := range current.Backends {
		currentByID[b.ID] = b
	}
	for i := range next.Backends {
		b := &next.Backends[i]
		actions := backendSecretActions(*b)
		currentBackend, hasCurrent := currentByID[b.ID]
		switch b.AuthType {
		case "api_key":
			action := defaultAction(actions["api_key"])
			if err := applySecretAction(action, &b.APIKey, currentBackend.APIKey, hasCurrent); err != nil {
				return codedf(errorCode(err, "secret.invalid_action"), map[string]any{"backend_id": b.ID, "secret": "api_key"}, "backend %q api_key_action: %v", b.ID, err)
			}
		case "password":
			action := defaultAction(actions["password"])
			if err := applySecretAction(action, &b.Password, currentBackend.Password, hasCurrent); err != nil {
				return codedf(errorCode(err, "secret.invalid_action"), map[string]any{"backend_id": b.ID, "secret": "password"}, "backend %q password_action: %v", b.ID, err)
			}
		default:
			b.APIKey = ""
			b.Password = ""
		}
		b.APIKeyAction = ""
		b.PasswordAction = ""
	}
	return nil
}

func applySubSecretActions(next *config.Config, current *config.Config) error {
	currentByID := map[string]config.Subscription{}
	for _, sub := range current.Subs {
		currentByID[sub.ID] = sub
	}
	for i := range next.Subs {
		sub := &next.Subs[i]
		currentSub, hasCurrent := currentByID[sub.ID]
		action := defaultAction(sub.AccessCodeHashAction)
		if strings.TrimSpace(sub.AccessCode) != "" {
			hash, err := auth.HashPassword(strings.TrimSpace(sub.AccessCode))
			if err != nil {
				return codedf("subscription.access_code.invalid", map[string]any{"sub_id": sub.ID, "min": 4, "max": 12}, "sub %q access_code: %v", sub.ID, err)
			}
			sub.AccessCodeHash = hash
			action = "replace"
		}
		if err := applySecretAction(action, &sub.AccessCodeHash, currentSub.AccessCodeHash, hasCurrent); err != nil {
			return codedf(errorCode(err, "secret.invalid_action"), map[string]any{"sub_id": sub.ID, "secret": "access_code"}, "sub %q access_code_hash_action: %v", sub.ID, err)
		}
		sub.AccessCode = ""
		sub.AccessCodeHashAction = ""
	}
	return nil
}

func backendSecretActions(b config.Backend) map[string]string {
	return map[string]string{"api_key": b.APIKeyAction, "password": b.PasswordAction}
}

func defaultAction(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return "replace"
	}
	return action
}

func applySecretAction(action string, target *string, current string, hasCurrent bool) error {
	switch action {
	case "keep":
		if !hasCurrent {
			return newCodedError("secret.keep_missing", "cannot keep missing existing secret", nil)
		}
		*target = current
	case "replace":
	case "clear":
		*target = ""
	default:
		return newCodedError("secret.invalid_action", "must be keep, replace or clear", map[string]any{"action": action})
	}
	return nil
}

func saveJSONConfigAtomic(path string, cfg *config.Config) error {
	validateCfg := cloneConfig(*cfg)
	if err := validateCfg.ValidateEditable(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	tmpData, err := os.ReadFile(tmpPath)
	if err != nil {
		return err
	}
	var reloaded config.Config
	if err := json.Unmarshal(tmpData, &reloaded); err != nil {
		return err
	}
	if err := reloaded.ValidateEditable(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		backupPath := path + ".bak"
		tmpBackup, err := os.CreateTemp(dir, filepath.Base(backupPath)+".tmp.")
		if err != nil {
			return err
		}
		tmpBackupPath := tmpBackup.Name()
		if err := tmpBackup.Close(); err != nil {
			_ = os.Remove(tmpBackupPath)
			return err
		}
		backupCleanup := true
		defer func() {
			if backupCleanup {
				_ = os.Remove(tmpBackupPath)
			}
		}()
		if err := copyFile(path, tmpBackupPath); err != nil {
			return err
		}
		if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(tmpBackupPath, backupPath); err != nil {
			return err
		}
		backupCleanup = false
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func cloneConfig(cfg config.Config) config.Config {
	out := cfg
	out.Backends = append([]config.Backend(nil), cfg.Backends...)
	out.Subs = make([]config.Subscription, len(cfg.Subs))
	for i, sub := range cfg.Subs {
		out.Subs[i] = cloneSubscription(sub)
	}
	return out
}

func cloneSubscription(sub config.Subscription) config.Subscription {
	out := sub
	out.Lives = append([]config.Live(nil), sub.Lives...)
	out.Mounts = make([]config.Mount, len(sub.Mounts))
	for i, mount := range sub.Mounts {
		out.Mounts[i] = mount
		if mount.Params != nil {
			out.Mounts[i].Params = cloneStringMap(mount.Params)
		}
		if mount.PlayHeaders != nil {
			out.Mounts[i].PlayHeaders = cloneStringMap(mount.PlayHeaders)
		}
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

type editableConfig struct {
	PublicBaseURL         string                 `json:"public_base_url"`
	TrustForwardedHeaders bool                   `json:"trust_forwarded_headers"`
	TrustXForwardedFor    bool                   `json:"trust_x_forwarded_for,omitempty"`
	TVBox                 config.TVBox           `json:"tvbox"`
	Backends              []editableBackend      `json:"backends"`
	Subs                  []editableSubscription `json:"subs"`
}

type editableBackend struct {
	ID             string `json:"id"`
	Server         string `json:"server"`
	AuthType       string `json:"auth_type"`
	APIKey         string `json:"api_key,omitempty"`
	APIKeyAction   string `json:"api_key_action,omitempty"`
	User           string `json:"user,omitempty"`
	Password       string `json:"password,omitempty"`
	PasswordAction string `json:"password_action,omitempty"`
	Version        string `json:"version"`
	APIKeySet      bool   `json:"api_key_set"`
	PasswordSet    bool   `json:"password_set"`
}

type editableSubscription struct {
	ID                   string         `json:"id"`
	Path                 string         `json:"path,omitempty"`
	AccessCodeHash       string         `json:"access_code_hash,omitempty"`
	AccessCodeHashAction string         `json:"access_code_hash_action,omitempty"`
	AccessCodeHashSet    bool           `json:"access_code_hash_set"`
	AccessCode           string         `json:"access_code,omitempty"`
	SiteKey              string         `json:"site_key,omitempty"`
	SiteName             string         `json:"site_name,omitempty"`
	TVBox                config.TVBox   `json:"tvbox,omitempty"`
	Lives                []config.Live  `json:"lives,omitempty"`
	Mounts               []config.Mount `json:"mounts,omitempty"`
}

func (c editableConfig) Config() config.Config {
	cfg := config.Config{
		PublicBaseURL:         c.PublicBaseURL,
		TrustForwardedHeaders: c.TrustForwardedHeaders || c.TrustXForwardedFor,
		TVBox:                 c.TVBox,
		Backends:              make([]config.Backend, 0, len(c.Backends)),
		Subs:                  make([]config.Subscription, 0, len(c.Subs)),
	}
	for _, b := range c.Backends {
		cfg.Backends = append(cfg.Backends, config.Backend{
			ID:             b.ID,
			Server:         b.Server,
			AuthType:       b.AuthType,
			APIKey:         b.APIKey,
			APIKeyAction:   b.APIKeyAction,
			User:           b.User,
			Password:       b.Password,
			PasswordAction: b.PasswordAction,
			Version:        b.Version,
		})
	}
	for _, sub := range c.Subs {
		cfg.Subs = append(cfg.Subs, config.Subscription{
			ID:                   sub.ID,
			Path:                 sub.Path,
			AccessCodeHash:       sub.AccessCodeHash,
			AccessCodeHashAction: sub.AccessCodeHashAction,
			AccessCode:           sub.AccessCode,
			SiteKey:              sub.SiteKey,
			SiteName:             sub.SiteName,
			TVBox:                sub.TVBox,
			Lives:                sub.Lives,
			Mounts:               sub.Mounts,
		})
	}
	return cfg
}

type redactedBackend struct {
	ID             string `json:"id"`
	Server         string `json:"server"`
	AuthType       string `json:"auth_type"`
	User           string `json:"user,omitempty"`
	Version        string `json:"version"`
	APIKeySet      bool   `json:"api_key_set"`
	PasswordSet    bool   `json:"password_set"`
	APIKeyAction   string `json:"api_key_action"`
	PasswordAction string `json:"password_action"`
}

type redactedSubscription struct {
	ID                   string         `json:"id"`
	Path                 string         `json:"path,omitempty"`
	AccessCodeHashSet    bool           `json:"access_code_hash_set"`
	AccessCodeHashAction string         `json:"access_code_hash_action"`
	SiteKey              string         `json:"site_key,omitempty"`
	SiteName             string         `json:"site_name,omitempty"`
	TVBox                config.TVBox   `json:"tvbox,omitempty"`
	Lives                []config.Live  `json:"lives,omitempty"`
	Mounts               []config.Mount `json:"mounts,omitempty"`
}

func redactedConfig(cfg config.Config) map[string]any {
	backends := make([]redactedBackend, 0, len(cfg.Backends))
	for _, b := range cfg.Backends {
		item := redactedBackend{
			ID:             b.ID,
			Server:         b.Server,
			AuthType:       b.AuthType,
			User:           b.User,
			Version:        b.Version,
			APIKeySet:      b.APIKey != "",
			PasswordSet:    b.Password != "",
			APIKeyAction:   "keep",
			PasswordAction: "keep",
		}
		backends = append(backends, item)
	}
	subs := make([]redactedSubscription, 0, len(cfg.Subs))
	for _, sub := range cfg.Subs {
		subs = append(subs, redactedSubscription{
			ID:                   sub.ID,
			Path:                 sub.Path,
			AccessCodeHashSet:    sub.AccessCodeHash != "",
			AccessCodeHashAction: "keep",
			SiteKey:              sub.SiteKey,
			SiteName:             sub.SiteName,
			TVBox:                sub.TVBox,
			Lives:                sub.Lives,
			Mounts:               sub.Mounts,
		})
	}
	return map[string]any{
		"public_base_url":         cfg.PublicBaseURL,
		"trust_forwarded_headers": cfg.TrustForwardedHeaders,
		"tvbox":                   cfg.TVBox,
		"backends":                backends,
		"subs":                    subs,
	}
}
