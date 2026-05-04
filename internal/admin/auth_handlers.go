package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"openlist-tvbox/internal/auth"
)

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.hash != "" {
		writeAdminError(w, http.StatusConflict, "admin.already_initialized", "admin already initialized", nil)
		return
	}
	key := s.setupFailureKey(r)
	if s.authLimiter.Blocked(key) {
		s.logAuthFailure("admin setup throttled", r, "too_many_attempts")
		writeAdminError(w, http.StatusTooManyRequests, "auth.too_many_setup_attempts", "too many failed admin setup attempts", nil)
		return
	}
	var req struct {
		SetupCode  string `json:"setup_code"`
		AccessCode string `json:"access_code"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxConfigBodySize+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "request.invalid_json", "invalid setup json", map[string]any{"target": "setup"})
		return
	}
	if verifySetupCode(s.setupCode, req.SetupCode) != nil {
		if req.SetupCode != "" {
			s.authLimiter.RecordFailure(key)
			s.logAuthFailure("admin setup failed", r, "invalid_setup_code")
		}
		writeAdminError(w, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}
	hash, err := hashAdminCode(req.AccessCode)
	if err != nil {
		writeAdminErrorFromError(w, http.StatusBadRequest, err, "admin.access_code.invalid")
		return
	}
	secretPath := adminSecretPath(s.configPath)
	if err := os.WriteFile(secretPath, []byte(hash+"\n"), 0o600); err != nil {
		writeAdminError(w, http.StatusInternalServerError, "admin.setup_failed", "admin setup failed", nil)
		return
	}
	if s.setupCodePath != "" {
		if err := os.Remove(s.setupCodePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(secretPath)
			writeAdminError(w, http.StatusInternalServerError, "admin.setup_failed", "admin setup failed", nil)
			return
		}
	}
	s.hash = hash
	s.setupCode = ""
	s.setupCodePath = ""
	s.authLimiter.Clear(key)
	if err := s.issueSessionCookie(w, r); err != nil {
		writeAdminError(w, http.StatusInternalServerError, "admin.session_failed", "admin session failed", nil)
		return
	}
	if s.logger != nil {
		s.logger.Info("admin setup completed")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.adminHash() == "" {
		writeAdminError(w, http.StatusForbidden, "admin.setup_required", "admin setup required", nil)
		return
	}
	key := s.adminFailureKey(r)
	if s.authLimiter.Blocked(key) {
		s.logAuthFailure("admin login throttled", r, "too_many_attempts")
		writeAdminError(w, http.StatusTooManyRequests, "auth.too_many_login_attempts", "too many failed admin authentication attempts", nil)
		return
	}
	var req struct {
		AccessCode string `json:"access_code"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxConfigBodySize+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "request.invalid_json", "invalid login json", map[string]any{"target": "login"})
		return
	}
	if verifyAdminCode(s.adminHash(), req.AccessCode) != nil {
		if req.AccessCode != "" {
			s.authLimiter.RecordFailure(key)
			s.logAuthFailure("admin login failed", r, "invalid_access_code")
		}
		writeAdminError(w, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}
	s.authLimiter.Clear(key)
	if err := s.issueSessionCookie(w, r); err != nil {
		writeAdminError(w, http.StatusInternalServerError, "admin.session_failed", "admin session failed", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessionMu.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionMu.Unlock()
	}
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) updateAdminAccessCode(w http.ResponseWriter, r *http.Request) {
	key := s.adminFailureKey(r)
	if s.authLimiter.Blocked(key) {
		s.logAuthFailure("admin access code update throttled", r, "too_many_attempts")
		writeAdminError(w, http.StatusTooManyRequests, "auth.too_many_login_attempts", "too many failed admin authentication attempts", nil)
		return
	}
	var req struct {
		CurrentAccessCode string `json:"current_access_code"`
		NewAccessCode     string `json:"new_access_code"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxConfigBodySize+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, "request.invalid_json", "invalid access code json", map[string]any{"target": "access_code"})
		return
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if verifyAdminCode(s.hash, req.CurrentAccessCode) != nil {
		if req.CurrentAccessCode != "" {
			s.authLimiter.RecordFailure(key)
			s.logAuthFailure("admin access code update failed", r, "invalid_current_code")
		}
		writeAdminError(w, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
		return
	}
	hash, err := hashAdminCode(req.NewAccessCode)
	if err != nil {
		writeAdminErrorFromError(w, http.StatusBadRequest, err, "admin.access_code.invalid")
		return
	}
	if err := os.WriteFile(adminSecretPath(s.configPath), []byte(hash+"\n"), 0o600); err != nil {
		writeAdminError(w, http.StatusInternalServerError, "admin.access_code.update_failed", "admin access code update failed", nil)
		return
	}
	s.hash = hash
	s.authLimiter.Clear(key)
	if s.logger != nil {
		s.logger.Info("admin access code updated", "client", auth.ClientHost(r, s.trustXForwardedFor()))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if s.adminHash() == "" {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false, "setup_required": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": s.validSession(r), "setup_required": false})
}
