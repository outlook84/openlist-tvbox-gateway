package gateway

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/catvod"
	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/mount"
	"openlist-tvbox/internal/subscription"
)

//go:embed assets/openlist-tvbox.js
var spiderJS []byte

//go:embed assets/icons/folder.png
var folderIcon []byte

//go:embed assets/icons/video.png
var videoIcon []byte

//go:embed assets/icons/audio.png
var audioIcon []byte

//go:embed assets/icons/file.png
var fileIcon []byte

//go:embed assets/icons/playlist.png
var playlistIcon []byte

//go:embed assets/icons/refresh.png
var refreshIcon []byte

type Server struct {
	service      *mount.Service
	logger       *slog.Logger
	mux          *http.ServeMux
	authFailures map[string]authFailure
	authMu       sync.Mutex
}

type authFailure struct {
	Count        int
	LastFailedAt time.Time
	BlockedAt    time.Time
}

const (
	authFailureLimit = 5
	authCooldown     = 30 * time.Second
)

func NewServer(service *mount.Service, logger *slog.Logger) http.Handler {
	s := &Server{
		service:      service,
		logger:       logger,
		mux:          http.NewServeMux(),
		authFailures: map[string]authFailure{},
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

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
	if sub, ok := s.subByPath(r.URL.Path); ok {
		writeJSON(w, http.StatusOK, subscription.BuildForSub(s.service.Config(), sub, r))
		return
	}
	if r.URL.Path == "/sub" {
		http.NotFound(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) dynamic(w http.ResponseWriter, r *http.Request) {
	if sub, ok := s.subByPath(r.URL.Path); ok {
		writeJSON(w, http.StatusOK, subscription.BuildForSub(s.service.Config(), sub, r))
		return
	}
	subID, apiPath, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/api/tvbox/")
	if !ok || !strings.HasPrefix(subID, "s/") {
		if subID, apiPath, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/api/sub/"); ok && strings.HasPrefix(subID, "s/") && apiPath == "auth" {
			s.authSubID(w, r, strings.TrimPrefix(subID, "s/"))
			return
		}
		http.NotFound(w, r)
		return
	}
	subID = strings.TrimPrefix(subID, "s/")
	switch apiPath {
	case "home":
		if !s.authorize(w, r, subID) {
			return
		}
		writeJSON(w, http.StatusOK, s.service.HomeForSub(subID))
	case "category":
		s.categoryForSub(w, r, subID)
	case "detail":
		s.detailForSub(w, r, subID)
	case "search":
		s.searchForSub(w, r, subID)
	case "play":
		s.playForSub(w, r, subID)
	case "refresh":
		s.refreshForSub(w, r, subID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) subByPath(requestPath string) (config.Subscription, bool) {
	requestPath = strings.TrimRight(requestPath, "/")
	if requestPath == "" {
		requestPath = "/"
	}
	for _, sub := range s.service.Config().Subs {
		if sub.Path == requestPath {
			return sub, true
		}
	}
	return config.Subscription{}, false
}

func (s *Server) spider(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/spider/openlist-tvbox.js" &&
		(!strings.HasPrefix(r.URL.Path, "/spider/openlist-tvbox.") || !strings.HasSuffix(r.URL.Path, ".js")) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(spiderJS)
}

func (s *Server) icon(w http.ResponseWriter, r *http.Request) {
	var data []byte
	switch r.URL.Path {
	case "/assets/icons/folder.png":
		data = folderIcon
	case "/assets/icons/video.png":
		data = videoIcon
	case "/assets/icons/audio.png":
		data = audioIcon
	case "/assets/icons/file.png":
		data = fileIcon
	case "/assets/icons/playlist.png":
		data = playlistIcon
	case "/assets/icons/refresh.png":
		data = refreshIcon
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) categoryForSub(w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(w, r, subID) {
		return
	}
	q := r.URL.Query()
	result, err := s.service.CategoryForSub(r.Context(), subID, q.Get("tid"), q.Get("type"), q.Get("order"))
	writeResult(w, result, err)
}

func (s *Server) detailForSub(w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(w, r, subID) {
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Query().Get("ids"), "[")
		id = strings.TrimSuffix(id, "]")
		id = strings.Trim(id, "\"")
	}
	result, err := s.service.DetailForSub(r.Context(), subID, id)
	writeResult(w, result, err)
}

func (s *Server) searchForSub(w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(w, r, subID) {
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		key = r.URL.Query().Get("wd")
	}
	result, err := s.service.SearchForSub(r.Context(), subID, key)
	writeResult(w, result, err)
}

func (s *Server) playForSub(w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(w, r, subID) {
		return
	}
	result, err := s.service.PlayForSub(r.Context(), subID, r.URL.Query().Get("id"))
	writeResult(w, result, err)
}

func (s *Server) refreshForSub(w http.ResponseWriter, r *http.Request, subID string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !s.authorize(w, r, subID) {
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
	result, err := s.service.RefreshForSub(r.Context(), subID, id)
	writeResult(w, result, err)
}

func (s *Server) authSub(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sub/")
	subID, action, ok := strings.Cut(path, "/")
	if !ok || action != "auth" {
		http.NotFound(w, r)
		return
	}
	s.authSubID(w, r, subID)
}

func (s *Server) authSubID(w http.ResponseWriter, r *http.Request, subID string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	key := s.authFailureKey(r, subID)
	if s.authBlocked(key) {
		writeJSON(w, http.StatusTooManyRequests, map[string]bool{"ok": false})
		return
	}
	code := accessCodeFromRequest(r)
	if s.validCode(subID, code) {
		s.clearAuthFailure(key)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if code != "" {
		s.recordAuthFailure(key)
	}
	writeJSON(w, http.StatusUnauthorized, map[string]bool{"ok": false})
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request, subID string) bool {
	sub, ok := s.subByID(subID)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, catvod.Result{Error: "unauthorized"})
		return false
	}
	if sub.AccessCodeHash == "" {
		return true
	}
	key := s.authFailureKey(r, sub.ID)
	if s.authBlocked(key) {
		writeJSON(w, http.StatusTooManyRequests, catvod.Result{Error: "too many failed access code attempts"})
		return false
	}
	code := accessCodeFromRequest(r)
	if s.validCode(sub.ID, code) {
		s.clearAuthFailure(key)
		return true
	}
	if code != "" {
		s.recordAuthFailure(key)
	}
	writeJSON(w, http.StatusUnauthorized, catvod.Result{Error: "unauthorized"})
	return false
}

func (s *Server) validCode(subID, code string) bool {
	sub, ok := s.subByID(subID)
	if !ok || sub.AccessCodeHash == "" || code == "" {
		return false
	}
	if auth.ValidateAccessCode(code) != nil {
		return false
	}
	return auth.VerifyPassword(sub.AccessCodeHash, code) == nil
}

func (s *Server) authFailureKey(r *http.Request, subID string) string {
	host := r.RemoteAddr
	if s.service.Config().TrustXForwardedFor {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			host = strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if host == "" {
		host = r.RemoteAddr
	}
	return subID + "|" + host
}

func (s *Server) authBlocked(key string) bool {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	failure, ok := s.authFailures[key]
	if !ok {
		return false
	}
	if failure.Count < authFailureLimit {
		if time.Since(failure.LastFailedAt) >= authCooldown {
			delete(s.authFailures, key)
		}
		return false
	}
	if time.Since(failure.BlockedAt) >= authCooldown {
		delete(s.authFailures, key)
		return false
	}
	return true
}

func (s *Server) recordAuthFailure(key string) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	failure := s.authFailures[key]
	now := time.Now()
	if !failure.LastFailedAt.IsZero() && now.Sub(failure.LastFailedAt) >= authCooldown {
		failure = authFailure{}
	}
	failure.Count++
	failure.LastFailedAt = now
	if failure.Count >= authFailureLimit && failure.BlockedAt.IsZero() {
		failure.BlockedAt = now
	}
	s.authFailures[key] = failure
}

func (s *Server) clearAuthFailure(key string) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	delete(s.authFailures, key)
}

func (s *Server) subByID(subID string) (config.Subscription, bool) {
	for _, sub := range s.service.Config().Subs {
		if sub.ID == subID {
			return sub, true
		}
	}
	return config.Subscription{}, false
}

func accessCodeFromRequest(r *http.Request) string {
	if code := r.URL.Query().Get("code"); code != "" {
		return code
	}
	if code := r.Header.Get("X-Access-Code"); code != "" {
		return code
	}
	var body struct {
		Code string `json:"code"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body)
	}
	return body.Code
}

func writeResult(w http.ResponseWriter, result catvod.Result, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, catvod.Result{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
