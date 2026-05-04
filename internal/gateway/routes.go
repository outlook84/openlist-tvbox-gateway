package gateway

import (
	"net/http"
	"strings"

	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/mount"
	"openlist-tvbox/internal/subscription"
)

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /sub", s.subscription)
	s.mux.HandleFunc("GET /spider/openlist-tvbox.js", s.spider)
	s.mux.HandleFunc("GET /spider/", s.spider)
	s.mux.HandleFunc("GET /assets/icons/", s.icon)
	s.mux.HandleFunc("POST /api/sub/", s.authSub)
	s.mux.HandleFunc("POST /", s.dynamic)
	s.mux.HandleFunc("GET /", s.dynamic)
}

func (s *Server) subscription(w http.ResponseWriter, r *http.Request) {
	service := serviceFromRequest(r)
	if sub, ok := s.subByPath(service, r.URL.Path); ok {
		writeJSON(w, http.StatusOK, subscription.BuildForSub(service.Config(), sub, r))
		return
	}
	if r.URL.Path == "/sub" {
		http.NotFound(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) dynamic(w http.ResponseWriter, r *http.Request) {
	service := serviceFromRequest(r)
	if sub, ok := s.subByPath(service, r.URL.Path); ok {
		writeJSON(w, http.StatusOK, subscription.BuildForSub(service.Config(), sub, r))
		return
	}
	if subID, livePath, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/live/"); ok && strings.HasPrefix(subID, "s/") {
		s.liveForSub(service, w, r, strings.TrimPrefix(subID, "s/"), livePath)
		return
	}
	subID, apiPath, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/api/tvbox/")
	if !ok || !strings.HasPrefix(subID, "s/") {
		if subID, apiPath, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/api/sub/"); ok && strings.HasPrefix(subID, "s/") && apiPath == "auth" {
			s.authSubID(service, w, r, strings.TrimPrefix(subID, "s/"))
			return
		}
		http.NotFound(w, r)
		return
	}
	subID = strings.TrimPrefix(subID, "s/")
	switch apiPath {
	case "home":
		if !s.authorize(service, w, r, subID) {
			return
		}
		writeJSON(w, http.StatusOK, service.HomeForSub(subID))
	case "category":
		s.categoryForSub(service, w, r, subID)
	case "detail":
		s.detailForSub(service, w, r, subID)
	case "search":
		s.searchForSub(service, w, r, subID)
	case "play":
		s.playForSub(service, w, r, subID)
	case "refresh":
		s.refreshForSub(service, w, r, subID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) subByPath(service *mount.Service, requestPath string) (config.Subscription, bool) {
	requestPath = strings.TrimRight(requestPath, "/")
	if requestPath == "" {
		requestPath = "/"
	}
	for _, sub := range service.Config().Subs {
		if sub.Path == requestPath {
			return sub, true
		}
	}
	return config.Subscription{}, false
}
