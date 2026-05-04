package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"openlist-tvbox/internal/catvod"
	"openlist-tvbox/internal/mount"
)

func (s *Server) liveForSub(service *mount.Service, w http.ResponseWriter, r *http.Request, subID, livePath string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	liveIndex, _, _ := strings.Cut(livePath, "/")
	index, err := strconv.Atoi(liveIndex)
	if err != nil || index < 0 {
		http.NotFound(w, r)
		return
	}
	sub, ok := s.subByID(service, subID)
	if !ok || index >= len(sub.Lives) {
		http.NotFound(w, r)
		return
	}
	live := sub.Lives[index]
	ctx, cancel := context.WithTimeout(r.Context(), liveProxyTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, live.URL, nil)
	if err != nil {
		s.logLiveProxyFailure(subID, index, "build_request", "invalid configured url")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: invalid configured url"})
		return
	}
	if live.UA != "" {
		req.Header.Set("User-Agent", live.UA)
	} else if ua := r.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logLiveProxyFailure(subID, index, "request", "request failed")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: request failed"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logLiveProxyFailure(subID, index, "status", "upstream returned non-success status")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: fmt.Sprintf("live source request failed: upstream status %d", resp.StatusCode)})
		return
	}
	if resp.ContentLength > maxLivePlaylistBodySize {
		s.logLiveProxyFailure(subID, index, "size", "playlist too large")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: playlist too large"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLivePlaylistBodySize+1))
	if err != nil {
		s.logLiveProxyFailure(subID, index, "read", "read failed")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: read failed"})
		return
	}
	if len(body) > maxLivePlaylistBodySize {
		s.logLiveProxyFailure(subID, index, "size", "playlist too large")
		writeJSON(w, http.StatusBadGateway, catvod.Result{Error: "live source request failed: playlist too large"})
		return
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if cacheControl := resp.Header.Get("Cache-Control"); cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) logLiveProxyFailure(subID string, index int, stage, reason string) {
	if s.logger == nil {
		return
	}
	s.logger.Warn("live source request failed", "sub", subID, "index", index, "stage", stage, "reason", reason)
}
