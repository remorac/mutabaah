package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	apmw "github.com/aldoerianda/tracker/internal/middleware"
	"github.com/aldoerianda/tracker/internal/services"
)

// ProfileHandler renders and processes the self-service profile page.
// Currently scoped to password change; account-level fields could be added later.
type ProfileHandler struct {
	auth   *services.AuthService
	users  *services.UserAdminService
	tmpl   *Templates
	errs   *ErrorPages
	logger *slog.Logger
}

func NewProfileHandler(auth *services.AuthService, users *services.UserAdminService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *ProfileHandler {
	return &ProfileHandler{auth: auth, users: users, tmpl: tmpl, errs: errs, logger: logger}
}

type profileView struct {
	BaseView
	Email   string
	Errors  map[string]string
	Success bool
}

// Show renders the profile page (GET).
func (h *ProfileHandler) Show(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	h.render(w, profileView{
		BaseView: BaseView{
			Title:     "Profile — Settings",
			UserName:  user.Name,
			UserRole:  string(user.Role),
			CSRFToken: token,
		},
		Email: user.Email,
	}, http.StatusOK)
}

// ChangePassword processes POST /settings/profile. On success the user's other
// sessions are revoked by the service, but the current session was deleted too —
// so we redirect the user to /login to start fresh.
func (h *ProfileHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	next := r.PostFormValue("new_password")
	confirm := r.PostFormValue("confirm_password")

	if err := h.users.ChangePassword(r.Context(), user.ID, next, confirm); err != nil {
		var ve *services.ValidationError
		if errors.As(err, &ve) {
			h.render(w, profileView{
				BaseView: BaseView{
					Title:     "Profile — Settings",
					UserName:  user.Name,
					UserRole:  string(user.Role),
					CSRFToken: token,
				},
				Email:  user.Email,
				Errors: ve.Fields,
			}, http.StatusUnprocessableEntity)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     apmw.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	h.logger.Info("password changed", "user", user.ID)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *ProfileHandler) render(w http.ResponseWriter, view profileView, status int) {
	w.WriteHeader(status)
	if err := h.tmpl.Render(w, "settings/profile.html", view); err != nil {
		h.logger.Error("render profile", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}
