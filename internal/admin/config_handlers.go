package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/openlist"
)

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
		writeAdminError(w, http.StatusInternalServerError, "config.load_failed", "config load failed", nil)
		return
	}
	writeJSON(w, http.StatusOK, redactedConfig(*cfg))
}

func (s *Server) validateConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.decodeEditableConfig(r)
	if err != nil {
		writeValidationError(w, http.StatusBadRequest, err, "request.invalid_json")
		return
	}
	current, err := config.Load(s.configPath)
	if err != nil {
		writeValidationError(w, http.StatusInternalServerError, newCodedError("config.load_failed", "config load failed", nil), "config.load_failed")
		return
	}
	if err := applyBackendSecretActions(cfg, current); err != nil {
		writeValidationError(w, http.StatusOK, err, "secret.invalid_action")
		return
	}
	if err := applySubSecretActions(cfg, current); err != nil {
		writeValidationError(w, http.StatusOK, err, "subscription.access_code.invalid")
		return
	}
	if err := cfg.ValidateEditable(); err != nil {
		writeConfigValidationError(w, http.StatusOK, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true})
}

func (s *Server) testBackend(w http.ResponseWriter, r *http.Request) {
	var req editableBackend
	dec := json.NewDecoder(io.LimitReader(r.Body, maxConfigBodySize+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "request.invalid_json", "invalid backend test json", map[string]any{"target": "backend"})
		return
	}
	backend := config.Backend{
		ID:             req.ID,
		Server:         req.Server,
		AuthType:       req.AuthType,
		APIKey:         req.APIKey,
		APIKeyAction:   req.APIKeyAction,
		User:           req.User,
		Password:       req.Password,
		PasswordAction: req.PasswordAction,
		Version:        req.Version,
	}
	current, err := config.Load(s.configPath)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "config.load_failed", "config load failed", nil)
		return
	}
	testCfg := config.Config{Backends: []config.Backend{backend}, Subs: []config.Subscription{}}
	if err := applyBackendSecretActions(&testCfg, current); err != nil {
		writeAdminErrorFromError(w, http.StatusBadRequest, err, "secret.invalid_action")
		return
	}
	if err := testCfg.ValidateEditable(); err != nil {
		writeConfigAdminError(w, http.StatusBadRequest, err)
		return
	}
	client := openlist.NewClient(&http.Client{Timeout: 12 * time.Second}, s.logger)
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if _, err := client.List(ctx, testCfg.Backends[0], "/", ""); err != nil {
		if s.logger != nil {
			s.logger.Warn("admin backend test failed", "backend", testCfg.Backends[0].ID, "auth_type", testCfg.Backends[0].AuthType, "error_kind", backendTestErrorKind(err))
		}
		writeAdminError(w, http.StatusBadGateway, "backend.test_failed", "backend test failed", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "backend test passed"})
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	next, err := s.decodeEditableConfig(r)
	if err != nil {
		writeAdminErrorFromError(w, http.StatusBadRequest, err, "request.invalid_json")
		return
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	current, err := config.Load(s.configPath)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, "config.load_failed", "config load failed", nil)
		return
	}
	if err := applyBackendSecretActions(next, current); err != nil {
		writeAdminErrorFromError(w, http.StatusBadRequest, err, "secret.invalid_action")
		return
	}
	if err := applySubSecretActions(next, current); err != nil {
		writeAdminErrorFromError(w, http.StatusBadRequest, err, "subscription.access_code.invalid")
		return
	}
	validated := cloneConfig(*next)
	if err := validated.ValidateEditable(); err != nil {
		writeConfigAdminError(w, http.StatusBadRequest, err)
		return
	}
	if err := saveJSONConfigAtomic(s.configPath, next); err != nil {
		writeAdminError(w, http.StatusInternalServerError, "config.save_failed", "config save failed", nil)
		return
	}
	if s.onSaved != nil {
		s.onSaved(&validated)
	} else {
		s.ApplyConfig(&validated)
	}
	if s.logger != nil {
		s.logger.Info("admin config saved", "path", s.configPath, "backends", len(validated.Backends), "subs", len(validated.Subs))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "config saved"})
}

func (s *Server) ApplyConfig(cfg *config.Config) {
	s.setTrustForwardedHeaders(cfg.TrustForwardedHeaders)
	s.setPublicBaseURL(cfg.PublicBaseURL)
}

func (s *Server) setTrustForwardedHeaders(trust bool) {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	s.trustXFF = trust
}

func (s *Server) trustForwardedHeaders() bool {
	s.trustMu.RLock()
	defer s.trustMu.RUnlock()
	return s.trustXFF
}

func (s *Server) setPublicBaseURL(baseURL string) {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	s.publicBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func (s *Server) getPublicBaseURL() string {
	s.trustMu.RLock()
	defer s.trustMu.RUnlock()
	return s.publicBaseURL
}
