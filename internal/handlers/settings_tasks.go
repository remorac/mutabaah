package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

// SettingsTasksHandler renders admin task CRUD pages. All routes mounted under
// this handler must be wrapped with RequireAdmin.
type SettingsTasksHandler struct {
	auth   *services.AuthService
	admin  *services.TaskAdminService
	tmpl   *Templates
	errs   *ErrorPages
	logger *slog.Logger
}

func NewSettingsTasksHandler(auth *services.AuthService, admin *services.TaskAdminService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *SettingsTasksHandler {
	return &SettingsTasksHandler{auth: auth, admin: admin, tmpl: tmpl, errs: errs, logger: logger}
}

type taskListRow struct {
	ID          int64
	Title       string
	Description string
	Frequency   string
	StartDate   string
	EndDate     string
	Active      bool
	Sequence    int32
	IsFirst     bool
	IsLast      bool
}

type taskListView struct {
	BaseView
	Rows []taskListRow
}

type taskFormView struct {
	BaseView
	IsNew              bool
	FormAction         string
	DeleteURL          string
	Errors             map[string]string
	Title              string
	Description        string
	Frequency          string
	StartDate          string
	EndDate            string
	Active             bool
	ExemptDuringMenses bool
	Sequence           string
	Frequencies        []string
}

// List renders the settings/tasks index.
func (h *SettingsTasksHandler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	tasks, err := h.admin.List(r.Context())
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	rows := make([]taskListRow, 0, len(tasks))
	for i, t := range tasks {
		var desc, end string
		if t.Description.Valid {
			desc = t.Description.String
		}
		if t.EndDate.Valid {
			end = t.EndDate.Time.Format("2006-01-02")
		}
		rows = append(rows, taskListRow{
			ID:          t.ID,
			Title:       t.Title,
			Description: desc,
			Frequency:   string(t.Frequency),
			StartDate:   t.StartDate.Format("2006-01-02"),
			EndDate:     end,
			Active:      t.Active,
			Sequence:    t.Sequence,
			IsFirst:     i == 0,
			IsLast:      i == len(tasks)-1,
		})
	}

	view := taskListView{
		BaseView: BaseView{
			Title:     "Tasks — Settings",
			UserName:  user.Name,
			UserRole:  string(user.Role),
			CSRFToken: token,
		},
		Rows: rows,
	}
	if err := h.tmpl.Render(w, "settings/tasks/index.html", view); err != nil {
		h.logger.Error("render settings tasks list", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// NewForm renders the empty "create task" form.
func (h *SettingsTasksHandler) NewForm(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	view := taskFormView{
		BaseView: BaseView{
			Title:     "New Task — Settings",
			UserName:  user.Name,
			UserRole:  string(user.Role),
			CSRFToken: token,
		},
		IsNew:       true,
		FormAction:  "/settings/tasks",
		Active:      true,
		Sequence:    "0",
		Frequencies: allFrequencies(),
	}
	if err := h.tmpl.Render(w, "settings/tasks/form.html", view); err != nil {
		h.logger.Error("render new task form", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// Create handles POST /settings/tasks.
func (h *SettingsTasksHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	input := parseTaskInput(r)
	id, err := h.admin.Create(r.Context(), input)
	if err != nil {
		var ve *services.ValidationError
		if errors.As(err, &ve) {
			h.renderFormError(w, r, taskFormView{
				BaseView: BaseView{
					Title:     "New Task — Settings",
					UserName:  user.Name,
					UserRole:  string(user.Role),
					CSRFToken: token,
				},
				IsNew:              true,
				FormAction:         "/settings/tasks",
				Errors:             ve.Fields,
				Title:              input.Title,
				Description:        input.Description,
				Frequency:          input.Frequency,
				StartDate:          input.StartDate,
				EndDate:            input.EndDate,
				Active:             input.Active,
				ExemptDuringMenses: input.ExemptDuringMenses,
				Sequence:           input.Sequence,
				Frequencies:        allFrequencies(),
			})
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("task created", "id", id, "by", user.ID)
	http.Redirect(w, r, "/settings/tasks", http.StatusSeeOther)
}

// EditForm renders the populated edit form.
func (h *SettingsTasksHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	t, err := h.admin.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrTaskNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}

	var desc, end string
	if t.Description.Valid {
		desc = t.Description.String
	}
	if t.EndDate.Valid {
		end = t.EndDate.Time.Format("2006-01-02")
	}

	view := taskFormView{
		BaseView: BaseView{
			Title:     "Edit Task — Settings",
			UserName:  user.Name,
			UserRole:  string(user.Role),
			CSRFToken: token,
		},
		IsNew:              false,
		FormAction:         "/settings/tasks/" + strconv.FormatInt(id, 10),
		DeleteURL:          "/settings/tasks/" + strconv.FormatInt(id, 10) + "/delete",
		Title:              t.Title,
		Description:        desc,
		Frequency:          string(t.Frequency),
		StartDate:          t.StartDate.Format("2006-01-02"),
		EndDate:            end,
		Active:             t.Active,
		ExemptDuringMenses: t.ExemptDuringMenses,
		Sequence:           strconv.FormatInt(int64(t.Sequence), 10),
		Frequencies:        allFrequencies(),
	}
	if err := h.tmpl.Render(w, "settings/tasks/form.html", view); err != nil {
		h.logger.Error("render edit form", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// Update handles POST /settings/tasks/:id.
func (h *SettingsTasksHandler) Update(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	input := parseTaskInput(r)
	if err := h.admin.Update(r.Context(), id, input); err != nil {
		var ve *services.ValidationError
		if errors.As(err, &ve) {
			h.renderFormError(w, r, taskFormView{
				BaseView: BaseView{
					Title:     "Edit Task — Settings",
					UserName:  user.Name,
					UserRole:  string(user.Role),
					CSRFToken: token,
				},
				IsNew:              false,
				FormAction:         "/settings/tasks/" + strconv.FormatInt(id, 10),
				DeleteURL:          "/settings/tasks/" + strconv.FormatInt(id, 10) + "/delete",
				Errors:             ve.Fields,
				Title:              input.Title,
				Description:        input.Description,
				Frequency:          input.Frequency,
				StartDate:          input.StartDate,
				EndDate:            input.EndDate,
				Active:             input.Active,
				ExemptDuringMenses: input.ExemptDuringMenses,
				Sequence:           input.Sequence,
				Frequencies:        allFrequencies(),
			})
			return
		}
		if errors.Is(err, services.ErrTaskNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("task updated", "id", id, "by", user.ID)
	http.Redirect(w, r, "/settings/tasks", http.StatusSeeOther)
}

// MoveUp shifts a task one position earlier in the ordered list.
func (h *SettingsTasksHandler) MoveUp(w http.ResponseWriter, r *http.Request) {
	h.move(w, r, -1)
}

// MoveDown shifts a task one position later in the ordered list.
func (h *SettingsTasksHandler) MoveDown(w http.ResponseWriter, r *http.Request) {
	h.move(w, r, 1)
}

// SetActive activates or deactivates a task without deleting completion history.
func (h *SettingsTasksHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	active := r.PostFormValue("active") == "true" || r.PostFormValue("active") == "on"
	if err := h.admin.SetActive(r.Context(), id, active); err != nil {
		if errors.Is(err, services.ErrTaskNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("task active status changed", "id", id, "active", active, "by", user.ID)
	redirect := "/settings/tasks"
	if q := r.URL.RawQuery; q != "" {
		redirect += "?" + q
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (h *SettingsTasksHandler) move(w http.ResponseWriter, r *http.Request, delta int) {
	user, _ := apmw.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.admin.Move(r.Context(), id, delta); err != nil {
		if errors.Is(err, services.ErrTaskNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("task moved", "id", id, "delta", delta, "by", user.ID)
	redirect := "/settings/tasks"
	if q := r.URL.RawQuery; q != "" {
		redirect += "?" + q
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// Delete permanently removes a task and its completion history.
func (h *SettingsTasksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.admin.Delete(r.Context(), id); err != nil {
		if errors.Is(err, services.ErrTaskNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("task deleted", "id", id, "by", user.ID)
	http.Redirect(w, r, "/settings/tasks", http.StatusSeeOther)
}

func (h *SettingsTasksHandler) renderFormError(w http.ResponseWriter, _ *http.Request, view taskFormView) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := h.tmpl.Render(w, "settings/tasks/form.html", view); err != nil {
		h.logger.Error("render form", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func parseTaskInput(r *http.Request) services.TaskInput {
	return services.TaskInput{
		Title:              r.PostFormValue("title"),
		Description:        strings.TrimSpace(r.PostFormValue("description")),
		Frequency:          r.PostFormValue("frequency"),
		StartDate:          r.PostFormValue("start_date"),
		EndDate:            r.PostFormValue("end_date"),
		Active:             r.PostFormValue("active") == "on" || r.PostFormValue("active") == "true",
		ExemptDuringMenses: r.PostFormValue("exempt_during_menses") == "on" || r.PostFormValue("exempt_during_menses") == "true",
		Sequence:           strings.TrimSpace(r.PostFormValue("sequence")),
	}
}

func allFrequencies() []string {
	return []string{
		string(repository.TasksFrequencyDaily),
		string(repository.TasksFrequencyWeekly),
		string(repository.TasksFrequencyMonthly),
		string(repository.TasksFrequencyYearly),
	}
}
