package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders sets the baseline response headers we want on every page:
// a Content-Security-Policy tight enough to block off-origin script injection
// while still permitting the CDN-hosted UI assets the layout pulls in, plus
// the usual clickjacking / MIME / referrer guards.
//
// The CSP allows the specific CDNs referenced by web/templates/layout.html
// (Tailwind, DaisyUI, Lucide, Alpine, HTMX, Google Fonts). connect-src also
// allows the script CDNs so browser/devtools source-map lookups do not trigger
// CSP noise after those scripts load. Cloudflare Web Analytics injects its
// beacon in production and needs the documented script/connect hosts.
// 'unsafe-inline' is retained for scripts because per-page initializers like
// `window.lucide.createIcons()` run inline; tightening this to nonces is a
// Phase 9 task.
func SecurityHeaders(next http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com https://static.cloudflareinsights.com",
		"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://fonts.googleapis.com",
		"img-src 'self' data:",
		"font-src 'self' data: https://cdn.jsdelivr.net https://fonts.gstatic.com",
		"connect-src 'self' https://cdn.jsdelivr.net https://unpkg.com https://cloudflareinsights.com",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}, "; ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}
