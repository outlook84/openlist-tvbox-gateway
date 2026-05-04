package openlist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"openlist-tvbox/internal/config"
)

func TestClientAnonymousAuthSendsNoAuthorization(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeOpenListJSON(t, w, map[string]any{
			"code":    200,
			"message": "success",
			"data":    map[string]any{"content": []any{}},
		})
	}))
	defer server.Close()
	client := NewClient(server.Client(), nil)
	_, err := client.List(context.Background(), config.Backend{
		ID:     "guest",
		Server: server.URL,
	}, "/", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization header = %q", gotAuth)
	}
}

func TestClientRefreshListSendsRefreshFlag(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		writeOpenListJSON(t, w, map[string]any{
			"code":    200,
			"message": "success",
			"data":    map[string]any{"content": []any{}},
		})
	}))
	defer server.Close()
	client := NewClient(server.Client(), nil)
	_, err := client.RefreshList(context.Background(), config.Backend{ID: "main", Server: server.URL}, "/Movies", "pass")
	if err != nil {
		t.Fatal(err)
	}
	if body["path"] != "/Movies" || body["password"] != "pass" || body["refresh"] != true {
		t.Fatalf("body = %#v", body)
	}
}

func TestClientPasswordAuthCoalescesConcurrentLogin(t *testing.T) {
	var mu sync.Mutex
	loginCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			if got := r.Header.Get("Client-Id"); got != "openlist-tvbox-main" {
				t.Errorf("login Client-Id = %q", got)
			}
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			loginCount++
			mu.Unlock()
			writeOpenListJSON(t, w, map[string]any{
				"code":    200,
				"message": "success",
				"data":    map[string]any{"token": "login-token", "device_key": "alist-device"},
			})
			return
		}
		if got := r.Header.Get("Authorization"); got != "login-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Client-Id"); got != "openlist-tvbox-main" {
			t.Errorf("request Client-Id = %q", got)
		}
		writeOpenListJSON(t, w, map[string]any{
			"code":    200,
			"message": "success",
			"data":    map[string]any{"content": []any{}},
		})
	}))
	defer server.Close()
	client := NewClient(server.Client(), nil)
	backend := config.Backend{
		ID:       "main",
		Server:   server.URL,
		AuthType: "password",
		User:     "admin",
		Password: "password",
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.List(context.Background(), backend, "/", "")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if loginCount != 1 {
		t.Fatalf("login count = %d", loginCount)
	}
}

func TestClientPasswordAuthReloginsOnceAfterUnauthorized(t *testing.T) {
	var mu sync.Mutex
	loginCount := 0
	fsCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			mu.Lock()
			loginCount++
			token := "old-token"
			if loginCount > 1 {
				token = "new-token"
			}
			mu.Unlock()
			writeOpenListJSON(t, w, map[string]any{
				"code":    200,
				"message": "success",
				"data":    map[string]any{"token": token},
			})
			return
		}
		mu.Lock()
		fsCount++
		count := fsCount
		auth := r.Header.Get("Authorization")
		mu.Unlock()
		if count == 1 {
			if auth != "old-token" {
				t.Errorf("first Authorization = %q", auth)
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"message":"token expired","data":null}`))
			return
		}
		if auth != "new-token" {
			t.Errorf("retry Authorization = %q", auth)
		}
		writeOpenListJSON(t, w, map[string]any{
			"code":    200,
			"message": "success",
			"data":    map[string]any{"content": []any{}},
		})
	}))
	defer server.Close()
	client := NewClient(server.Client(), nil)
	_, err := client.List(context.Background(), config.Backend{
		ID:       "main",
		Server:   server.URL,
		AuthType: "password",
		User:     "admin",
		Password: "password",
	}, "/", "")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if loginCount != 2 {
		t.Fatalf("login count = %d", loginCount)
	}
	if fsCount != 2 {
		t.Fatalf("fs count = %d", fsCount)
	}
}

func TestClientPasswordRefreshPermissionDeniedDoesNotRelogin(t *testing.T) {
	var mu sync.Mutex
	loginCount := 0
	fsCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			mu.Lock()
			loginCount++
			mu.Unlock()
			writeOpenListJSON(t, w, map[string]any{
				"code":    200,
				"message": "success",
				"data":    map[string]any{"token": "login-token"},
			})
			return
		}
		mu.Lock()
		fsCount++
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
		writeOpenListJSON(t, w, map[string]any{
			"code":    403,
			"message": "Refresh without permission",
			"data":    nil,
		})
	}))
	defer server.Close()
	client := NewClient(server.Client(), nil)
	_, err := client.RefreshList(context.Background(), config.Backend{
		ID:       "main",
		Server:   server.URL,
		AuthType: "password",
		User:     "admin",
		Password: "password",
	}, "/", "")
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), "Refresh without permission") {
		t.Fatalf("error = %q", err.Error())
	}
	mu.Lock()
	defer mu.Unlock()
	if loginCount != 1 {
		t.Fatalf("login count = %d", loginCount)
	}
	if fsCount != 1 {
		t.Fatalf("fs count = %d", fsCount)
	}
}

func TestBackendAPIURLBuildsAllowedOpenListPaths(t *testing.T) {
	got, err := backendAPIURL(" https://openlist.example.com/base/ ", "/api/fs/list")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://openlist.example.com/base/api/fs/list" {
		t.Fatalf("url = %q", got)
	}
}

func TestBackendAPIURLRejectsUnsafeServerURL(t *testing.T) {
	tests := []struct {
		name   string
		server string
	}{
		{name: "relative", server: "/openlist"},
		{name: "ftp", server: "ftp://openlist.example.com"},
		{name: "userinfo", server: "https://user:pass@openlist.example.com"},
		{name: "query", server: "https://openlist.example.com?token=secret"},
		{name: "fragment", server: "https://openlist.example.com#token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := backendAPIURL(tt.server, "/api/fs/list"); err == nil {
				t.Fatalf("backendAPIURL() = %q, nil error", got)
			}
		})
	}
}

func TestBackendAPIURLRejectsUnsupportedAPIPath(t *testing.T) {
	if got, err := backendAPIURL("https://openlist.example.com", "/api/fs/remove"); err == nil {
		t.Fatalf("backendAPIURL() = %q, nil error", got)
	}
}

func writeOpenListJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
