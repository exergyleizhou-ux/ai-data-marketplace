// Package httpx defines the uniform HTTP response envelope and business error
// codes shared by every module. All /api/v1 handlers respond through OK/Fail so
// clients see one consistent shape:
//
//	{ "code": 0, "message": "ok", "data": {...}, "request_id": "..." }
//
// code 0 means success; any non-zero code is a business error.
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Context keys / headers for request correlation. Kept here (not in middleware)
// so both the middleware and response helpers agree without an import cycle.
const (
	RequestIDKey    = "request_id"
	RequestIDHeader = "X-Request-ID"
)

// Body is the wire format for every API response.
type Body struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// OK writes a 200 success envelope carrying data.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "ok", Data: data, RequestID: RequestID(c)})
}

// Fail writes an error envelope using the AppError's HTTP status and code.
func Fail(c *gin.Context, err *AppError) {
	c.JSON(err.HTTPStatus, Body{Code: err.Code, Message: err.Message, RequestID: RequestID(c)})
}

// RequestID returns the correlation id stored by the RequestID middleware.
func RequestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
