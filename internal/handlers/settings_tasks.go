package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	apmw "github.com/aldoerianda/tracker/internal/middleware"
	"github.com/aldoerianda/tracker/internal/repository"
	"github.com/aldoerianda/tracker/internal/services"
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
	ID        int64
	Title     string
	Category  string
	Frequency string
	StartDate string
	EndDate   string
	Active    bool
}

type taskListView struct {
	BaseView
	Search string
	Rows   []taskListRow
}

type taskFormView struct {
	BaseView
	IsNew       bool
	FormAction  string
	DeleteURL   string
	Errors      map[string]string
	Title       string
	Description string
	Category    string
	Frequency   string
	StartDate   string
	EndDate     string
	Active      bool
	Frequencies []string
}

// List renders the settings/tasks index.
func (h *SettingsTasksHandler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	q := r.URL.Query()
	filter := services.TaskListFilter{
		Search: q.Get("q"),
	}
	tasks, err := h.admin.List(r.Context(), filter)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	rows := make([]taskListRow, 0, len(tasks))
	for _, t := range tasks {
		var cat, end string
		if t.Category.Valid {
			cat = t.Category.String
		}
		if t.EndDate.Valid {
			end = t.EndDate.Time.Format("2006-01-02")
		}
		rows = append(rows, taskListRow{
			ID:        t.ID,
			Title:     t.Title,
			Category:  cat,
			Frequency: string(t.Frequency),
			StartDate: t.StartDate.Format("2006-01-02"),
			EndDate:   end,
			Active:    t.Active,
		})
	}

	view := taskListView{
		BaseView: BaseView{
			Title:     "Tasks — Settings",
			UserName:  user.Name,
			UserRole:  string(user.Role),
			CSRFToken: token,
		},
		Search: filter.Search,
		Rows:   rows,
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
				IsNew:       true,
				FormAction:  "/settings/tasks",
				Errors:      ve.Fields,
				Title:       input.Title,
				Description: input.Description,
				Category:    input.Category,
				Frequency:   input.Frequency,
				StartDate:   input.StartDate,
				EndDate:     input.EndDate,
				Active:      input.Active,
				Frequencies: allFrequencies(),
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

	var desc, cat, end string
	if t.Description.Valid {
		desc = t.Description.String
	}
	if t.Category.Valid {
		cat = t.Category.String
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
		IsNew:       false,
		FormAction:  "/settings/tasks/" + strconv.FormatInt(id, 10),
		DeleteURL:   "/settings/tasks/" + strconv.FormatInt(id, 10) + "/delete",
		Title:       t.Title,
		Description: desc,
		Category:    cat,
		Frequency:   string(t.Frequency),
		StartDate:   t.StartDate.Format("2006-01-02"),
		EndDate:     end,
		Active:      t.Active,
		Frequencies: allFrequencies(),
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
				IsNew:       false,
				FormAction:  "/settings/tasks/" + strconv.FormatInt(id, 10),
				DeleteURL:   "/settings/tasks/" + strconv.FormatInt(id, 10) + "/delete",
				Errors:      ve.Fields,
				Title:       input.Title,
				Description: input.Description,
				Category:    input.Category,
				Frequency:   input.Frequency,
				StartDate:   input.StartDate,
				EndDate:     input.EndDate,
				Active:      input.Active,
				Frequencies: allFrequencies(),
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

// Delete soft-deletes a task (sets active=false).
func (h *SettingsTasksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.admin.SoftDelete(r.Context(), id); err != nil {
		if errors.Is(err, services.ErrTaskNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("task soft-deleted", "id", id, "by", user.ID)
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
		Title:       r.PostFormValue("title"),
		Description: r.PostFormValue("description"),
		Category:    strings.TrimSpace(r.PostFormValue("category")),
		Frequency:   r.PostFormValue("frequency"),
		StartDate:   r.PostFormValue("start_date"),
		EndDate:     r.PostFormValue("end_date"),
		Active:      r.PostFormValue("active") == "on" || r.PostFormValue("active") == "true",
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
