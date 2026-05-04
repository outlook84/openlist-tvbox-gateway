package admin

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/logging"
)

func init() {
	adminBcryptCost = bcrypt.MinCost
}

func TestNewServerCreatesSetupCodeForJSONMode(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if server.hash != "" {
		t.Fatal("admin hash should not be initialized before setup")
	}
	setupPath := filepath.Join(filepath.Dir(path), setupCodeFileName)
	if _, err := os.Stat(setupPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(setupPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != server.setupCode {
		t.Fatal("setup code file does not match server state")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), secretFileName)); !os.IsNotExist(err) {
		t.Fatalf("admin hash should not exist before setup: %v", err)
	}
}

func TestNewServerFailsClosedWhenSecretCannotBeRead(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	secretPath := filepath.Join(filepath.Dir(path), secretFileName)
	if err := os.Mkdir(secretPath, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := NewServer(Options{ConfigPath: path})
	if err == nil {
		t.Fatal("expected unreadable admin secret to fail startup")
	}
	if !strings.Contains(err.Error(), "read admin secret file") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), setupCodeFileName)); !os.IsNotExist(err) {
		t.Fatalf("setup code should not be created when admin secret is unreadable: %v", err)
	}
}

func TestNewServerDoesNotLogSetupCode(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	server, err := NewServer(Options{ConfigPath: path, Listen: ":18989", Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	text := logs.String()
	if strings.Contains(text, server.setupCode) {
		t.Fatalf("log leaked setup code: %s", text)
	}
	if !strings.Contains(text, setupCodeFileName) {
		t.Fatalf("log should point to setup code file: %s", text)
	}
}

func TestAdminSetupInitializesAccessCode(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	blockedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(blockedRec, req)
	if blockedRec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", blockedRec.Code, blockedRec.Body.String())
	}

	body := `{"setup_code":"` + server.setupCode + `","access_code":"123456789012"}`
	setupReq := httptest.NewRequest(http.MethodPost, "http://example.com/admin/setup", strings.NewReader(body))
	setupRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(setupRec, setupReq)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup status = %d body = %s", setupRec.Code, setupRec.Body.String())
	}
	if server.hash == "" {
		t.Fatal("missing admin hash after setup")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), setupCodeFileName)); !os.IsNotExist(err) {
		t.Fatalf("setup code should be removed after setup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), secretFileName)); err != nil {
		t.Fatal(err)
	}
	cookie := adminSessionCookieFrom(t, setupRec)

	authedReq := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	authedReq.AddCookie(cookie)
	authedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(authedRec, authedReq)
	if authedRec.Code != http.StatusOK {
		t.Fatalf("authed status = %d body = %s", authedRec.Code, authedRec.Body.String())
	}
}

func TestAdminLoginIssuesSessionCookie(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	cookie := loginAdmin(t, server, "123456789012")
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/admin" {
		t.Fatalf("weak session cookie attributes: %#v", cookie)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/session", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"authenticated":true`) {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminLogoutClearsSession(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")

	logoutReq := httptest.NewRequest(http.MethodPost, "http://example.com/admin/logout", nil)
	logoutReq.AddCookie(cookie)
	logoutRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("logout status = %d body = %s", logoutRec.Code, logoutRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminLogsRequireAuth(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	buffer := logging.NewBuffer(10)
	server, err := NewServer(Options{ConfigPath: path, LogBuffer: buffer})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/logs", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminLogsReturnsBufferedEntries(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	buffer := logging.NewBuffer(10)
	buffer.Append(logging.Entry{Time: time.Unix(1, 0), Level: "INFO", Message: "first"})
	buffer.Append(logging.Entry{Time: time.Unix(2, 0), Level: "WARN", Message: "second", Attrs: map[string]any{"operation": "test"}})
	server, err := NewServer(Options{ConfigPath: path, LogBuffer: buffer})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/logs?limit=10&level=warn", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Logs []logging.Entry `json:"logs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Logs) != 1 || got.Logs[0].Message != "second" {
		t.Fatalf("logs = %#v", got.Logs)
	}
}

