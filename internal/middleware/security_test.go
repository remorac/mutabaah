package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersAllowCDNSourceMapConnections(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	csp := recorder.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"connect-src 'self' https://cdn.jsdelivr.net https://unpkg.com",
		"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP %q does not contain %q", csp, want)
		}
	}
}
