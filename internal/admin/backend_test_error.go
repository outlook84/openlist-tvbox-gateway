package admin

import (
	"strings"
)

func backendTestErrorKind(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "authorization"):
		return "authorization"
	case strings.Contains(msg, "permission denied"):
		return "permission"
	case strings.Contains(msg, "openlist request failed"):
		return "request"
	case strings.Contains(msg, "openlist"):
		return "upstream"
	default:
		return "unknown"
	}
}