func TestAdminLogStreamReceivesLiveEntryAndCloses(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	buffer := logging.NewBuffer(10)
	server, err := NewServer(Options{ConfigPath: path, LogBuffer: buffer})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/admin/logs/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	done := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		var text strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- text.String()
				return
			}
			text.WriteString(line)
			if strings.Contains(text.String(), "stream-test") {
				done <- text.String()
				return
			}
		}
	}()
	buffer.Append(logging.Entry{Time: time.Now(), Level: "ERROR", Message: "stream-test"})

	select {
	case text := <-done:
		if !strings.Contains(text, "stream-test") {
			t.Fatalf("stream text = %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream entry")
	}
}

func TestAdminLogStreamStopsOnServerShutdown(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	buffer := logging.NewBuffer(10)
	server, err := NewServer(Options{ConfigPath: path, LogBuffer: buffer})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/admin/logs/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(resp.Body)
		done <- err
	}()
	server.Shutdown()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("read error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not stop after shutdown")
	}
}

func TestUpdateAdminAccessCodeChangesLoginSecret(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/access-code", strings.NewReader(`{"current_access_code":"123456789012","new_access_code":"abcdefghijkl"}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if verifyAdminCode(server.adminHash(), "abcdefghijkl") != nil {
		t.Fatal("new admin code does not verify")
	}
	if verifyAdminCode(server.adminHash(), "123456789012") == nil {
		t.Fatal("old admin code should no longer verify")
	}

	secretData, err := os.ReadFile(filepath.Join(filepath.Dir(path), secretFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(secretData), "abcdefghijkl") {
		t.Fatal("admin secret file contains plaintext access code")
	}
	loginAdmin(t, server, "abcdefghijkl")
}

func TestUpdateAdminAccessCodeRejectsMissingCurrentCode(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/access-code", strings.NewReader(`{"new_access_code":"abcdefghijkl"}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if verifyAdminCode(server.adminHash(), "123456789012") != nil {
		t.Fatal("current admin code should still verify")
	}
}

func TestUpdateAdminAccessCodeRejectsWrongCurrentCode(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/access-code", strings.NewReader(`{"current_access_code":"wrong-code","new_access_code":"abcdefghijkl"}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if verifyAdminCode(server.adminHash(), "123456789012") != nil {
		t.Fatal("current admin code should still verify")
	}
}

