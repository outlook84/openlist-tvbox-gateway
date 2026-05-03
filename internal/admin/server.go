package admin

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/config"
)

const (
	envAdminCode      = "OPENLIST_TVBOX_ADMIN_ACCESS_CODE"
	envAdminCodeHash  = "OPENLIST_TVBOX_ADMIN_ACCESS_CODE_HASH"
	secretFileName    = "admin_access_code_hash"
	setupCodeFileName = "admin_setup_code"
	maxConfigBodySize = 1 << 20
)

type Server struct {
	configPath    string
	logger        *slog.Logger
	hash          string
	setupCodePath string
	setupCode     string
	onSaved       func(*config.Config)
	saveMu        sync.Mutex
	setupMu       sync.Mutex
	authLimiter   *auth.FailureLimiter
	trustMu       sync.RWMutex
	trustXFF      bool
}

type Options struct {
	ConfigPath string
	Listen     string
	Logger     *slog.Logger
	OnSaved    func(*config.Config)
}

func NewServer(opts Options) (*Server, error) {
	state, err := loadAdminState(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	trustXFF := false
	if cfg, err := config.Load(opts.ConfigPath); err == nil {
		trustXFF = cfg.TrustXForwardedFor
	}
	if opts.Logger != nil {
		if state.Hash != "" {
			opts.Logger.Info("admin access code loaded")
		} else {
			opts.Logger.Warn("admin setup required; use the setup code file to create an access code", "admin_url", adminURL(opts.Listen), "setup_code_path", state.SetupCodePath)
		}
	}
	return &Server{
		configPath:    opts.ConfigPath,
		logger:        opts.Logger,
		hash:          state.Hash,
		setupCodePath: state.SetupCodePath,
		setupCode:     state.SetupCode,
		onSaved:       opts.OnSaved,
		authLimiter:   auth.NewFailureLimiter(auth.DefaultFailureLimit, auth.DefaultFailureCooldown),
		trustXFF:      trustXFF,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/setup", s.setup)
	mux.HandleFunc("GET /admin/config/meta", s.requireAuth(s.meta))
	mux.HandleFunc("GET /admin/config", s.requireAuth(s.getConfig))
	mux.HandleFunc("POST /admin/config/validate", s.requireAuth(s.validateConfig))
	mux.HandleFunc("PUT /admin/config", s.requireAuth(s.putConfig))
	mux.HandleFunc("/admin/", s.requireAuth(s.notFound))
	return mux
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminHash() == "" {
			writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "admin setup required"})
			return
		}
		code := adminCodeFromRequest(r)
		key := s.adminFailureKey(r)
		if s.authLimiter.Blocked(key) {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "too many failed admin authentication attempts"})
			return
		}
		if code == "" || verifyAdminCode(s.adminHash(), code) != nil {
			if code != "" {
				s.authLimiter.RecordFailure(key)
			}
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		s.authLimiter.Clear(key)
		next(w, r)
	}
}

