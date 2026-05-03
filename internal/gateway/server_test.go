package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/mount"
	"openlist-tvbox/internal/openlist"
)

type fakeOpenListClient struct{}

func (fakeOpenListClient) List(context.Context, config.Backend, string, string) ([]openlist.Item, error) {
	return nil, nil
}

func (fakeOpenListClient) RefreshList(context.Context, config.Backend, string, string) ([]openlist.Item, error) {
	return nil, nil
}

func (fakeOpenListClient) Get(context.Context, config.Backend, string, string) (openlist.Item, error) {
	return openlist.Item{}, nil
}

func (fakeOpenListClient) Search(context.Context, config.Backend, string, string, string) ([]openlist.Item, error) {
	return nil, nil
}

func TestConfiguredSubPathReturnsScopedSubscription(t *testing.T) {
	cfg := testGatewayConfig(t)
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/custom/shows", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Sites []struct {
			Key string `json:"key"`
			Ext string `json:"ext"`
		} `json:"sites"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Sites) != 1 || !strings.HasPrefix(got.Sites[0].Key, "shows_key_u") {
		t.Fatalf("unexpected subscription: %s", rec.Body.String())
	}
	var ext map[string]string
	if err := json.Unmarshal([]byte(got.Sites[0].Ext), &ext); err != nil {
		t.Fatal(err)
	}
	if ext["gateway"] != "http://gateway.example.com/s/shows" || !strings.HasPrefix(ext["skey"], "openlist_tvbox_shows_u") {
		t.Fatalf("unexpected subscription ext: %#v", ext)
	}
}

func TestUnconfiguredSubPathReturnsNotFound(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[0].Path = "/custom/movies"
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/sub", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestSubscriptionAliasesDoNotFallbackToFirstSub(t *testing.T) {
	handler := NewServer(mount.NewService(testGatewayConfig(t), fakeOpenListClient{}, nil), nil)
	for _, path := range []string{"/config.json", "/tvbox.json"} {
		req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com"+path, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d body = %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestUnscopedTVBoxAPIDoesNotFallbackToFirstSub(t *testing.T) {
	handler := NewServer(mount.NewService(testGatewayConfig(t), fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/api/tvbox/home", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestIconRouteServesOnlyBuiltInIcons(t *testing.T) {
	handler := NewServer(mount.NewService(testGatewayConfig(t), fakeOpenListClient{}, nil), nil)
	for _, path := range []string{"/assets/icons/folder.png", "/assets/icons/logo.svg", "/assets/icons/video.png", "/assets/icons/audio.png", "/assets/icons/file.png", "/assets/icons/playlist.png", "/assets/icons/refresh.png"} {
		req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com"+path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		wantContentType := "image/png"
		if path == "/assets/icons/logo.svg" {
			wantContentType = "image/svg+xml; charset=utf-8"
		}
		if rec.Header().Get("Content-Type") != wantContentType {
			t.Fatalf("%s content-type = %q", path, rec.Header().Get("Content-Type"))
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("%s returned empty body", path)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/assets/icons/other.png", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected icon status = %d", rec.Code)
	}
}

func TestScopedAPIUsesSubMounts(t *testing.T) {
	cfg := testGatewayConfig(t)
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/api/tvbox/home", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Class []struct {
			TypeID   string `json:"type_id"`
			TypeName string `json:"type_name"`
		} `json:"class"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Class) != 1 || got.Class[0].TypeID != "shows" {
		t.Fatalf("class = %#v", got.Class)
	}
}

