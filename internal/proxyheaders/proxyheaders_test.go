package proxyheaders

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestFirstValue(t *testing.T) {
	if got := FirstValue(" https, http "); got != "https" {
		t.Fatalf("FirstValue = %q", got)
	}
}

func TestSchemeUsesTLSByDefault(t *testing.T) {
	req := httptest.NewRequest("GET", "https://internal.example.com", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("X-Forwarded-Proto", "http")

	if got := Scheme(req, false); got != "https" {
		t.Fatalf("Scheme = %q", got)
	}
}

func TestSchemeUsesTrustedForwardedProtoFirstValue(t *testing.T) {
	req := httptest.NewRequest("GET", "http://internal.example.com", nil)
	req.Header.Set("X-Forwarded-Proto", "HTTPS, http")

	if got := Scheme(req, true); got != "https" {
		t.Fatalf("Scheme = %q", got)
	}
	if !IsHTTPS(req, true) {
		t.Fatal("IsHTTPS = false")
	}
}

func TestSchemeIgnoresUntrustedAndInvalidForwardedProto(t *testing.T) {
	req := httptest.NewRequest("GET", "http://internal.example.com", nil)
	req.Header.Set("X-Forwarded-Proto", "ftp")

	if got := Scheme(req, false); got != "http" {
		t.Fatalf("untrusted Scheme = %q", got)
	}
	if got := Scheme(req, true); got != "http" {
		t.Fatalf("invalid Scheme = %q", got)
	}
}

func TestHostUsesTrustedForwardedHostFirstValue(t *testing.T) {
	req := httptest.NewRequest("GET", "http://internal.example.com", nil)
	req.Header.Set("X-Forwarded-Host", " public.example.com, internal.example.com ")

	if got := Host(req, false); got != "internal.example.com" {
		t.Fatalf("untrusted Host = %q", got)
	}
	if got := Host(req, true); got != "public.example.com" {
		t.Fatalf("trusted Host = %q", got)
	}
}