func (s *Server) adminHash() string {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	return s.hash
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.hash != "" {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "admin already initialized"})
		return
	}
	key := s.setupFailureKey(r)
	if s.authLimiter.Blocked(key) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "too many failed admin setup attempts"})
		return
	}
	var req struct {
		SetupCode  string `json:"setup_code"`
		AccessCode string `json:"access_code"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxConfigBodySize+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid setup json"})
		return
	}
	if verifySetupCode(s.setupCode, req.SetupCode) != nil {
		if req.SetupCode != "" {
			s.authLimiter.RecordFailure(key)
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	hash, err := hashAdminCode(req.AccessCode)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	secretPath := adminSecretPath(s.configPath)
	if err := os.WriteFile(secretPath, []byte(hash+"\n"), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "admin setup failed"})
		return
	}
	if s.setupCodePath != "" {
		if err := os.Remove(s.setupCodePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(secretPath)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "admin setup failed"})
			return
		}
	}
	s.hash = hash
	s.setupCode = ""
	s.setupCodePath = ""
	s.authLimiter.Clear(key)
	if s.logger != nil {
		s.logger.Info("admin setup completed")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":     "editable",
		"format":   "json",
		"editable": true,
		"path":     s.configPath,
	})
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "config load failed"})
		return
	}
	writeJSON(w, http.StatusOK, redactedConfig(*cfg))
}

func (s *Server) validateConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.decodeEditableConfig(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	current, err := config.Load(s.configPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"valid": false, "error": "config load failed"})
		return
	}
	if err := applyBackendSecretActions(cfg, current); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	if err := applySubSecretActions(cfg, current); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	if err := cfg.ValidateEditable(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true})
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	next, err := s.decodeEditableConfig(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	current, err := config.Load(s.configPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "config load failed"})
		return
	}
	if err := applyBackendSecretActions(next, current); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := applySubSecretActions(next, current); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	validated := cloneConfig(*next)
	if err := validated.ValidateEditable(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := saveJSONConfigAtomic(s.configPath, next); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "config save failed"})
		return
	}
	if s.onSaved != nil {
		s.onSaved(&validated)
	} else {
		s.ApplyConfig(&validated)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "config saved"})
}

func (s *Server) ApplyConfig(cfg *config.Config) {
	s.setTrustXForwardedFor(cfg.TrustXForwardedFor)
}

func (s *Server) setTrustXForwardedFor(trust bool) {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	s.trustXFF = trust
}

func (s *Server) trustXForwardedFor() bool {
	s.trustMu.RLock()
	defer s.trustMu.RUnlock()
	return s.trustXFF
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

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
			currentValue := currentBackend.APIKey
			if currentBackend.APIKeyEnv != "" {
				currentValue = ""
			}
			if err := applySecretAction(action, &b.APIKey, currentValue, hasCurrent); err != nil {
				return fmt.Errorf("backend %q api_key_action: %w", b.ID, err)
			}
		case "password":
			action := defaultAction(actions["password"])
			currentValue := currentBackend.Password
			if currentBackend.PasswordEnv != "" {
				currentValue = ""
			}
			if err := applySecretAction(action, &b.Password, currentValue, hasCurrent); err != nil {
				return fmt.Errorf("backend %q password_action: %w", b.ID, err)
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
		if err := applySecretAction(action, &sub.AccessCodeHash, currentSub.AccessCodeHash, hasCurrent); err != nil {
			return fmt.Errorf("sub %q access_code_hash_action: %w", sub.ID, err)
		}
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
			return errors.New("cannot keep missing existing secret")
		}
		*target = current
	case "replace":
	case "clear":
		*target = ""
	default:
		return errors.New("must be keep, replace or clear")
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
	PublicBaseURL      string                 `json:"public_base_url"`
	TrustXForwardedFor bool                   `json:"trust_x_forwarded_for"`
	TVBox              config.TVBox           `json:"tvbox"`
	Backends           []editableBackend      `json:"backends"`
	Subs               []editableSubscription `json:"subs"`
}

type editableBackend struct {
	ID             string `json:"id"`
	Server         string `json:"server"`
	AuthType       string `json:"auth_type"`
	APIKey         string `json:"api_key,omitempty"`
	APIKeyAction   string `json:"api_key_action,omitempty"`
	APIKeyEnv      string `json:"api_key_env,omitempty"`
	User           string `json:"user,omitempty"`
	Password       string `json:"password,omitempty"`
	PasswordAction string `json:"password_action,omitempty"`
	PasswordEnv    string `json:"password_env,omitempty"`
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
		PublicBaseURL:      c.PublicBaseURL,
		TrustXForwardedFor: c.TrustXForwardedFor,
		TVBox:              c.TVBox,
		Backends:           make([]config.Backend, 0, len(c.Backends)),
		Subs:               make([]config.Subscription, 0, len(c.Subs)),
	}
	for _, b := range c.Backends {
		cfg.Backends = append(cfg.Backends, config.Backend{
			ID:             b.ID,
			Server:         b.Server,
			AuthType:       b.AuthType,
			APIKey:         b.APIKey,
			APIKeyAction:   b.APIKeyAction,
			APIKeyEnv:      b.APIKeyEnv,
			User:           b.User,
			Password:       b.Password,
			PasswordAction: b.PasswordAction,
			PasswordEnv:    b.PasswordEnv,
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
	APIKeyEnv      string `json:"api_key_env,omitempty"`
	User           string `json:"user,omitempty"`
	PasswordEnv    string `json:"password_env,omitempty"`
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
			APIKeyEnv:      b.APIKeyEnv,
			User:           b.User,
			PasswordEnv:    b.PasswordEnv,
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
		"public_base_url":       cfg.PublicBaseURL,
		"trust_x_forwarded_for": cfg.TrustXForwardedFor,
		"tvbox":                 cfg.TVBox,
		"backends":              backends,
		"subs":                  subs,
	}
}

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
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(code), auth.DefaultBcryptCost)
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

func adminCodeFromRequest(r *http.Request) string {
	if code := r.Header.Get("X-Admin-Code"); code != "" {
		return code
	}
	return ""
}

func (s *Server) adminFailureKey(r *http.Request) string {
	return "admin|" + auth.ClientHost(r, s.trustXForwardedFor())
}

func (s *Server) setupFailureKey(r *http.Request) string {
	return "setup|" + auth.ClientHost(r, s.trustXForwardedFor())
}

func adminURL(listen string) string {
	host := listen
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	return "http://" + host + "/admin"
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