func TestRequestUsesSingleServiceSnapshotAfterReload(t *testing.T) {
	client := &swapDuringListClient{started: make(chan struct{}), release: make(chan struct{})}
	oldCfg := &config.Config{
		Backends: []config.Backend{{ID: "old", Server: "https://old.example.com"}},
		Subs: []config.Subscription{{
			ID:     "movies",
			Mounts: []config.Mount{{ID: "media", Backend: "old", Path: "/Old"}},
		}},
	}
	if err := oldCfg.Validate(); err != nil {
		t.Fatal(err)
	}
	newCfg := &config.Config{
		Backends: []config.Backend{{ID: "new", Server: "https://new.example.com"}},
		Subs: []config.Subscription{{
			ID:             "movies",
			AccessCodeHash: mustHash(t, "123456"),
			Mounts:         []config.Mount{{ID: "media", Backend: "new", Path: "/New"}},
		}},
	}
	if err := newCfg.Validate(); err != nil {
		t.Fatal(err)
	}
	handler := NewServer(mount.NewService(oldCfg, client, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/movies/api/tvbox/category?tid=media", nil)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	<-client.started
	handler.SetService(mount.NewService(newCfg, client, nil))
	close(client.release)
	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	client.mu.Lock()
	backendID, listPath := client.backendID, client.path
	client.mu.Unlock()
	if backendID != "old" || listPath != "/Old" {
		t.Fatalf("category used backend/path = %q %q, want old /Old", backendID, listPath)
	}

	req = httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/movies/api/tvbox/category?tid=media", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("next request status = %d body = %s, want unauthorized from new config", rec.Code, rec.Body.String())
	}
}

type swapDuringListClient struct {
	mu        sync.Mutex
	backendID string
	path      string
	started   chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (s *swapDuringListClient) List(_ context.Context, backend config.Backend, path, _ string) ([]openlist.Item, error) {
	s.mu.Lock()
	s.backendID = backend.ID
	s.path = path
	s.mu.Unlock()
	s.once.Do(func() {
		close(s.started)
		<-s.release
	})
	return nil, nil
}

func (s *swapDuringListClient) RefreshList(context.Context, config.Backend, string, string) ([]openlist.Item, error) {
	return nil, nil
}

func (s *swapDuringListClient) Get(context.Context, config.Backend, string, string) (openlist.Item, error) {
	return openlist.Item{}, nil
}

func (s *swapDuringListClient) Search(context.Context, config.Backend, string, string, string) ([]openlist.Item, error) {
	return nil, nil
}

func TestScopedRefreshAPIUsesSubMounts(t *testing.T) {
	client := &recordingGatewayClient{}
	cfg := &config.Config{
		Backends: []config.Backend{
			{ID: "b1", Server: "https://one.example.com"},
			{ID: "b2", Server: "https://two.example.com"},
		},
		Subs: []config.Subscription{
			{
				ID:     "movies",
				Mounts: []config.Mount{{ID: "same", Backend: "b1", Path: "/Movies", Refresh: true}},
			},
			{
				ID:     "shows",
				Mounts: []config.Mount{{ID: "same", Backend: "b2", Path: "/Shows", Refresh: true}},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	handler := NewServer(mount.NewService(cfg, client, nil), nil)
	req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/s/shows/api/tvbox/refresh", strings.NewReader(`{"id":"same/season"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if client.refreshBackendID != "b2" || client.refreshPath != "/Shows/season" {
		t.Fatalf("refresh backend/path = %q %q, want b2 /Shows/season", client.refreshBackendID, client.refreshPath)
	}
}

func TestProtectedSubSubscriptionIsPublicButAPIRequiresCode(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)

	subReq := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/custom/shows", nil)
	subRec := httptest.NewRecorder()
	handler.ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusOK {
		t.Fatalf("subscription status = %d body = %s", subRec.Code, subRec.Body.String())
	}

	apiReq := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/api/tvbox/home", nil)
	apiRec := httptest.NewRecorder()
	handler.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusUnauthorized {
		t.Fatalf("api status = %d body = %s", apiRec.Code, apiRec.Body.String())
	}
}

func TestProtectedSubAPIAcceptsAccessToken(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	token := authenticateSub(t, handler, "shows", "123456")
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/api/tvbox/home", nil)
	req.Header.Set("X-Access-Token", token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedSubAPIRejectsAccessTokenInURL(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	token := authenticateSub(t, handler, "shows", "123456")
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/api/tvbox/home?access_token="+token, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedSubAPIRejectsAccessCodeOnResourceRequest(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/api/tvbox/home?code=123456", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedSubAccessTokenInvalidAfterAccessCodeChange(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	token := authenticateSub(t, handler, "shows", "123456")

	next := testGatewayConfig(t)
	next.Subs[1].AccessCodeHash = mustHash(t, "654321")
	handler.SetService(mount.NewService(next, fakeOpenListClient{}, nil))

	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/api/tvbox/home", nil)
	req.Header.Set("X-Access-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestSubscriptionLiveUsesGatewayProxyURL(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.PublicBaseURL = "http://gateway.example.com"
	cfg.Subs[1].Lives = []config.Live{{Name: "Live", URL: "https://live.example.com/list.m3u", PlayerType: 2}}
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/custom/shows", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "live.example.com") {
		t.Fatalf("subscription leaked live source URL: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"url":"http://gateway.example.com/s/shows/live/0/list.m3u"`) {
		t.Fatalf("subscription missing live proxy URL: %s", rec.Body.String())
	}
}

func TestLiveProxyFetchesConfiguredLiveListOnly(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/list.m3u" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,Test\nhttp://stream.example.com/live.m3u8\n"))
	}))
	t.Cleanup(liveServer.Close)

	cfg := testGatewayConfig(t)
	cfg.Subs[1].Lives = []config.Live{{Name: "Live", URL: liveServer.URL + "/list.m3u"}}
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/live/0/list.m3u", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "audio/x-mpegurl" {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "http://stream.example.com/live.m3u8") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLiveProxyRejectsPlaylistAboveDeclaredSizeLimit(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(maxLivePlaylistBodySize+1))
		_, _ = w.Write([]byte("#EXTM3U\n"))
	}))
	t.Cleanup(liveServer.Close)

	cfg := testGatewayConfig(t)
	cfg.Subs[1].Lives = []config.Live{{Name: "Live", URL: liveServer.URL + "/list.m3u"}}
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/live/0/list.m3u", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "playlist too large") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLiveProxyRejectsPlaylistAboveReadSizeLimit(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", maxLivePlaylistBodySize+1)))
	}))
	t.Cleanup(liveServer.Close)

	cfg := testGatewayConfig(t)
	cfg.Subs[1].Lives = []config.Live{{Name: "Live", URL: liveServer.URL + "/list.m3u"}}
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/live/0/list.m3u", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body length = %d", rec.Code, rec.Body.Len())
	}
	if rec.Body.Len() >= maxLivePlaylistBodySize {
		t.Fatalf("response was truncated success-sized body: length = %d", rec.Body.Len())
	}
	if !strings.Contains(rec.Body.String(), "playlist too large") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLiveProxyRejectsUnknownLiveIndex(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].Lives = []config.Live{{Name: "Live", URL: "https://live.example.com/list.m3u"}}
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/live/1/list.m3u", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestLiveProxyLogsDoNotLeakConfiguredLiveURL(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].Lives = []config.Live{{Name: "Live", URL: "http://127.0.0.1:1/list.m3u?token=secret-live-token"}}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), logger)
	req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/live/0/list.m3u", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	text := logs.String()
	for _, forbidden := range []string{"secret-live-token", "token=", "live.example.com", "127.0.0.1:1/list.m3u"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("log leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "reason=\"request failed\"") && !strings.Contains(text, "reason=\"invalid configured url\"") {
		t.Fatalf("log missing fixed reason: %s", text)
	}
}

