package github

import (
	"errors"
	"fmt"
)

// Коды ошибок репозитория в отчёте.
const (
	CodeNotFound        = "not_found"
	CodeRateLimited     = "rate_limited"
	CodeRequestFailed   = "request_failed"
	CodeInvalidResponse = "invalid_response"
)

// Error — ошибка получения issue одного репозитория; Code попадает в отчёт.
type Error struct {
	Code   string
	Detail string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Detail
}

func errorf(code, format string, args ...any) error {
	return &Error{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// ErrUnauthorized — сервер ответил 401. Это фатально для всего запуска.
var ErrUnauthorized = errors.New("сервер ответил 401 Unauthorized")

// CodeOf возвращает код ошибки репозитория для отчёта.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeRequestFailed
}
