package admin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/config"
)

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
	req.Header.Set("X-Admin-Code", "123456789012")
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

	authedReq := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	authedReq.Header.Set("X-Admin-Code", "123456789012")
	authedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(authedRec, authedReq)
	if authedRec.Code != http.StatusOK {
		t.Fatalf("authed status = %d body = %s", authedRec.Code, authedRec.Body.String())
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
	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config", nil)
	req.Header.Set("X-Admin-Code", "123456789012")
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
	body := `{
  "backends": [
    {"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key_action":"keep"},
    {"id":"b2","server":"https://two.example.com","auth_type":"api_key","api_key_action":"replace","api_key":"new-secret"}
  ],
  "subs": []
}`
	req := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(body))
	req.Header.Set("X-Admin-Code", "123456789012")
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

func TestPutConfigAllowsAsterisksInReplacedSecrets(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
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
	req.Header.Set("X-Admin-Code", "123456789012")
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
	first := `{"backends":[{"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key":"first-secret"}],"subs":[]}`
	second := `{"backends":[{"id":"b1","server":"https://openlist.example.com","auth_type":"api_key","api_key":"second-secret"}],"subs":[]}`

	for _, body := range []string{first, second} {
		req := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(body))
		req.Header.Set("X-Admin-Code", "123456789012")
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
	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config", nil)
	getReq.Header.Set("X-Admin-Code", "123456789012")
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", getRec.Code, getRec.Body.String())
	}

	putReq := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(getRec.Body.String()))
	putReq.Header.Set("X-Admin-Code", "123456789012")
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
	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config", nil)
	getReq.Header.Set("X-Admin-Code", "123456789012")
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", getRec.Code, getRec.Body.String())
	}

	validateReq := httptest.NewRequest(http.MethodPost, "http://example.com/admin/config/validate", strings.NewReader(getRec.Body.String()))
	validateReq.Header.Set("X-Admin-Code", "123456789012")
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

func TestPutConfigKeepsEnvBackedSecretsWithoutPersistingResolvedValues(t *testing.T) {
	t.Setenv("OPENLIST_TEST_API_KEY", "env-api-secret")
	t.Setenv("OPENLIST_TEST_PASSWORD", "env-password-secret")
	path := writeAdminConfig(t, `{
  "backends": [
    {"id":"api","server":"https://api.example.com","auth_type":"api_key","api_key_env":"OPENLIST_TEST_API_KEY"},
    {"id":"pwd","server":"https://pwd.example.com","auth_type":"password","user":"admin","password_env":"OPENLIST_TEST_PASSWORD"}
  ],
  "subs": []
}`)
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	body := `{
  "backends": [
    {"id":"api","server":"https://api.example.com","auth_type":"api_key","api_key_env":"OPENLIST_TEST_API_KEY","api_key_action":"keep"},
    {"id":"pwd","server":"https://pwd.example.com","auth_type":"password","user":"admin","password_env":"OPENLIST_TEST_PASSWORD","password_action":"keep"}
  ],
  "subs": []
}`
	req := httptest.NewRequest(http.MethodPut, "http://example.com/admin/config", strings.NewReader(body))
	req.Header.Set("X-Admin-Code", "123456789012")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "env-api-secret") || strings.Contains(text, "env-password-secret") {
		t.Fatalf("persisted resolved env secret: %s", text)
	}
	if !strings.Contains(text, `"api_key_env": "OPENLIST_TEST_API_KEY"`) || !strings.Contains(text, `"password_env": "OPENLIST_TEST_PASSWORD"`) {
		t.Fatalf("env references were not preserved: %s", text)
	}
	if strings.Contains(text, `"api_key": ""`) || strings.Contains(text, `"password": ""`) {
		t.Fatalf("persisted empty secret field next to env reference: %s", text)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("saved env-backed config did not reload: %v", err)
	}
	if cfg.Backends[0].APIKey != "env-api-secret" || cfg.Backends[1].Password != "env-password-secret" {
		t.Fatalf("saved env-backed secrets did not resolve on reload: %+v", cfg.Backends)
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

func TestAdminAuthCoolsDownAfterFailures(t *testing.T) {
	path := writeAdminConfig(t, testJSONConfig("old-secret"))
	t.Setenv(envAdminCode, "123456789012")
	server, err := NewServer(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < auth.DefaultFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
		req.Header.Set("X-Admin-Code", "wrong-code")
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	req.Header.Set("X-Admin-Code", "wrong-code")
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
		req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
		req.Header.Set("X-Admin-Code", "wrong-code")
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i+1))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	req.Header.Set("X-Admin-Code", "wrong-code")
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
		req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
		req.Header.Set("X-Admin-Code", "wrong-code")
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i+1))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body = %s", i+1, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/config/meta", nil)
	req.Header.Set("X-Admin-Code", "wrong-code")
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
	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/config/validate", strings.NewReader(`{"backends":[{"id":"b1","server":"https://openlist.example.com"}],"subs":[]}`))
	req.Header.Set("X-Admin-Code", "123456789012")
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
	body := `{
  "backends": [{"id":"b1","server":"https://openlist.example.com"}],
  "subs": [{"id":"admin","path":"/admin/config","mounts":[{"id":"m1","backend":"b1","path":"/"}]}]
}`
	req := httptest.NewRequest(http.MethodPost, "http://example.com/admin/config/validate", strings.NewReader(body))
	req.Header.Set("X-Admin-Code", "123456789012")
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

func writeAdminConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
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
