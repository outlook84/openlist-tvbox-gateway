package mount

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

func encodePlayToken(token playToken) string {
	raw, err := json.Marshal(token)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodePlayToken(value string) (playToken, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return playToken{}, false
	}
	var token playToken
	if json.Unmarshal(raw, &token) != nil || token.ID == "" {
		return playToken{}, false
	}
	return token, true
}

func playItemName(name string) string {
	name = strings.ReplaceAll(name, "#", "＃")
	return strings.ReplaceAll(name, "$", "＄")
}

func playHeader(raw string, mountHeaders map[string]string) map[string]string {
	headers := map[string]string{}
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		host := strings.ToLower(u.Host)
		if strings.Contains(host, "115") {
			headers["User-Agent"] = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124 Safari/537.36"
		}
		if strings.Contains(host, "baidupcs.com") {
			headers["User-Agent"] = "pan.baidu.com"
		}
	}
	for name, value := range mountHeaders {
		headers[name] = value
	}
	return headers
}
