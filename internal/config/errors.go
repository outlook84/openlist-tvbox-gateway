package config

import (
	"errors"
	"fmt"
)

type CodedError struct {
	Code    string
	Message string
	Params  map[string]any
}

func NewCodedError(code, message string, params map[string]any) CodedError {
	return CodedError{Code: code, Message: message, Params: params}
}

func CodedErrorf(code string, params map[string]any, format string, args ...any) error {
	return NewCodedError(code, fmt.Sprintf(format, args...), params)
}

func (e CodedError) Error() string {
	return e.Message
}

func ErrorCode(err error, fallback string) string {
	var coded CodedError
	if errors.As(err, &coded) && coded.Code != "" {
		return coded.Code
	}
	return fallback
}

func ErrorParams(err error) map[string]any {
	var coded CodedError
	if errors.As(err, &coded) {
		return coded.Params
	}
	return nil
}