func TestUpdateAdminAccessCodeRateLimitsWrongCurrentCode(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")

	for i := 0; i < auth.DefaultFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/access-code", strings.NewReader(`{"current_access_code":"wrong-code","new_access_code":"abcdefghijkl"}`))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/access-code", strings.NewReader(`{"current_access_code":"wrong-code","new_access_code":"abcdefghijkl"}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminWriteRejectsCrossOrigin(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/config/validate", strings.NewReader(`{"backends":[],"subs":[]}`))
	req.Header.Set("Origin", "http://evil.example.com")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminWriteAllowsSameOriginReferer(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/config/validate", strings.NewReader(`{"backends":[{"id":"b1","server":"https://openlist.example.com"}],"subs":[]}`))
	req.Header.Set("Referer", "http://example.com/admin/")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminOriginUsesPublicBaseURL(t *testing.T) {
	path := writeAdminConfig(t, `{
  "public_base_url": "https://public.example.com",
  "backends": [
    {"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key":"old-secret"}
  ],
  "subs": []
}`)
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")

	req := httptest.NewRequest(http.MethodPost, "http://internal.example.com/admin/config/validate", strings.NewReader(`{"backends":[{"id":"b1","server":"https://openlist.example.com"}],"subs":[]}`))
	req.Header.Set("Origin", "https://public.example.com")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminSecureCookieUsesHTTPSPublicBaseURL(t *testing.T) {
	path := writeAdminConfig(t, `{
  "public_base_url": "https://public.example.com",
  "backends": [
    {"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key":"old-secret"}
  ],
  "subs": []
}`)
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://internal.example.com/admin/login", strings.NewReader(`{"access_code":"123456789012"}`))
	req.Header.Set("Origin", "https://public.example.com")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	cookie := adminSessionCookieFrom(t, rec)
	if !cookie.Secure {
		t.Fatalf("cookie should be secure when public_base_url is HTTPS: %#v", cookie)
	}
}

func TestAdminSecureCookieRequiresTrustedForwardedProto(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"123456789012"}`))
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	cookie := adminSessionCookieFrom(t, rec)
	if cookie.Secure {
		t.Fatalf("cookie should not trust X-Forwarded-Proto by default: %#v", cookie)
	}

	server.setTrustXForwardedFor(true)
	req = httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"123456789012"}`))
	req.Header.Set("X-Forwarded-Proto", "https")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	cookie = adminSessionCookieFrom(t, rec)
	if !cookie.Secure {
		t.Fatalf("cookie should be secure behind trusted HTTPS proxy: %#v", cookie)
	}
}

func TestAdminPrunesExpiredSessionsOnLogin(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	server.sessionMu.Lock()
	server.sessions["expired"] = adminSession{ExpiresAt: time.Now().Add(-time.Minute)}
	server.sessionMu.Unlock()

	loginAdmin(t, server, "123456789012")

	server.sessionMu.Lock()
	_, expiredExists := server.sessions["expired"]
	sessionCount := len(server.sessions)
	server.sessionMu.Unlock()
	if expiredExists || sessionCount != 1 {
		t.Fatalf("sessions = %#v expiredExists = %v count = %d", server.sessions, expiredExists, sessionCount)
	}
}

func TestAdminSessionsAreBounded(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	firstCookie := loginAdmin(t, server, "123456789012")
	var latestCookie *http.Cookie
	for range maxAdminSessions {
		latestCookie = loginAdmin(t, server, "123456789012")
	}

	server.sessionMu.Lock()
	sessionCount := len(server.sessions)
	server.sessionMu.Unlock()
	if sessionCount != maxAdminSessions {
		t.Fatalf("session count = %d, want %d", sessionCount, maxAdminSessions)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	req.AddCookie(firstCookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("oldest session status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	req.AddCookie(latestCookie)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("latest session status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminSetupCoolsDownAfterFailures(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < auth.DefaultFailureLimit; i++ {
		body := `{"setup_code":"wrong-code","access_code":"123456789012"}`
		req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/setup", strings.NewReader(body))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	body := `{"setup_code":"wrong-code","access_code":"123456789012"}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/setup", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestGetConfigRedactsSecrets(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "old-secret") {
		t.Fatalf("response leaked secret: %s", body)
	}
	if !strings.Contains(body, `"api_key_set":true`) {
		t.Fatalf("missing api_key_set: %s", body)
	}
}

func TestPutConfigKeepsAndReplacesSecrets(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	var saved *config.Config
	server, err := NewServer(Options{ConfigPath: path, OnSaved: func(cfg *config.Config) { saved = cfg }})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	body := `{
  "backends": [
    {"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key_action":"keep"},
    {"id":"b2","server":"https://two.example.com","auth_type":"api_key","api_key_action":"replace","api_key":"new-secret"}
  ],
  "subs": []
}`
	req := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backends[0].APIKey != "old-secret" || cfg.Backends[1].APIKey != "new-secret" {
		t.Fatalf("secrets = %#v", cfg.Backends)
	}
	if saved == nil || len(saved.Backends) != 2 {
		t.Fatalf("OnSaved not called with config: %#v", saved)
	}
	matches, err := filepath.Glob(path + ".bak*")
	if err != nil || len(matches) != 1 || matches[0] != path+".bak" {
		t.Fatalf("backup matches = %#v err = %v", matches, err)
	}
}

func TestPutConfigHashesSubscriptionAccessCode(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	body := `{
  "backends": [
    {"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key_action":"keep"}
  ],
  "subs": [
    {"id":"default","access_code_hash_action":"replace","access_code":"456789","mounts":[{"id":"media","backend":"b1","path":"/Media"}]}
  ]
}`
	req := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "456789") {
		t.Fatalf("saved config contains plaintext access code: %s", data)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.VerifyPassword(cfg.Subs[0].AccessCodeHash, "456789"); err != nil {
		t.Fatalf("saved access code hash does not verify: %v", err)
	}
}

