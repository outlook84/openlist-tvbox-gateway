package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"openlist-tvbox/internal/logging"
)

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	limit := parseLogLimit(r.URL.Query().Get("limit"), 300, 1000)
	level := parseLogLevel(r.URL.Query().Get("level"))
	entries := []logging.Entry{}
	if s.logBuffer != nil {
		entries = s.logBuffer.Snapshot(limit, level)
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": entries})
}

func (s *Server) streamLogs(w http.ResponseWriter, r *http.Request) {
	if s.logBuffer == nil {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAdminError(w, http.StatusInternalServerError, "logs.stream_unsupported", "log streaming is not supported", nil)
		return
	}
	level := parseLogLevel(r.URL.Query().Get("level"))
	entries, unsubscribe := s.logBuffer.Subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-s.shutdown:
			return
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": heartbeat\n\n")
			flusher.Flush()
		case entry, ok := <-entries:
			if !ok {
				return
			}
			if logEntryLevel(entry) < level {
				continue
			}
			if err := writeLogSSE(w, entry); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
func parseLogLimit(value string, fallback, max int) int {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit <= 0 {
		return fallback
	}
	if limit > max {
		return max
	}
	return limit
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func logEntryLevel(entry logging.Entry) slog.Level {
	switch entry.Level {
	case slog.LevelDebug.String():
		return slog.LevelDebug
	case slog.LevelWarn.String():
		return slog.LevelWarn
	case slog.LevelError.String():
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func writeLogSSE(w io.Writer, entry logging.Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, "event: log\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "data: "); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n\n")
	return err
}