func TestProtectedSubAPIMissingCodeDoesNotConsumeCooldown(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)

	for i := 0; i < authFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://gateway.example.com/s/shows/api/tvbox/home", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/shows/auth", strings.NewReader(`{"code":"123456"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAuthEndpointAcceptsJSONCode(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/shows/auth", strings.NewReader(`{"code":"123456"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"access_token"`) || !strings.Contains(rec.Body.String(), `"expires_at"`) {
		t.Fatalf("missing token response: %s", rec.Body.String())
	}
}

func TestAuthEndpointCoolsDownAfterFailures(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)

	for i := 0; i < authFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/shows/auth", strings.NewReader(`{"code":"0000"}`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/shows/auth", strings.NewReader(`{"code":"0000"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAuthEndpointExpiredPartialFailuresDoNotTripCooldown(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)
	server := handler
	key := "shows|192.0.2.1"
	server.authLimiter.Set(key, auth.Failure{
		Count:        authFailureLimit - 1,
		LastFailedAt: time.Now().Add(-authCooldown),
	})

	req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/shows/auth", strings.NewReader(`{"code":"0000"}`))
	req.RemoteAddr = "192.0.2.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	failure, _ := server.authLimiter.Get(key)
	if failure.Count != 1 {
		t.Fatalf("failure count = %d, want 1", failure.Count)
	}
}

func TestAuthEndpointCooldownIgnoresForwardedForByDefault(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)

	for i := 0; i < authFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/shows/auth", strings.NewReader(`{"code":"0000"}`))
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i+1))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/shows/auth", strings.NewReader(`{"code":"0000"}`))
	req.Header.Set("X-Forwarded-For", "198.51.100.250")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAuthEndpointCooldownCanTrustForwardedFor(t *testing.T) {
	cfg := testGatewayConfig(t)
	cfg.TrustXForwardedFor = true
	cfg.Subs[1].AccessCodeHash = mustHash(t, "123456")
	handler := NewServer(mount.NewService(cfg, fakeOpenListClient{}, nil), nil)

	for i := 0; i < authFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/shows/auth", strings.NewReader(`{"code":"0000"}`))
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i+1))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/shows/auth", strings.NewReader(`{"code":"0000"}`))
	req.Header.Set("X-Forwarded-For", "198.51.100.250")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func testGatewayConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		Backends: []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs: []config.Subscription{
			{
				ID:      "movies",
				Path:    "/sub",
				SiteKey: "movies_key",
				Mounts:  []config.Mount{{ID: "movies", Backend: "b1", Path: "/Movies"}},
			},
			{
				ID:      "shows",
				Path:    "/custom/shows",
				SiteKey: "shows_key",
				Mounts:  []config.Mount{{ID: "shows", Backend: "b1", Path: "/Shows"}},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func mustHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func authenticateSub(t *testing.T, handler http.Handler, subID, code string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/sub/"+subID+"/auth", strings.NewReader(`{"code":"`+code+`"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("auth status = %d body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		AccessToken string `json:"access_token"`
		ExpiresAt   int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.AccessToken == "" || got.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("invalid auth token response: %s", rec.Body.String())
	}
	return got.AccessToken
}

type recordingGatewayClient struct {
	refreshBackendID string
	refreshPath      string
}

func (r *recordingGatewayClient) List(context.Context, config.Backend, string, string) ([]openlist.Item, error) {
	return nil, nil
}

func (r *recordingGatewayClient) RefreshList(_ context.Context, backend config.Backend, path string, _ string) ([]openlist.Item, error) {
	r.refreshBackendID = backend.ID
	r.refreshPath = path
	return nil, nil
}

func (r *recordingGatewayClient) Get(context.Context, config.Backend, string, string) (openlist.Item, error) {
	return openlist.Item{}, nil
}

func (r *recordingGatewayClient) Search(context.Context, config.Backend, string, string, string) ([]openlist.Item, error) {
	return nil, nil
}
