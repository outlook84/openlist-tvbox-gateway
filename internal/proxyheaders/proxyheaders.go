package proxyheaders

import (
	"net/http"
	"strings"
)

// FirstValue returns the left-most value from a comma-separated forwarded header.
func FirstValue(value string) string {
	return strings.TrimSpace(strings.Split(value, ",")[0])
}

// Scheme returns the request scheme, optionally using trusted X-Forwarded-Proto.
func Scheme(r *http.Request, trustForwardedHeaders bool) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if trustForwardedHeaders {
		forwarded := FirstValue(r.Header.Get("X-Forwarded-Proto"))
		if strings.EqualFold(forwarded, "http") {
			scheme = "http"
		}
		if strings.EqualFold(forwarded, "https") {
			scheme = "https"
		}
	}
	return scheme
}

// IsHTTPS reports whether the request should be treated as HTTPS.
func IsHTTPS(r *http.Request, trustForwardedHeaders bool) bool {
	return Scheme(r, trustForwardedHeaders) == "https"
}

// Host returns the request host, optionally using trusted X-Forwarded-Host.
func Host(r *http.Request, trustForwardedHeaders bool) string {
	host := strings.TrimSpace(r.Host)
	if trustForwardedHeaders {
		if forwarded := FirstValue(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
			host = forwarded
		}
	}
	return host
}