func TestValidateConfigReturnsStructuredSubscriptionAccessCodeError(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	body := `{
  "backends": [
    {"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key_action":"keep"}
  ],
  "subs": [
    {"id":"sub1","access_code_hash_action":"replace","access_code":"12","mounts":[{"id":"media","backend":"b1","path":"/Media"}]}
  ]
}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/config/validate", strings.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["valid"] != false || got["error_code"] != "subscription.access_code.invalid" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	params, ok := got["error_params"].(map[string]any)
	if !ok || params["sub_id"] != "sub1" {
		t.Fatalf("missing sub_id params: %s", rec.Body.String())
	}
}

func TestPutConfigReturnsStructuredConfigValidationError(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	body := `{
  "backends": [
    {"id":"b1","server":"not-a-url","auth_type":"anonymous"}
  ],
  "subs": []
}`
	req := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error_code"] != "backend.server.invalid" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	params, ok := got["error_params"].(map[string]any)
	if !ok || params["backend_id"] != "b1" {
		t.Fatalf("missing backend_id params: %s", rec.Body.String())
	}
}

func TestPutConfigAllowsAsterisksInReplacedSecrets(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	body := `{
  "backends": [
    {"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key_action":"replace","api_key":"new*api*key"},
    {"id":"b2","server":"https://two.example.com","auth_type":"password","user":"admin","password_action":"replace","password":"new*password"}
  ],
  "subs": [
    {"id":"default","access_code_hash_action":"keep","mounts":[{"id":"media","backend":"b1","path":"/Media"}]}
  ]
}`
	req := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backends[0].APIKey != "new*api*key" {
		t.Fatalf("api_key = %q", cfg.Backends[0].APIKey)
	}
	if cfg.Backends[1].Password != "new*password" {
		t.Fatalf("password = %q", cfg.Backends[1].Password)
	}
}

func TestPutConfigReusesSingleBackupForConsecutiveSaves(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	first := `{"backends":[{"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key":"first-secret"}],"subs":[]}`
	second := `{"backends":[{"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key":"second-secret"}],"subs":[]}`

	for _, body := range []string{first, second} {
		req := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
		}
	}

	matches, err := filepath.Glob(path + ".bak*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0] != path+".bak" {
		t.Fatalf("backup matches = %#v", matches)
	}
	data, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "first-secret") || strings.Contains(string(data), "old-secret") {
		t.Fatalf("backup does not contain previous saved config: %s", data)
	}
}

func TestPutConfigAcceptsRedactedGetResponse(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", getRec.Code, getRec.Body.String())
	}

	putReq := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(getRec.Body.String()))
	putReq.AddCookie(cookie)
	putRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("put status = %d body = %s", putRec.Code, putRec.Body.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backends[0].APIKey != "old-secret" {
		t.Fatalf("api_key = %q", cfg.Backends[0].APIKey)
	}
}

func TestValidateConfigAcceptsRedactedGetResponse(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", getRec.Code, getRec.Body.String())
	}

	validateReq := httptest.NewRequest(http.MethodPost, "http://example.com/admin/config/validate", strings.NewReader(getRec.Body.String()))
	validateReq.AddCookie(cookie)
	validateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(validateRec, validateReq)

	var got map[string]any
	if err := json.Unmarshal(validateRec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if validateRec.Code != http.StatusOK || got["valid"] != true {
		t.Fatalf("status = %d body = %s", validateRec.Code, validateRec.Body.String())
	}
}

func TestPutConfigRejectsEnvBackedSecrets(t *testing.T) {
	t.Setenv("OPENLIST_TEST_API_KEY", "env-api-secret")
	t.Setenv("OPENLIST_TEST_PASSWORD", "env-password-secret")
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	body := `{
  "backends": [
    {"id":"api","server":"https://api.example.com","auth_type":"api_key","api_key_env":"OPENLIST_TEST_API_KEY"}
  ],
  "subs": [
    {"id":"default","mounts":[{"id":"media","backend":"api","path":"/Media"}]}
  ]
}`
	req := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid config json") {
		t.Fatalf("missing invalid json error: %s", rec.Body.String())
	}
}

func TestAdminRejectsMissingCode(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	hash, err := hashAdminCode("123456789012")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(envAdminCodeHash, hash)
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminServesEmbeddedAppWithoutSession(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `id="root"`) {
		t.Fatalf("missing app root: %s", rec.Body.String())
	}
}

func TestAdminServesEmbeddedAssetWithoutSession(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/assets/index.html", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "immutable") {
		t.Fatalf("cache-control = %q", cache)
	}
}

func TestAdminRejectsCodeQueryParameter(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta?code=123456789012", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminRejectsCodeHeader(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	req.Header.Set("X-Admin-Code", "123456789012")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminAuthCoolsDownAfterFailures(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < auth.DefaultFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"wrong-code"}`))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"wrong-code"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminAuthCooldownIgnoresForwardedForByDefault(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < auth.DefaultFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"wrong-code"}`))
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i+1))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"wrong-code"}`))
	req.Header.Set("X-Forwarded-For", "198.51.100.250")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminAuthCooldownCanTrustForwardedFor(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfigWithTrustXForwardedFor("old-secret", true))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < auth.DefaultFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"wrong-code"}`))
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i+1))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"wrong-code"}`))
	req.Header.Set("X-Forwarded-For", "198.51.100.250")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestValidateAllowsEmptySubs(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/config/validate", strings.NewReader(`{"backends":[{"id":"b1","server":"https://openlist.example.com"}],"subs":[]}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || got["valid"] != true {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestValidateRejectsAdminSubPath(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	body := `{
  "backends": [{"id":"b1","server":"https://openlist.example.com"}],
  "subs": [{"id":"admin","path":"/admin/config","mounts":[{"id":"m1","backend":"b1","path":"/"}]}]
}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/config/validate", strings.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || got["valid"] != false {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `reserved path prefix`) {
		t.Fatalf("missing reserved path error: %s", rec.Body.String())
	}
}

