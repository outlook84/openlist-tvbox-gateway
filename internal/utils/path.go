package utils

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

func Join(root, rel string) (string, error) {
	rel, err := CleanRelative(rel)
	if err != nil {
		return "", err
	}
	if root == "" {
		root = "/"
	}
	joined := path.Clean("/" + strings.Trim(root, "/") + "/" + rel)
	if joined == "." {
		return "/", nil
	}
	return joined, nil
}

func CleanRelative(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/" {
		return "", nil
	}
	if strings.Contains(raw, "\x00") || strings.Contains(raw, "\\") {
		return "", errors.New("invalid path")
	}
	if strings.Contains(raw, "://") || strings.HasPrefix(raw, "//") {
		return "", errors.New("absolute URL is not allowed")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", errors.New("invalid path encoding")
	}
	if decoded != raw && strings.Contains(decoded, "%") {
		return "", errors.New("nested path encoding is not allowed")
	}
	if strings.Contains(decoded, "\x00") || strings.Contains(decoded, "\\") || strings.Contains(decoded, "://") {
		return "", errors.New("invalid path")
	}
	for _, part := range strings.Split(strings.Trim(decoded, "/"), "/") {
		if part == ".." || part == "." {
			return "", errors.New("path traversal is not allowed")
		}
	}
	clean := path.Clean("/" + decoded)
	if clean == "/" {
		return "", nil
	}
	for _, part := range strings.Split(strings.Trim(clean, "/"), "/") {
		if part == ".." || part == "." || part == "" {
			return "", errors.New("path traversal is not allowed")
		}
	}
	return strings.TrimPrefix(clean, "/"), nil
}
