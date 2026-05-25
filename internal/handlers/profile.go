package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

// ProfileHandler renders and processes the self-service profile page.
// Covers password change plus menses-period entries for users who want exempt
// tasks (e.g. salah, Quran, sawm) hidden during their period.
type ProfileHandler struct {
	auth   *services.AuthService
	users  *services.UserAdminService
	menses *services.MensesAdminService
	tmpl   *Templates
	errs   *ErrorPages
	logger *slog.Logger
}

func NewProfileHandler(auth *services.AuthService, users *services.UserAdminService, menses *services.MensesAdminService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *ProfileHandler {
	return &ProfileHandler{auth: auth, users: users, menses: menses, tmpl: tmpl, errs: errs, logger: logger}
}

type periodRow struct {
	ID        int64
	StartDate string
	EndDate   string // "" when ongoing
	Ongoing   bool
}

type profileView struct {
	BaseView
	Email        string
	Errors       map[string]string
	Success      bool
	Periods      []periodRow
	PeriodErrors map[string]string
	NewStart     string
	NewEnd       string
}

// Show renders the profile page (GET).
func (h *ProfileHandler) Show(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	periods, err := h.menses.List(r.Context(), user.ID)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	h.render(w, profileView{
		BaseView: BaseView{
			Title:     "Profile — Settings",
			UserName:  user.Name,
			UserRole:  string(user.Role),
			CSRFToken: token,
		},
		Email:   user.Email,
		Periods: rowsFromPeriods(periods),
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
			periods, perr := h.menses.List(r.Context(), user.ID)
			if perr != nil {
				h.errs.ServerError(w, r, perr)
				return
			}
			h.render(w, profileView{
				BaseView: BaseView{
					Title:     "Profile — Settings",
					UserName:  user.Name,
					UserRole:  string(user.Role),
					CSRFToken: token,
				},
				Email:   user.Email,
				Errors:  ve.Fields,
				Periods: rowsFromPeriods(periods),
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

// CreatePeriod handles POST /settings/periods.
func (h *ProfileHandler) CreatePeriod(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	input := services.MensesPeriodInput{
		StartDate: r.PostFormValue("start_date"),
		EndDate:   r.PostFormValue("end_date"),
	}
	if _, err := h.menses.Create(r.Context(), user.ID, input); err != nil {
		var ve *services.ValidationError
		if errors.As(err, &ve) {
			periods, perr := h.menses.List(r.Context(), user.ID)
			if perr != nil {
				h.errs.ServerError(w, r, perr)
				return
			}
			h.render(w, profileView{
				BaseView: BaseView{
					Title:     "Profile — Settings",
					UserName:  user.Name,
					UserRole:  string(user.Role),
					CSRFToken: token,
				},
				Email:        user.Email,
				Periods:      rowsFromPeriods(periods),
				PeriodErrors: ve.Fields,
				NewStart:     input.StartDate,
				NewEnd:       input.EndDate,
			}, http.StatusUnprocessableEntity)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("menses period created", "user", user.ID)
	http.Redirect(w, r, "/settings/profile", http.StatusSeeOther)
}

// DeletePeriod handles POST /settings/periods/{id}/delete.
func (h *ProfileHandler) DeletePeriod(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.menses.Delete(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, services.ErrMensesPeriodNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("menses period deleted", "id", id, "user", user.ID)
	http.Redirect(w, r, "/settings/profile", http.StatusSeeOther)
}

func (h *ProfileHandler) render(w http.ResponseWriter, view profileView, status int) {
	w.WriteHeader(status)
	if err := h.tmpl.Render(w, "settings/profile.html", view); err != nil {
		h.logger.Error("render profile", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func rowsFromPeriods(periods []repository.MensesPeriod) []periodRow {
	out := make([]periodRow, 0, len(periods))
	for _, p := range periods {
		row := periodRow{
			ID:        p.ID,
			StartDate: p.StartDate.Format("2006-01-02"),
		}
		if p.EndDate.Valid {
			row.EndDate = p.EndDate.Time.Format("2006-01-02")
		} else {
			row.Ongoing = true
		}
		out = append(out, row)
	}
	return out
}
