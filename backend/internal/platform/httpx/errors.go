package httpx

import "net/http"

// AppError is a domain error carrying a stable business code and the HTTP
// status to return. Handlers return these; the response layer renders them.
type AppError struct {
	Code       int
	Message    string
	HTTPStatus int
}

func (e *AppError) Error() string { return e.Message }

// New builds an AppError. Prefer defining reusable codes in the catalog below.
func New(code, httpStatus int, msg string) *AppError {
	return &AppError{Code: code, Message: msg, HTTPStatus: httpStatus}
}

// WithMessage returns a copy with a caller-specific message, preserving the
// code and HTTP status — useful for adding context to a catalog error.
func (e *AppError) WithMessage(msg string) *AppError {
	cp := *e
	cp.Message = msg
	return &cp
}

// Error code catalog. Convention: 0 = ok. Group by domain in 1000-wide ranges
// so each module owns its band:
//
//	1xxx general   2xxx auth/identity   3xxx dataset/upload   4xxx order
//	5xxx payment/settlement   6xxx delivery
//
// Modules add their own codes in their packages; the common ones live here.
var (
	ErrInternal     = New(1000, http.StatusInternalServerError, "internal error")
	ErrInvalidParam = New(1001, http.StatusBadRequest, "invalid parameter")
	ErrNotFound     = New(1002, http.StatusNotFound, "resource not found")
	ErrConflict     = New(1003, http.StatusConflict, "resource conflict")
	ErrRateLimited  = New(1004, http.StatusTooManyRequests, "rate limited")
	ErrUnauthorized = New(2000, http.StatusUnauthorized, "unauthorized")
	ErrForbidden    = New(2001, http.StatusForbidden, "forbidden")
)
