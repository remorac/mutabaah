package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/remorac/mutabaah/internal/imageutil"
	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

// avatarDir is the on-disk directory served by the /static handler that holds
// uploaded profile pictures and their generated thumbnails.
const avatarDir = "web/static/avatars"

// maxAvatarBytes caps the request body for profile picture uploads.
const maxAvatarBytes = 5 << 20

// maxAvatarRequestBytes leaves room for multipart overhead around a 5 MiB file.
const maxAvatarRequestBytes = 6 << 20

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
		BaseView: NewBaseView(user, token, "Profile — Settings"),
		Email:    user.Email,
		Periods:  rowsFromPeriods(periods),
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
				BaseView: NewBaseView(user, token, "Profile — Settings"),
				Email:    user.Email,
				Errors:   ve.Fields,
				Periods:  rowsFromPeriods(periods),
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

// CreatePeriod handles POST /settings/periods. HX-Request returns only the
// refreshed #periods-section fragment; plain form posts redirect.
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
			if isPartialRequest(r) {
				h.renderPeriodsSection(w, r, user, token, ve.Fields, input.StartDate, input.EndDate, http.StatusUnprocessableEntity)
				return
			}
			periods, perr := h.menses.List(r.Context(), user.ID)
			if perr != nil {
				h.errs.ServerError(w, r, perr)
				return
			}
			h.render(w, profileView{
				BaseView:     NewBaseView(user, token, "Profile — Settings"),
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
	if isPartialRequest(r) {
		h.renderPeriodsSection(w, r, user, token, nil, "", "", http.StatusOK)
		return
	}
	http.Redirect(w, r, "/settings/profile", http.StatusSeeOther)
}

// DeletePeriod handles POST /settings/periods/{id}/delete. HX-Request returns
// only the refreshed #periods-section fragment.
func (h *ProfileHandler) DeletePeriod(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)
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
	if isPartialRequest(r) {
		h.renderPeriodsSection(w, r, user, token, nil, "", "", http.StatusOK)
		return
	}
	http.Redirect(w, r, "/settings/profile", http.StatusSeeOther)
}

// UploadPicture handles POST /settings/profile/picture. Accepts a JPEG or PNG
// avatar field, stores the original plus a square thumbnail under
// web/static/avatars/, and points the user's avatar_path at the new file. The
// previous original/thumbnail are removed best-effort.
func (h *ProfileHandler) UploadPicture(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	if r.MultipartForm == nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxAvatarRequestBytes)
		if err := r.ParseMultipartForm(maxAvatarRequestBytes); err != nil {
			h.renderUploadError(w, r, user, token, "Image must be 5 MB or smaller.", http.StatusRequestEntityTooLarge)
			return
		}
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, _, err := r.FormFile("avatar")
	if err != nil {
		h.renderUploadError(w, r, user, token, "Choose a file.", http.StatusUnprocessableEntity)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}
	if int64(len(data)) > maxAvatarBytes {
		h.renderUploadError(w, r, user, token, "Image must be 5 MB or smaller.", http.StatusRequestEntityTooLarge)
		return
	}

	_, ext, err := imageutil.Validate(data)
	if err != nil {
		h.renderUploadError(w, r, user, token, "JPEG or PNG only.", http.StatusUnprocessableEntity)
		return
	}

	basename, err := randomBasename(user.ID)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	if err := imageutil.SaveOriginalAndThumb(avatarDir, basename, ext, data); err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	// Best-effort cleanup of the prior files. We swallow errors so the upload
	// still succeeds when the old files are missing (e.g. moved between hosts).
	if user.AvatarPath.Valid && user.AvatarPath.String != "" {
		old := user.AvatarPath.String
		oldBase := strings.TrimSuffix(old, filepath.Ext(old))
		oldOrig := filepath.Join(avatarDir, old)
		oldThumb := filepath.Join(avatarDir, "thumb_"+oldBase+".jpg")
		if err := os.Remove(oldOrig); err != nil && !errors.Is(err, os.ErrNotExist) {
			h.logger.Warn("remove old avatar original", "err", err, "path", oldOrig)
		}
		if err := os.Remove(oldThumb); err != nil && !errors.Is(err, os.ErrNotExist) {
			h.logger.Warn("remove old avatar thumb", "err", err, "path", oldThumb)
		}
	}

	if err := h.users.UpdateAvatar(r.Context(), user.ID, basename+ext); err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	h.logger.Info("avatar uploaded", "user", user.ID, "file", basename+ext)
	http.Redirect(w, r, "/settings/profile", http.StatusSeeOther)
}

// renderUploadError re-renders the profile page with an inline avatar error.
func (h *ProfileHandler) renderUploadError(w http.ResponseWriter, r *http.Request, user repository.User, token, msg string, status int) {
	periods, err := h.menses.List(r.Context(), user.ID)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}
	h.render(w, profileView{
		BaseView: NewBaseView(user, token, "Profile — Settings"),
		Email:    user.Email,
		Errors:   map[string]string{"avatar": msg},
		Periods:  rowsFromPeriods(periods),
	}, status)
}

// randomBasename returns "<userID>_<6 hex bytes>".
func randomBasename(userID int64) (string, error) {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d_%s", userID, hex.EncodeToString(buf[:])), nil
}

func (h *ProfileHandler) renderPeriodsSection(w http.ResponseWriter, r *http.Request, user repository.User, csrfToken string, fieldErrors map[string]string, newStart, newEnd string, status int) {
	periods, err := h.menses.List(r.Context(), user.ID)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}
	view := profileView{
		BaseView:     BaseView{CSRFToken: csrfToken},
		Periods:      rowsFromPeriods(periods),
		PeriodErrors: fieldErrors,
		NewStart:     newStart,
		NewEnd:       newEnd,
	}
	w.WriteHeader(status)
	if err := h.tmpl.RenderPartial(w, "settings/_periods_section.html", view); err != nil {
		h.logger.Error("render periods section", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
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
