package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
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

//go:embed assets/icons/logo.svg
var logoIcon []byte

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
	service     atomic.Pointer[mount.Service]
	logger      *slog.Logger
	mux         *http.ServeMux
	httpClient  *http.Client
	authLimiter *auth.FailureLimiter
	tokenSecret []byte
}

const (
	authFailureLimit        = auth.DefaultFailureLimit
	authCooldown            = auth.DefaultFailureCooldown
	accessTokenTTL          = 12 * time.Hour
	liveProxyTimeout        = 20 * time.Second
	maxLivePlaylistBodySize = 32 << 20
)

func NewServer(service *mount.Service, logger *slog.Logger) *Server {
	tokenSecret := make([]byte, 32)
	if _, err := rand.Read(tokenSecret); err != nil {
		panic(fmt.Errorf("generate access token secret: %w", err))
	}
	s := &Server{
		logger:      logger,
		mux:         http.NewServeMux(),
		httpClient:  &http.Client{Timeout: 20 * time.Second},
		authLimiter: auth.NewFailureLimiter(authFailureLimit, authCooldown),
		tokenSecret: tokenSecret,
	}
	s.SetService(service)
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	service := s.currentService()
	r = r.WithContext(context.WithValue(r.Context(), serviceContextKey{}, service))
	s.mux.ServeHTTP(w, r)
}

func (s *Server) SetService(service *mount.Service) {
	s.service.Store(service)
}

func (s *Server) currentService() *mount.Service {
	return s.service.Load()
}

type serviceContextKey struct{}

func serviceFromRequest(r *http.Request) *mount.Service {
	service, _ := r.Context().Value(serviceContextKey{}).(*mount.Service)
	return service
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
	contentType := "image/png"
	switch r.URL.Path {
	case "/assets/icons/folder.png":
		data = folderIcon
	case "/assets/icons/logo.svg":
		data = logoIcon
		contentType = "image/svg+xml; charset=utf-8"
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
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) categoryForSub(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(service, w, r, subID) {
		return
	}
	q := r.URL.Query()
	result, err := service.CategoryForSub(r.Context(), subID, q.Get("tid"), q.Get("type"), q.Get("order"))
	writeResult(w, result, err)
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
	writeResult(w, result, err)
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
	writeResult(w, result, err)
}

func (s *Server) playForSub(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) {
	if !s.authorize(service, w, r, subID) {
		return
	}
	result, err := service.PlayForSub(r.Context(), subID, r.URL.Query().Get("id"))
	writeResult(w, result, err)
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
	writeResult(w, result, err)
}

func (s *Server) liveForSub(service *mount.Service, w http.ResponseWriter, r *http.Request, subID, livePath string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	liveIndex, _, _ := strings.Cut(livePath, "/")
	index, err := strconv.Atoi(liveIndex)
	if err != nil || index < 0 {
		http.NotFound(w, r)
		return
	}
	sub, ok := s.subByID(service, subID)
	if !ok || index >= len(sub.Lives) {
		http.NotFound(w, r)
		return
	}
	live := sub.Lives[index]
	ctx, cancel := context.WithTimeout(r.Context(), liveProxyTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, live.URL, nil)
	if err != nil {
		s.logLiveProxyFailure(subID, index, "build_request", "invalid configured url")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: invalid configured url"})
		return
	}
	if live.UA != "" {
		req.Header.Set("User-Agent", live.UA)
	} else if ua := r.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logLiveProxyFailure(subID, index, "request", "request failed")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: request failed"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logLiveProxyFailure(subID, index, "status", "upstream returned non-success status")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: fmt.Sprintf("live source request failed: upstream status %d", resp.StatusCode)})
		return
	}
	if resp.ContentLength > maxLivePlaylistBodySize {
		s.logLiveProxyFailure(subID, index, "size", "playlist too large")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: playlist too large"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLivePlaylistBodySize+1))
	if err != nil {
		s.logLiveProxyFailure(subID, index, "read", "read failed")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: read failed"})
		return
	}
	if len(body) > maxLivePlaylistBodySize {
		s.logLiveProxyFailure(subID, index, "size", "playlist too large")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: playlist too large"})
		return
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if cacheControl := resp.Header.Get("Cache-Control"); cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) logLiveProxyFailure(subID string, index int, stage, reason string) {
	if s.logger == nil {
		return
	}
	s.logger.Warn("live source request failed", "sub", subID, "index", index, "stage", stage, "reason", reason)
}

func (s *Server) authSub(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sub/")
	subID, action, ok := strings.Cut(path, "/")
	if !ok || action != "auth" {
		http.NotFound(w, r)
		return
	}
	s.authSubID(serviceFromRequest(r), w, r, subID)
}

func (s *Server) authSubID(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	key := s.authFailureKey(r, subID)
	if s.authLimiter.Blocked(key) {
		writeJSON(w, http.StatusTooManyRequests, map[string]bool{"ok": false})
		return
	}
	code := accessCodeFromRequest(r)
	if s.validCode(service, subID, code) {
		s.authLimiter.Clear(key)
		sub, _ := s.subByID(service, subID)
		token, expiresAt := s.issueAccessToken(sub)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "access_token": token, "expires_at": expiresAt})
		return
	}
	if code != "" {
		s.authLimiter.RecordFailure(key)
	}
	writeJSON(w, http.StatusUnauthorized, map[string]bool{"ok": false})
}

