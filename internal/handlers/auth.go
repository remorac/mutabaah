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
	resets        *services.PasswordResetService
	tmpl          *Templates
	logger        *slog.Logger
	secureCookies bool
}

func NewAuthHandler(auth *services.AuthService, resets *services.PasswordResetService, tmpl *Templates, logger *slog.Logger, secureCookies bool) *AuthHandler {
	return &AuthHandler{auth: auth, resets: resets, tmpl: tmpl, logger: logger, secureCookies: secureCookies}
}

type loginViewData struct {
	BaseView
	Error string
	Email string
}

type forgotPasswordViewData struct {
	BaseView
	CSRFToken string
	Error     string
	Email     string
	Sent      bool
}

type resetPasswordViewData struct {
	BaseView
	CSRFToken string
	Token     string
	Errors    map[string]string
	Invalid   bool
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

func (h *AuthHandler) ForgotPasswordForm(w http.ResponseWriter, r *http.Request) {
	h.renderForgotPassword(w, forgotPasswordViewData{
		BaseView:  BaseView{Title: "Forgot password"},
		CSRFToken: h.optionalCSRFToken(r),
	}, http.StatusOK)
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.PostFormValue("email"))
	if err := h.resets.RequestReset(r.Context(), email); err != nil {
		h.logger.Error("request password reset", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderForgotPassword(w, forgotPasswordViewData{
		BaseView:  BaseView{Title: "Forgot password"},
		CSRFToken: h.optionalCSRFToken(r),
		Email:     email,
		Sent:      true,
	}, http.StatusOK)
}

func (h *AuthHandler) ResetPasswordForm(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	data := resetPasswordViewData{
		BaseView:  BaseView{Title: "Reset password"},
		CSRFToken: h.optionalCSRFToken(r),
		Token:     token,
	}
	if err := h.resets.ValidateToken(r.Context(), token); err != nil {
		if errors.Is(err, services.ErrInvalidResetToken) {
			data.Invalid = true
			h.renderResetPassword(w, data, http.StatusUnprocessableEntity)
			return
		}
		h.logger.Error("validate reset token", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderResetPassword(w, data, http.StatusOK)
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(r.PostFormValue("token"))
	if err := h.resets.ResetPassword(r.Context(), token, r.PostFormValue("new_password"), r.PostFormValue("confirm_password")); err != nil {
		var ve *services.ValidationError
		if errors.As(err, &ve) {
			h.renderResetPassword(w, resetPasswordViewData{
				BaseView:  BaseView{Title: "Reset password"},
				CSRFToken: h.optionalCSRFToken(r),
				Token:     token,
				Errors:    ve.Fields,
			}, http.StatusUnprocessableEntity)
			return
		}
		if errors.Is(err, services.ErrInvalidResetToken) {
			h.renderResetPassword(w, resetPasswordViewData{
				BaseView:  BaseView{Title: "Reset password"},
				CSRFToken: h.optionalCSRFToken(r),
				Invalid:   true,
			}, http.StatusUnprocessableEntity)
			return
		}
		h.logger.Error("reset password", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
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

func (h *AuthHandler) renderForgotPassword(w http.ResponseWriter, data forgotPasswordViewData, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.tmpl.Render(w, "auth/forgot_password.html", data); err != nil {
		h.logger.Error("render forgot password", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func (h *AuthHandler) renderResetPassword(w http.ResponseWriter, data resetPasswordViewData, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.tmpl.Render(w, "auth/reset_password.html", data); err != nil {
		h.logger.Error("render reset password", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func (h *AuthHandler) optionalCSRFToken(r *http.Request) string {
	if sid := apmw.SessionIDFromContext(r.Context()); sid != "" {
		return h.auth.CSRFToken(sid)
	}
	return ""
}
