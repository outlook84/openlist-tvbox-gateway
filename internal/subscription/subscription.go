package subscription

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/proxyheaders"
)

var SpiderFingerprint = "v2"

var invalidSpiderFingerprintChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func SpiderPath() string {
	fingerprint := strings.Trim(invalidSpiderFingerprintChars.ReplaceAllString(SpiderFingerprint, "-"), ".-_")
	if fingerprint == "" {
		fingerprint = "dev"
	}
	return "/spider/openlist-tvbox." + fingerprint + ".js"
}

type Config struct {
	Sites  []Site        `json:"sites"`
	Parses []any         `json:"parses"`
	Lives  []config.Live `json:"lives,omitempty"`
}

type Site struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Type        int    `json:"type"`
	API         string `json:"api"`
	Ext         string `json:"ext"`
	Searchable  int    `json:"searchable"`
	QuickSearch int    `json:"quickSearch"`
	Filterable  int    `json:"filterable"`
	Changeable  int    `json:"changeable"`
	Timeout     int    `json:"timeout"`
}

type siteExt struct {
	Gateway string `json:"gateway"`
	SKey    string `json:"skey"`
}

func BuildForSub(cfg *config.Config, sub config.Subscription, r *http.Request) Config {
	base := BaseURL(cfg, r)
	gateway := base + "/s/" + sub.ID
	siteKey := scopedIdentityKey(sub.TVBox.SiteKey, base)
	storageKey := scopedIdentityKey("openlist_tvbox_"+sub.ID, base)
	ext, _ := json.Marshal(siteExt{
		Gateway: gateway,
		SKey:    storageKey,
	})
	out := Config{
		Sites: []Site{{
			Key:         siteKey,
			Name:        sub.TVBox.SiteName,
			Type:        3,
			API:         base + SpiderPath(),
			Ext:         string(ext),
			Searchable:  valueOrDefault(sub.TVBox.Searchable, 1),
			QuickSearch: valueOrDefault(sub.TVBox.QuickSearch, 0),
			Filterable:  1,
			Changeable:  valueOrDefault(sub.TVBox.Changeable, 0),
			Timeout:     sub.TVBox.Timeout,
		}},
		Parses: []any{},
	}
	if len(sub.Lives) > 0 {
		out.Lives = buildLivesForSub(sub.Lives, gateway)
	}
	return out
}

func buildLivesForSub(lives []config.Live, gateway string) []config.Live {
	out := make([]config.Live, len(lives))
	for i, live := range lives {
		out[i] = live
		out[i].URL = gateway + "/live/" + strconv.Itoa(i) + "/" + liveProxyName(live.URL)
		out[i].UA = ""
	}
	return out
}

func liveProxyName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "live.m3u"
	}
	name := path.Base(u.EscapedPath())
	if name == "." || name == "/" {
		return "live.m3u"
	}
	unescaped, err := url.PathUnescape(name)
	if err != nil || unescaped == "" {
		return "live.m3u"
	}
	return url.PathEscape(unescaped)
}

func scopedIdentityKey(key, base string) string {
	scope := strings.ToLower(strings.TrimRight(base, "/"))
	return key + "_u" + base64.RawURLEncoding.EncodeToString([]byte(scope))
}

func BaseURL(cfg *config.Config, r *http.Request) string {
	if cfg.PublicBaseURL != "" {
		return cfg.PublicBaseURL
	}
	scheme := proxyheaders.Scheme(r, cfg.TrustForwardedHeaders)
	host := proxyheaders.Host(r, cfg.TrustForwardedHeaders)
	return strings.TrimRight(scheme+"://"+strings.TrimSpace(host), "/")
}

func valueOrDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
