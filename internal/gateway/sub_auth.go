package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"openlist-tvbox/internal/auth"
	"openlist-tvbox/internal/catvod"
	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/mount"
)

func (s *Server) authSub(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sub/")
	subID, action, ok := strings.Cut(path, "/")
	if !ok || action != "auth" {
		http.NotFound(w, r)
		return
	}
	s.authSubID(serviceFromRequest(r), w, r, subID)
}

func (s *Server) authSubID(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	key := s.authFailureKey(r, subID)
	if s.authLimiter.Blocked(key) {
		s.logSubAuthFailure("sub auth throttled", subID, r, "too_many_attempts")
		writeJSON(w, http.StatusTooManyRequests, map[string]bool{"ok": false})
		return
	}
	code := accessCodeFromRequest(r)
	if s.validCode(service, subID, code) {
		s.authLimiter.Clear(key)
		sub, _ := s.subByID(service, subID)
		token, expiresAt := s.issueAccessToken(sub)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "access_token": token, "expires_at": expiresAt})
		return
	}
	if code != "" {
		s.authLimiter.RecordFailure(key)
		s.logSubAuthFailure("sub auth failed", subID, r, "invalid_code")
	}
	writeJSON(w, http.StatusUnauthorized, map[string]bool{"ok": false})
}

func (s *Server) authorize(service *mount.Service, w http.ResponseWriter, r *http.Request, subID string) bool {
	sub, ok := s.subByID(service, subID)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, catvod.Result{Error: "unauthorized"})
		return false
	}
	if sub.AccessCodeHash == "" {
		return true
	}
	if s.validAccessToken(sub, accessTokenFromRequest(r)) {
		return true
	}
	key := s.authFailureKey(r, sub.ID)
	if s.authLimiter.Blocked(key) {
		s.logSubAuthFailure("sub api auth throttled", sub.ID, r, "too_many_attempts")
		writeJSON(w, http.StatusTooManyRequests, catvod.Result{Error: "too many failed access code attempts"})
		return false
	}
	s.logSubAuthFailure("sub api unauthorized", sub.ID, r, "missing_or_invalid_token")
	writeJSON(w, http.StatusUnauthorized, catvod.Result{Error: "unauthorized"})
	return false
}

func (s *Server) validCode(service *mount.Service, subID, code string) bool {
	sub, ok := s.subByID(service, subID)
	if !ok || sub.AccessCodeHash == "" || code == "" {
		return false
	}
	if auth.ValidateAccessCode(code) != nil {
		return false
	}
	return auth.VerifyPassword(sub.AccessCodeHash, code) == nil
}

func (s *Server) authFailureKey(r *http.Request, subID string) string {
	return subID + "|" + auth.ClientHost(r, serviceFromRequest(r).Config().TrustXForwardedFor)
}

func (s *Server) subByID(service *mount.Service, subID string) (config.Subscription, bool) {
	for _, sub := range service.Config().Subs {
		if sub.ID == subID {
			return sub, true
		}
	}
	return config.Subscription{}, false
}

func accessCodeFromRequest(r *http.Request) string {
	var body struct {
		Code string `json:"code"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body)
	}
	return body.Code
}

func accessTokenFromRequest(r *http.Request) string {
	return r.Header.Get("X-Access-Token")
}

func (s *Server) issueAccessToken(sub config.Subscription) (string, int64) {
	expiresAt := time.Now().Add(accessTokenTTL).Unix()
	fingerprint := accessHashFingerprint(sub.AccessCodeHash)
	payload := fmt.Sprintf("%s.%d.%s", sub.ID, expiresAt, fingerprint)
	signature := s.signAccessTokenPayload(payload)
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "." + signature)), expiresAt
}

func (s *Server) validAccessToken(sub config.Subscription, token string) bool {
	if token == "" {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 4 {
		return false
	}
	if parts[0] != sub.ID {
		return false
	}
	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() >= expiresAt {
		return false
	}
	if parts[2] != accessHashFingerprint(sub.AccessCodeHash) {
		return false
	}
	payload := strings.Join(parts[:3], ".")
	want := s.signAccessTokenPayload(payload)
	return hmac.Equal([]byte(parts[3]), []byte(want))
}

func (s *Server) signAccessTokenPayload(payload string) string {
	mac := hmac.New(sha256.New, s.tokenSecret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func accessHashFingerprint(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
