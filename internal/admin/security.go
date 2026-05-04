package admin

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/proxyheaders"
)

func (s *Server) adminFailureKey(r *http.Request) string {
	return "admin|" + auth.ClientHost(r, s.trustForwardedHeaders())
}

func (s *Server) setupFailureKey(r *http.Request) string {
	return "setup|" + auth.ClientHost(r, s.trustForwardedHeaders())
}

func (s *Server) logAuthFailure(message string, r *http.Request, reason string) {
	if s.logger == nil {
		return
	}
	s.logger.Warn(message, "client", auth.ClientHost(r, s.trustForwardedHeaders()), "reason", reason)
}
func adminURL(listen string) string {
	host := listen
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	return "http://" + host + "/admin"
}

func (s *Server) issueSessionCookie(w http.ResponseWriter, r *http.Request) error {
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	now := time.Now()
	expires := now.Add(sessionTTL)
	s.sessionMu.Lock()
	s.pruneExpiredSessionsLocked(now)
	s.pruneOldestSessionsLocked(maxAdminSessions - 1)
	s.sessions[token] = adminSession{CreatedAt: now, ExpiresAt: expires}
	s.sessionMu.Unlock()
	http.SetCookie(w, s.sessionCookie(r, token, expires, int(sessionTTL.Seconds())))
	return nil
}

func (s *Server) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	now := time.Now()
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	session, ok := s.sessions[cookie.Value]
	if !ok {
		return false
	}
	if !session.ExpiresAt.After(now) {
		delete(s.sessions, cookie.Value)
		return false
	}
	return true
}

func (s *Server) pruneExpiredSessionsLocked(now time.Time) {
	for token, session := range s.sessions {
		if !session.ExpiresAt.After(now) {
			delete(s.sessions, token)
		}
	}
}

func (s *Server) pruneOldestSessionsLocked(max int) {
	for len(s.sessions) > max {
		var oldestToken string
		var oldestCreatedAt time.Time
		for token, session := range s.sessions {
			if oldestToken == "" || session.CreatedAt.Before(oldestCreatedAt) {
				oldestToken = token
				oldestCreatedAt = session.CreatedAt
			}
		}
		delete(s.sessions, oldestToken)
	}
}

func randomToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, s.sessionCookie(r, "", time.Unix(0, 0), -1))
}

func (s *Server) sessionCookie(r *http.Request, value string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/admin",
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.sessionCookieSecure(r),
		SameSite: http.SameSiteStrictMode,
	}
}

func (s *Server) sessionCookieSecure(r *http.Request) bool {
	if baseURL := s.getPublicBaseURL(); baseURL != "" {
		u, err := url.Parse(baseURL)
		if err == nil && strings.EqualFold(u.Scheme, "https") {
			return true
		}
	}
	return s.requestIsHTTPS(r)
}

func (s *Server) requestIsHTTPS(r *http.Request) bool {
	return proxyheaders.IsHTTPS(r, s.trustForwardedHeaders())
}

func (s *Server) sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		return s.originMatchesRequest(r, origin)
	}
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer != "" {
		return s.originMatchesRequest(r, referer)
	}
	return true
}

func (s *Server) originMatchesRequest(r *http.Request, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	for _, baseURL := range s.adminOriginBaseURLs(r) {
		want, err := url.Parse(baseURL)
		if err != nil || want.Scheme == "" || want.Host == "" {
			continue
		}
		if strings.EqualFold(u.Scheme, want.Scheme) && strings.EqualFold(u.Host, want.Host) {
			return true
		}
	}
	return false
}

func (s *Server) adminBaseURL(r *http.Request) string {
	baseURLs := s.adminOriginBaseURLs(r)
	if len(baseURLs) > 0 {
		return baseURLs[0]
	}
	return "http://localhost"
}

func (s *Server) adminOriginBaseURLs(r *http.Request) []string {
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	scheme := proxyheaders.Scheme(r, s.trustForwardedHeaders())
	baseURLs := []string{scheme + "://" + host}
	if baseURL := s.getPublicBaseURL(); baseURL != "" {
		baseURLs = append(baseURLs, baseURL)
	}
	if s.trustForwardedHeaders() {
		forwardedHost := proxyheaders.Host(r, true)
		if forwardedHost != "" && forwardedHost != host {
			baseURLs = append(baseURLs, proxyheaders.Scheme(r, true)+"://"+forwardedHost)
		}
	}
	return baseURLs
}
