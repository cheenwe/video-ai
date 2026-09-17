package response

import (
	"net/http"
	"testing"
)

func TestHTTPStatusForCode(t *testing.T) {
	cases := []struct {
		code int
		want int
	}{
		{401, http.StatusUnauthorized},
		{403, http.StatusForbidden},
		{404, http.StatusNotFound},
		{409, http.StatusConflict},
		{422, http.StatusUnprocessableEntity},
		{429, http.StatusTooManyRequests},
		{503, http.StatusServiceUnavailable},
		{400, http.StatusBadRequest},
		{500, http.StatusInternalServerError},
		{502, http.StatusBadGateway},
		{418, 418},
		{0, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := HTTPStatusForCode(tc.code); got != tc.want {
			t.Fatalf("HTTPStatusForCode(%d) = %d, want %d", tc.code, got, tc.want)
		}
	}
}
