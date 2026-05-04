package gateway

import (
	"context"
	"crypto/rand"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/mount"
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
