package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/services"
)

// AuthHandler renders the login view and processes login/logout POSTs.
type AuthHandler struct {
	auth          *services.AuthService
	tmpl          *Templates
	logger        *slog.Logger
	secureCookies bool
}

func NewAuthHandler(auth *services.AuthService, tmpl *Templates, logger *slog.Logger, secureCookies bool) *AuthHandler {
	return &AuthHandler{auth: auth, tmpl: tmpl, logger: logger, secureCookies: secureCookies}
}

type loginViewData struct {
	BaseView
	Error string
	Email string
}

// LoginForm renders the login page. If already authenticated, redirect home.
func (h *AuthHandler) LoginForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := apmw.UserFromContext(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	h.renderLogin(w, loginViewData{BaseView: BaseView{Title: "Sign in"}})
}

// Login verifies credentials and issues a session cookie. Errors are folded
// into a single generic message to avoid user-enumeration.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.PostFormValue("email"))
	password := r.PostFormValue("password")

	token, _, err := h.auth.Login(r.Context(), email, password)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			w.WriteHeader(http.StatusUnauthorized)
			h.renderLogin(w, loginViewData{BaseView: BaseView{Title: "Sign in"}, Error: "Invalid email or password.", Email: email})
			return
		}
		h.logger.Error("login failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     apmw.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(h.auth.SessionLifetime().Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout destroys the active session and clears the cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if sid := apmw.SessionIDFromContext(r.Context()); sid != "" {
		if err := h.auth.Logout(r.Context(), sid); err != nil {
			h.logger.Warn("logout cleanup", "err", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     apmw.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthHandler) renderLogin(w http.ResponseWriter, data loginViewData) {
	if err := h.tmpl.Render(w, "auth/login.html", data); err != nil {
		h.logger.Error("render login", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}
