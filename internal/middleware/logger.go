package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// SlogRequestLogger emits one structured log entry per HTTP request, using the
// request id chimw.RequestID puts on the context. Replaces chimw.Logger so the
// access log is JSON/key-value rather than the default colored text.
func SlogRequestLogger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			reqID := chimw.GetReqID(r.Context())

			defer func() {
				base.LogAttrs(r.Context(), slog.LevelInfo, "http",
					slog.String("request_id", reqID),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", ww.Status()),
					slog.Int("bytes", ww.BytesWritten()),
					slog.Duration("duration", time.Since(start)),
					slog.String("remote", r.RemoteAddr),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// RequestLogger returns a per-request logger derived from base, pre-tagged with
// the chi request id so handler logs correlate with the access log line.
func RequestLogger(base *slog.Logger, r *http.Request) *slog.Logger {
	id := chimw.GetReqID(r.Context())
	if id == "" {
		return base
	}
	return base.With("request_id", id)
}
