package admin

import (
	"errors"
	"fmt"
)

type codedError struct {
	code    string
	message string
	params  map[string]any
}

func newCodedError(code, message string, params map[string]any) codedError {
	return codedError{code: code, message: message, params: params}
}

func (e codedError) Error() string {
	return e.message
}

func codedf(code string, params map[string]any, format string, args ...any) error {
	return newCodedError(code, fmt.Sprintf(format, args...), params)
}

func errorCode(err error, fallback string) string {
	var coded codedError
	if errors.As(err, &coded) && coded.code != "" {
		return coded.code
	}
	return fallback
}

func errorParams(err error) map[string]any {
	var coded codedError
	if errors.As(err, &coded) {
		return coded.params
	}
	return nil
}
