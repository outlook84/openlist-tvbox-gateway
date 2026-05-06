package admin

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) requireSameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.crossOriginProtection(r).Check(r); err != nil || !s.originFallbackAllows(r) || !s.refererFallbackAllows(r) {
			writeAdminError(w, http.StatusForbidden, "request.cross_origin", "cross-origin admin request rejected", nil)
			return
		}
		next(w, r)
	}
}

func (s *Server) originFallbackAllows(r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")) != "" {
		return true
	}
	origin, ok := originOnly(r.Header.Get("Origin"))
	if !ok {
		return true
	}
	for _, baseURL := range s.adminOriginBaseURLs(r) {
		want, ok := originOnly(baseURL)
		if ok && strings.EqualFold(origin, want) {
			return true
		}
	}
	return false
}

func (s *Server) refererFallbackAllows(r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")) != "" || strings.TrimSpace(r.Header.Get("Origin")) != "" {
		return true
	}
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer == "" {
		return true
	}
	origin, ok := originOnly(referer)
	if !ok {
		return false
	}
	for _, baseURL := range s.adminOriginBaseURLs(r) {
		want, ok := originOnly(baseURL)
		if ok && strings.EqualFold(origin, want) {
			return true
		}
	}
	return false
}

func (s *Server) crossOriginProtection(r *http.Request) *http.CrossOriginProtection {
	cop := http.NewCrossOriginProtection()
	for _, baseURL := range s.adminOriginBaseURLs(r) {
		origin, ok := originOnly(baseURL)
		if !ok {
			continue
		}
		_ = cop.AddTrustedOrigin(origin)
	}
	return cop
}

func originOnly(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminHash() == "" {
			writeAdminError(w, http.StatusForbidden, "admin.setup_required", "admin setup required", nil)
			return
		}
		if !s.validSession(r) {
			writeAdminError(w, http.StatusUnauthorized, "auth.unauthorized", "unauthorized", nil)
			return
		}
		next(w, r)
	}
}

func (s *Server) adminHash() string {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	return s.hash
}
