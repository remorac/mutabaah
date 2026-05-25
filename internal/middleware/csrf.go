package middleware

import (
	"mime"
	"net/http"

	"github.com/remorac/mutabaah/internal/services"
)

// CSRFFieldName is the form field carrying the CSRF token on POST forms.
const CSRFFieldName = "_csrf"

// CSRFHeaderName is the alternative header HTMX/AJAX clients can use to
// supply the token.
const CSRFHeaderName = "X-CSRF-Token"

// maxCSRFMultipartBytes bounds multipart parsing done only to read the CSRF
// field before a handler sees the request.
const maxCSRFMultipartBytes = 6 << 20

// CSRF gates state-changing requests: any non-safe method requires a valid
// token tied to the current session. Safe methods (GET/HEAD/OPTIONS) pass
// through. Anonymous state-changing requests (e.g. POST /login) are exempted
// at the route level rather than here, so the same cookie-bound token works
// once a session exists.
func CSRF(auth *services.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			sessionID := SessionIDFromContext(r.Context())
			if sessionID == "" {
				next.ServeHTTP(w, r)
				return
			}
			supplied := r.Header.Get(CSRFHeaderName)
			if supplied == "" {
				if isMultipartForm(r) {
					r.Body = http.MaxBytesReader(w, r.Body, maxCSRFMultipartBytes)
					if err := r.ParseMultipartForm(maxCSRFMultipartBytes); err == nil {
						supplied = r.PostFormValue(CSRFFieldName)
					}
				} else if err := r.ParseForm(); err == nil {
					supplied = r.PostFormValue(CSRFFieldName)
				}
			}
			if !auth.VerifyCSRF(sessionID, supplied) {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isMultipartForm(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "multipart/form-data"
}

func isSafeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}
