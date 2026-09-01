package apperr

import (
	"errors"
	"net/http"
)

type Kind int

const (
	KindInvalid Kind = iota
	KindNotFound
	KindConflict
	KindUnauthorized
	KindForbidden
	KindInternal
)

type Error struct {
	Kind    Kind
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func New(kind Kind, msg string) *Error { return &Error{Kind: kind, Message: msg} }
func Wrap(kind Kind, err error) *Error {
	return &Error{Kind: kind, Err: err, Message: "unexpected failure"}
}

func Invalid(msg string) *Error      { return New(KindInvalid, msg) }
func NotFound(msg string) *Error     { return New(KindNotFound, msg) }
func Conflict(msg string) *Error     { return New(KindConflict, msg) }
func Unauthorized(msg string) *Error { return New(KindUnauthorized, msg) }
func Forbidden(msg string) *Error    { return New(KindForbidden, msg) }
func Internal(err error) *Error {
	return &Error{Kind: KindInternal, Err: err, Message: "internal server error"}
}

func From(err error) *Error {
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	return Internal(err)
}

// ErrorResponse is the standard error body emitted by the server's error
// handler: {"error": "<message>"}. Referenced by Swagger @Failure annotations.
type ErrorResponse struct {
	Error string `json:"error"`
}

func HTTPStatus(k Kind) int {
	switch k {
	case KindInvalid:
		return http.StatusBadRequest
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
