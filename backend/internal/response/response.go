package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope is the unified JSON body for all JSON API responses.
// HTTP status reflects the outcome; success=1 only on 2xx.
type Envelope struct {
	Success int         `json:"success"`
	Msg     string      `json:"msg"`
	Code    int         `json:"code,omitempty"`
	Data    interface{} `json:"data"`
}

func OK(c *gin.Context, data interface{}, msg string) {
	if msg == "" {
		msg = "ok"
	}
	c.JSON(http.StatusOK, Envelope{Success: 1, Msg: msg, Data: data})
}

// HTTPStatusForCode maps business code to standard HTTP status (RFC 7231).
func HTTPStatusForCode(code int) int {
	switch code {
	case 401:
		return http.StatusUnauthorized
	case 403:
		return http.StatusForbidden
	case 404:
		return http.StatusNotFound
	case 409:
		return http.StatusConflict
	case 422:
		return http.StatusUnprocessableEntity
	case 429:
		return http.StatusTooManyRequests
	case 503:
		return http.StatusServiceUnavailable
	}
	if code >= 500 && code < 600 {
		return code
	}
	if code >= 400 && code < 500 {
		return code
	}
	return http.StatusBadRequest
}

func Fail(c *gin.Context, httpStatus int, msg string, code int) {
	FailData(c, httpStatus, msg, code, nil)
}

func FailData(c *gin.Context, httpStatus int, msg string, code int, data interface{}) {
	if code == 0 {
		code = httpStatus
	}
	c.JSON(httpStatus, Envelope{Success: 0, Msg: msg, Code: code, Data: data})
}

// BizFail returns a business error with HTTP status aligned to code (not HTTP 200).
func BizFail(c *gin.Context, msg string, code int) {
	BizFailData(c, msg, code, nil)
}

// BizFailData is BizFail with optional data payload (e.g. captcha_required).
func BizFailData(c *gin.Context, msg string, code int, data interface{}) {
	if code == 0 {
		code = http.StatusBadRequest
	}
	FailData(c, HTTPStatusForCode(code), msg, code, data)
}
