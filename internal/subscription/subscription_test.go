package subscription

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"openlist-tvbox/internal/config"
)

func TestBuildSubscriptionDoesNotLeakBackendSecrets(t *testing.T) {
	cfg := &config.Config{
		PublicBaseURL: "http://gateway.example.com",
		Backends:      []config.Backend{{ID: "private_backend", Server: "https://openlist.example.com", AuthType: "api_key", APIKey: "secret-token"}},
		Subs:          []config.Subscription{{Mounts: []config.Mount{{ID: "movies", Backend: "private_backend", Path: "/Movies"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := BuildForSub(cfg, cfg.Subs[0], httptest.NewRequest("GET", "http://ignored/sub", nil))
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"secret-token", "openlist.example.com", "private_backend"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("subscription leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "http://gateway.example.com/spider/openlist-tvbox.v2.js") {
		t.Fatalf("subscription missing spider url: %s", text)
	}
	var ext map[string]string
	if err := json.Unmarshal([]byte(got.Sites[0].Ext), &ext); err != nil {
		t.Fatalf("site ext is not json: %v", err)
	}
	if ext["gateway"] != "http://gateway.example.com/s/default" {
		t.Fatalf("site ext gateway = %q", ext["gateway"])
	}
	if !strings.HasPrefix(ext["skey"], "openlist_tvbox_default_u") {
		t.Fatalf("site ext skey = %q", ext["skey"])
	}
}

func TestBuildForSubUsesSubTVBoxAndScopedGateway(t *testing.T) {
	cfg := &config.Config{
		PublicBaseURL: "http://gateway.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs: []config.Subscription{{
			ID:       "movies",
			Path:     "/sub/movies",
			SiteKey:  "movies_key",
			SiteName: "Movies",
			Mounts:   []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := BuildForSub(cfg, cfg.Subs[0], httptest.NewRequest("GET", "http://ignored/sub/movies", nil))
	if len(got.Sites) != 1 {
		t.Fatalf("sites = %#v", got.Sites)
	}
	site := got.Sites[0]
	if !strings.HasPrefix(site.Key, "movies_key_u") || site.Name != "Movies" {
		t.Fatalf("site = %#v", site)
	}
	if !strings.Contains(site.Ext, "http://gateway.example.com/s/movies") {
		t.Fatalf("site ext = %q", site.Ext)
	}
	var ext map[string]string
	if err := json.Unmarshal([]byte(site.Ext), &ext); err != nil {
		t.Fatalf("site ext is not json: %v", err)
	}
	if ext["gateway"] != "http://gateway.example.com/s/movies" || !strings.HasPrefix(ext["skey"], "openlist_tvbox_movies_u") {
		t.Fatalf("site ext = %#v", ext)
	}
}

func TestBuildAlwaysEmitsFilterable(t *testing.T) {
	cfg := &config.Config{
		PublicBaseURL: "http://gateway.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs:          []config.Subscription{{Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := BuildForSub(cfg, cfg.Subs[0], httptest.NewRequest("GET", "http://ignored/sub", nil))
	if got.Sites[0].Filterable != 1 {
		t.Fatalf("filterable = %d, want 1", got.Sites[0].Filterable)
	}
}

func TestBuildForSubUsesDistinctSiteAndStorageKeys(t *testing.T) {
	cfg := &config.Config{
		PublicBaseURL: "http://gateway.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		TVBox:         config.TVBox{SiteKey: "shared_key"},
		Subs: []config.Subscription{
			{ID: "movies", Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}}},
			{ID: "shows", Mounts: []config.Mount{{ID: "s", Backend: "b1", Path: "/Shows"}}},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}

	movies := BuildForSub(cfg, cfg.Subs[0], httptest.NewRequest("GET", "http://ignored/sub/movies", nil))
	shows := BuildForSub(cfg, cfg.Subs[1], httptest.NewRequest("GET", "http://ignored/sub/shows", nil))
	var movieExt, showExt map[string]string
	if err := json.Unmarshal([]byte(movies.Sites[0].Ext), &movieExt); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(shows.Sites[0].Ext), &showExt); err != nil {
		t.Fatal(err)
	}
	if movieExt["skey"] == "" || showExt["skey"] == "" || movieExt["skey"] == showExt["skey"] {
		t.Fatalf("storage keys are not distinct: movies=%q shows=%q", movieExt["skey"], showExt["skey"])
	}
	if movies.Sites[0].Key == "" || shows.Sites[0].Key == "" || movies.Sites[0].Key == shows.Sites[0].Key {
		t.Fatalf("site keys are not distinct: movies=%q shows=%q", movies.Sites[0].Key, shows.Sites[0].Key)
	}
}

func TestBuildForSubScopesIdentityKeysByBaseURL(t *testing.T) {
	cfgA := &config.Config{
		PublicBaseURL: "http://gateway-a.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs:          []config.Subscription{{ID: "movies", SiteKey: "same_key", Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}}}},
	}
	cfgB := &config.Config{
		PublicBaseURL: "http://gateway-b.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs:          []config.Subscription{{ID: "movies", SiteKey: "same_key", Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}}}},
	}
	if err := cfgA.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := cfgB.Validate(); err != nil {
		t.Fatal(err)
	}

	subA := BuildForSub(cfgA, cfgA.Subs[0], httptest.NewRequest("GET", "http://ignored/sub/movies", nil))
	subB := BuildForSub(cfgB, cfgB.Subs[0], httptest.NewRequest("GET", "http://ignored/sub/movies", nil))
	if subA.Sites[0].Key == subB.Sites[0].Key {
		t.Fatalf("site keys collide across hosts: %q", subA.Sites[0].Key)
	}
	var extA, extB map[string]string
	if err := json.Unmarshal([]byte(subA.Sites[0].Ext), &extA); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(subB.Sites[0].Ext), &extB); err != nil {
		t.Fatal(err)
	}
	if extA["skey"] == extB["skey"] {
		t.Fatalf("storage keys collide across hosts: %q", extA["skey"])
	}
}

func TestScopedIdentityKeyIncludesFullBaseURLScope(t *testing.T) {
	got := scopedIdentityKey("openlist_tvbox_movies", "HTTPS://Gateway.Example.Com/")
	const prefix = "openlist_tvbox_movies_u"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("scoped key = %q", got)
	}
	scope, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(got, prefix))
	if err != nil {
		t.Fatal(err)
	}
	if string(scope) != "https://gateway.example.com" {
		t.Fatalf("scope = %q", scope)
	}
}

func TestBuildDefaultsQuickSearchOff(t *testing.T) {
	cfg := &config.Config{
		PublicBaseURL: "http://gateway.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs:          []config.Subscription{{Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := BuildForSub(cfg, cfg.Subs[0], httptest.NewRequest("GET", "http://ignored/sub", nil))
	if got.Sites[0].QuickSearch != 0 {
		t.Fatalf("quickSearch = %d, want 0", got.Sites[0].QuickSearch)
	}
}

func TestBuildOmitsEmptyLives(t *testing.T) {
	cfg := &config.Config{
		PublicBaseURL: "http://gateway.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs:          []config.Subscription{{Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := BuildForSub(cfg, cfg.Subs[0], httptest.NewRequest("GET", "http://ignored/sub", nil))
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"lives":[]`) || strings.Contains(string(data), `"lives"`) {
		t.Fatalf("empty lives should be omitted: %s", data)
	}
}

func TestBuildEmitsConfiguredLives(t *testing.T) {
	cfg := &config.Config{
		PublicBaseURL: "http://gateway.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs: []config.Subscription{{
			Lives:  []config.Live{{Name: "Live", URL: "https://live.example.com/iptv.m3u", PlayerType: 2, UA: "Secret-UA-Token"}},
			Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := BuildForSub(cfg, cfg.Subs[0], httptest.NewRequest("GET", "http://ignored/sub", nil))
	if len(got.Lives) != 1 {
		t.Fatalf("lives = %#v", got.Lives)
	}
	if got.Lives[0].Name != "Live" || got.Lives[0].Type != 0 || got.Lives[0].URL != "http://gateway.example.com/s/default/live/0/iptv.m3u" || got.Lives[0].PlayerType != 2 {
		t.Fatalf("live = %#v", got.Lives[0])
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"playerType":2`) || strings.Contains(text, "player_type") {
		t.Fatalf("live json uses unexpected field names: %s", text)
	}
	if strings.Contains(text, "live.example.com") {
		t.Fatalf("subscription leaked live source URL: %s", text)
	}
	if strings.Contains(text, "Secret-UA-Token") || strings.Contains(text, `"ua"`) {
		t.Fatalf("subscription leaked live source user-agent: %s", text)
	}
}

func TestBuildLiveURLDoesNotLeakSourceQuery(t *testing.T) {
	cfg := &config.Config{
		PublicBaseURL: "http://gateway.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs: []config.Subscription{{
			Lives: []config.Live{{
				Name: "Live",
				URL:  "https://live.example.com/path/iptv list.m3u?token=secret#frag",
			}},
			Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := BuildForSub(cfg, cfg.Subs[0], httptest.NewRequest("GET", "http://ignored/sub", nil))
	if len(got.Lives) != 1 {
		t.Fatalf("lives = %#v", got.Lives)
	}
	if got.Lives[0].URL != "http://gateway.example.com/s/default/live/0/iptv%20list.m3u" {
		t.Fatalf("live url = %q", got.Lives[0].URL)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"live.example.com", "token=secret", "#frag"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("subscription leaked %q: %s", forbidden, text)
		}
	}
}

func TestBuildUsesInjectedSpiderFingerprint(t *testing.T) {
	original := SpiderFingerprint
	SpiderFingerprint = "build.abc123"
	t.Cleanup(func() { SpiderFingerprint = original })

	cfg := &config.Config{
		PublicBaseURL: "http://gateway.example.com",
		Backends:      []config.Backend{{ID: "b1", Server: "https://openlist.example.com"}},
		Subs:          []config.Subscription{{Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/Movies"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := BuildForSub(cfg, cfg.Subs[0], httptest.NewRequest("GET", "http://ignored/sub", nil))
	if got.Sites[0].API != "http://gateway.example.com/spider/openlist-tvbox.build.abc123.js" {
		t.Fatalf("api = %q", got.Sites[0].API)
	}
}

func TestBaseURLUsesForwardedHeaders(t *testing.T) {
	cfg := &config.Config{}
	req := httptest.NewRequest("GET", "http://internal/sub", nil)
	req.Host = "internal"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "public.example.com")
	if got := BaseURL(cfg, req); got != "https://public.example.com" {
		t.Fatalf("BaseURL = %q", got)
	}
}