func TestBackendTestUsesSavedSecretWithoutLeakingIt(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/list" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "old-secret" {
			t.Fatalf("Authorization = %q, want saved secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"content":[]}}`))
	}))
	defer upstream.Close()

	path := writeAdminConfig(t, `{
  "backends": [
    {"id":"b1","server":"`+upstream.URL+`","auth_type":"api_key","api_key":"old-secret"}
  ],
  "subs": [
    {"id":"default","mounts":[{"id":"media","backend":"b1","path":"/Media"}]}
  ]
}`)
	hash, err := hashAdminCode("123456789012")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(envAdminCodeHash, hash)
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginAdmin(t, server, "123456789012")
	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/backend/test", strings.NewReader(`{"id":"b1","server":"`+upstream.URL+`","auth_type":"api_key","api_key_action":"keep","version":"v3"}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "old-secret") {
		t.Fatalf("response leaked secret: %s", rec.Body.String())
	}
}

func writeAdminConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func loginAdmin(t *testing.T, server *Server, code string) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/login", strings.NewReader(`{"access_code":"`+code+`"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", rec.Code, rec.Body.String())
	}
	return adminSessionCookieFrom(t, rec)
}

func adminSessionCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatalf("missing %s cookie in Set-Cookie: %v", sessionCookieName, rec.Result().Header.Values("Set-Cookie"))
	return nil
}

func testJSONConfig(apiKey string) string {
	return testJSONConfigWithTrustXForwardedFor(apiKey, false)
}

func testJSONConfigWithTrustXForwardedFor(apiKey string, trust bool) string {
	return `{
  "trust_x_forwarded_for": ` + strconv.FormatBool(trust) + `,
  "backends": [
    {"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key":"` + apiKey + `"}
  ],
  "subs": [
    {"id":"default","mounts":[{"id":"media","backend":"b1","path":"/Media"}]}
  ]
}`
}
