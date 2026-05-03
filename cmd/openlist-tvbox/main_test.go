package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"openlist-tvbox/internal/admin"
	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/gateway"
	"openlist-tvbox/internal/mount"
	"openlist-tvbox/internal/openlist"
)

func TestWatchConfigReloadsChangedFile(t *testing.T) {
	path := writeTestConfig(t, "Movies")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reloaded := make(chan string, 1)
	go watchConfigWithTimings(ctx, path, slog.New(slog.NewTextHandler(os.Stdout, nil)), 20*time.Millisecond, 20*time.Millisecond, func(cfg *config.Config) {
		reloaded <- cfg.Subs[0].Mounts[0].Name
	})

	time.Sleep(40 * time.Millisecond)
	writeFile(t, path, testConfigYAML("Shows"))

	select {
	case got := <-reloaded:
		if got != "Shows" {
			t.Fatalf("reloaded mount name = %q, want Shows", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for config reload")
	}
}

func TestWatchConfigKeepsCurrentConfigWhenReloadFails(t *testing.T) {
	path := writeTestConfig(t, "Movies")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reloaded := make(chan string, 1)
	go watchConfigWithTimings(ctx, path, slog.New(slog.NewTextHandler(os.Stdout, nil)), 20*time.Millisecond, 20*time.Millisecond, func(cfg *config.Config) {
		reloaded <- cfg.Subs[0].Mounts[0].Name
	})

	time.Sleep(40 * time.Millisecond)
	writeFile(t, path, "backends: []\nsubs: []\n")

	select {
	case got := <-reloaded:
		t.Fatalf("unexpected reload with invalid config: %q", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestApplyConfigReloadsGatewayAndAdminTrustXForwardedFor(t *testing.T) {
	path := writeTestJSONConfig(t, "Movies", true)
	t.Setenv("OPENLIST_TVBOX_ADMIN_ACCESS_CODE", "123456789012")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	gatewayHandler := gateway.NewServer(mount.NewService(cfg, openlist.NewClient(http.DefaultClient, logger), logger), logger)
	var adminHandler *admin.Server
	applyConfig := func(cfg *config.Config) {
		reloadedClient := openlist.NewClient(http.DefaultClient, logger)
		gatewayHandler.SetService(mount.NewService(cfg, reloadedClient, logger))
		if adminHandler != nil {
			adminHandler.ApplyConfig(cfg)
		}
	}
	adminHandler, err = admin.NewServer(admin.Options{ConfigPath: path, OnSaved: applyConfig})
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := config.Load(writeTestJSONConfig(t, "Shows", false))
	if err != nil {
		t.Fatal(err)
	}
	applyConfig(reloaded)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/s/default/api/tvbox/home", nil)
	rec := httptest.NewRecorder()
	gatewayHandler.ServeHTTP(rec, req)
	if body := rec.Body.String(); !strings.Contains(body, "Shows") {
		t.Fatalf("gateway did not use reloaded service: %s", body)
	}
	loginReq := httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"123456789012"}`))
	loginRec := httptest.NewRecorder()
	adminHandler.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", loginRec.Code, loginRec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	for _, cookie := range loginRec.Result().Cookies() {
		req.AddCookie(cookie)
	}
	rec = httptest.NewRecorder()
	adminHandler.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func writeTestConfig(t *testing.T, mountName string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, testConfigYAML(mountName))
	return path
}

func writeTestJSONConfig(t *testing.T, mountName string, trustXForwardedFor bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, testConfigJSON(mountName, trustXForwardedFor))
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func testConfigYAML(mountName string) string {
	return `backends:
  - id: b1
    server: https://openlist.example.com
subs:
  - id: default
    mounts:
      - id: media
        name: ` + mountName + `
        backend: b1
        path: /Media
`
}

func testConfigJSON(mountName string, trustXForwardedFor bool) string {
	return `{
  "public_base_url": "http://127.0.0.1:18989",
  "trust_x_forwarded_for": ` + strconv.FormatBool(trustXForwardedFor) + `,
  "backends": [
    {
      "id": "b1",
      "server": "https://openlist.example.com"
    }
  ],
  "subs": [
    {
      "id": "default",
      "path": "/sub",
      "mounts": [
        {
          "id": "media",
          "name": "` + mountName + `",
          "backend": "b1",
          "path": "/Media"
        }
      ]
    }
  ]
}
`
}
