package admin

import (
	"net/http"
)

func (s *Server) requireSameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.sameOrigin(r) {
			writeAdminError(w, http.StatusForbidden, "request.cross_origin", "cross-origin admin request rejected", nil)
			return
		}
		next(w, r)
	}
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
