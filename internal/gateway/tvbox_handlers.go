package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/catvod"
	"openlist-tvbox/internal/mount"
)

func (s *Server) categoryForSub(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(service, w, r, subID) {
		return
	}
	q := r.URL.Query()
	result, err := service.CategoryForSub(r.Context(), subID, q.Get("tid"), q.Get("type"), q.Get("order"))
	s.writeResult(w, result, err, "category", subID)
}

func (s *Server) detailForSub(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(service, w, r, subID) {
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Query().Get("ids"), "[")
		id = strings.TrimSuffix(id, "]")
		id = strings.Trim(id, "\"")
	}
	result, err := service.DetailForSub(r.Context(), subID, id)
	s.writeResult(w, result, err, "detail", subID)
}

func (s *Server) searchForSub(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(service, w, r, subID) {
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		key = r.URL.Query().Get("wd")
	}
	result, err := service.SearchForSub(r.Context(), subID, key)
	s.writeResult(w, result, err, "search", subID)
}

func (s *Server) playForSub(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(service, w, r, subID) {
		return
	}
	result, err := service.PlayForSub(r.Context(), subID, r.URL.Query().Get("id"))
	s.writeResult(w, result, err, "play", subID)
}

func (s *Server) refreshForSub(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !s.authorize(service, w, r, subID) {
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		var body struct {
			ID string `json:"id"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
		}
		id = body.ID
	}
	result, err := service.RefreshForSub(r.Context(), subID, id)
	s.writeResult(w, result, err, "refresh", subID)
}
func (s *Server) writeResult(w http.ResponseWriter, result catvod.Result, err error, operation, subID string) {
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("tvbox api failed", "operation", operation, "sub", subID, "error_kind", tvboxErrorKind(err))
		}
		writeJSON(w, http.StatusBadRequest, catvod.Result{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) logSubAuthFailure(message, subID string, r *http.Request, reason string) {
	if s.logger == nil {
		return
	}
	s.logger.Warn(message, "sub", subID, "client", auth.ClientHost(r, serviceFromRequest(r).Config().TrustXForwardedFor), "reason", reason)
}

func tvboxErrorKind(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "authorization"):
		return "authorization"
	case strings.Contains(msg, "permission denied"):
		return "permission"
	case strings.Contains(msg, "invalid play id"):
		return "invalid_play_id"
	case strings.Contains(msg, "unknown mount"):
		return "unknown_mount"
	case strings.Contains(msg, "unknown sub"):
		return "unknown_sub"
	case strings.Contains(msg, "refresh is not enabled"):
		return "refresh_disabled"
	case strings.Contains(msg, "openlist request failed"):
		return "upstream_request"
	case strings.Contains(msg, "openlist"):
		return "upstream"
	default:
		return "request"
	}
}
