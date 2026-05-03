package auth

import (
	"net"
	"net/http"
	"strings"
)

func ClientHost(r *http.Request, trustXForwardedFor bool) string {
	host := r.RemoteAddr
	if trustXForwardedFor {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			host = strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if host == "" {
		host = r.RemoteAddr
	}
	return host
}