func (s *Server) authorize(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) bool {
	sub, ok := s.subByID(service, subID)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, catvod.Result{Error: "unauthorized"})
		return false
	}
	if sub.AccessCodeHash == "" {
		return true
	}
	if s.validAccessToken(sub, accessTokenFromRequest(r)) {
		return true
	}
	key := s.authFailureKey(r, sub.ID)
	if s.authLimiter.Blocked(key) {
		writeJSON(w, http.StatusTooManyRequests, catvod.Result{Error: "too many failed access code attempts"})
		return false
	}
	writeJSON(w, http.StatusUnauthorized, catvod.Result{Error: "unauthorized"})
	return false
}

func (s *Server) validCode(service *mount.Service, subID, code string) bool {
	sub, ok := s.subByID(service, subID)
	if !ok || sub.AccessCodeHash == "" || code == "" {
		return false
	}
	if auth.ValidateAccessCode(code) != nil {
		return false
	}
	return auth.VerifyPassword(sub.AccessCodeHash, code) == nil
}

func (s *Server) authFailureKey(r *http.Request, subID string) string {
	return subID + "|" + auth.ClientHost(r, serviceFromRequest(r).Config().TrustXForwardedFor)
}

func (s *Server) subByID(service *mount.Service, subID string) (config.Subscription, bool) {
	for _, sub := range service.Config().Subs {
		if sub.ID == subID {
			return sub, true
		}
	}
	return config.Subscription{}, false
}

func accessCodeFromRequest(r *http.Request) string {
	var body struct {
		Code string `json:"code"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body)
	}
	return body.Code
}

func accessTokenFromRequest(r *http.Request) string {
	return r.Header.Get("X-Access-Token")
}

func (s *Server) issueAccessToken(sub config.Subscription) (string, int64) {
	expiresAt := time.Now().Add(accessTokenTTL).Unix()
	fingerprint := accessHashFingerprint(sub.AccessCodeHash)
	payload := fmt.Sprintf("%s.%d.%s", sub.ID, expiresAt, fingerprint)
	signature := s.signAccessTokenPayload(payload)
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "." + signature)), expiresAt
}

func (s *Server) validAccessToken(sub config.Subscription, token string) bool {
	if token == "" {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 4 {
		return false
	}
	if parts[0] != sub.ID {
		return false
	}
	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() >= expiresAt {
		return false
	}
	if parts[2] != accessHashFingerprint(sub.AccessCodeHash) {
		return false
	}
	payload := strings.Join(parts[:3], ".")
	want := s.signAccessTokenPayload(payload)
	return hmac.Equal([]byte(parts[3]), []byte(want))
}

func (s *Server) signAccessTokenPayload(payload string) string {
	mac := hmac.New(sha256.New, s.tokenSecret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func accessHashFingerprint(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return base64.RawURLEncoding.EncodeToString(sum[:])
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
