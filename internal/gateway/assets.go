package gateway

import (
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) spider(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/spider/openlist-tvbox.js" &&
		(!strings.HasPrefix(r.URL.Path, "/spider/openlist-tvbox.") || !strings.HasSuffix(r.URL.Path, ".js")) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(spiderJS)
}

func (s *Server) icon(w http.ResponseWriter, r *http.Request) {
	var data []byte
	contentType := "image/png"
	switch r.URL.Path {
	case "/assets/icons/folder.png":
		data = folderIcon
	case "/assets/icons/logo.svg":
		data = logoIcon
		contentType = "image/svg+xml; charset=utf-8"
	case "/assets/icons/video.png":
		data = videoIcon
	case "/assets/icons/audio.png":
		data = audioIcon
	case "/assets/icons/file.png":
		data = fileIcon
	case "/assets/icons/playlist.png":
		data = playlistIcon
	case "/assets/icons/refresh.png":
		data = refreshIcon
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
