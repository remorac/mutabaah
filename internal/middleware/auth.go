package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

// SessionCookieName is the name of the cookie that carries the session token.
const SessionCookieName = "tracker_session"

type ctxKey int

const (
	ctxKeyUser ctxKey = iota
	ctxKeySessionID
)

// UserFromContext returns the authenticated user attached to ctx, if any.
func UserFromContext(ctx context.Context) (repository.User, bool) {
	u, ok := ctx.Value(ctxKeyUser).(repository.User)
	return u, ok
}

// SessionIDFromContext returns the active session token, if any.
func SessionIDFromContext(ctx context.Context) string {
	if s, ok := ctx.Value(ctxKeySessionID).(string); ok {
		return s
	}
	return ""
}

// LoadUser inspects the session cookie and, on success, attaches the user
// and session id to the request context. Invalid/expired cookies are
// transparently cleared so the client stops sending them.
func LoadUser(auth *services.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			user, err := auth.LookupSession(r.Context(), cookie.Value)
			if err != nil {
				if errors.Is(err, services.ErrSessionNotFound) {
					clearSessionCookie(w)
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyUser, user)
			ctx = context.WithValue(ctx, ctxKeySessionID, cookie.Value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth redirects unauthenticated requests to /login.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFromContext(r.Context()); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ForbiddenHandler is invoked by RequireAdmin to render the 403 response.
// Defaults to plain text; main wires the styled error page after templates
// have loaded. Exposed as a var so we can avoid pulling the handlers package
// into the middleware package (which would invert the dependency direction).
var ForbiddenHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "forbidden", http.StatusForbidden)
}

// RequireAdmin gates a handler to users with role=admin. Unauthenticated
// requests get a redirect (same as RequireAuth); authenticated non-admins
// get a 403 via ForbiddenHandler.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if user.Role != repository.UsersRoleAdmin {
			ForbiddenHandler(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
