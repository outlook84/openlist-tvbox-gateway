package admin

import (
	"encoding/json"
	"net/http"

	"openlist-tvbox/internal/config"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAdminError(w http.ResponseWriter, status int, code, message string, params map[string]any) {
	body := map[string]any{"ok": false, "error": message, "error_code": code}
	if len(params) > 0 {
		body["error_params"] = params
	}
	writeJSON(w, status, body)
}

func writeAdminErrorFromError(w http.ResponseWriter, status int, err error, fallbackCode string) {
	writeAdminError(w, status, errorCode(err, fallbackCode), err.Error(), errorParams(err))
}

func writeConfigAdminError(w http.ResponseWriter, status int, err error) {
	writeAdminError(w, status, config.ErrorCode(err, "config.invalid"), err.Error(), config.ErrorParams(err))
}

func writeValidationError(w http.ResponseWriter, status int, err error, fallbackCode string) {
	body := map[string]any{"valid": false, "error": err.Error(), "error_code": errorCode(err, fallbackCode)}
	if params := errorParams(err); len(params) > 0 {
		body["error_params"] = params
	}
	writeJSON(w, status, body)
}

func writeConfigValidationError(w http.ResponseWriter, status int, err error) {
	body := map[string]any{"valid": false, "error": err.Error(), "error_code": config.ErrorCode(err, "config.invalid")}
	if params := config.ErrorParams(err); len(params) > 0 {
		body["error_params"] = params
	}
	writeJSON(w, status, body)
}
