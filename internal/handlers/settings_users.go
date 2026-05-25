package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	apmw "github.com/aldoerianda/tracker/internal/middleware"
	"github.com/aldoerianda/tracker/internal/repository"
	"github.com/aldoerianda/tracker/internal/services"
)

// SettingsUsersHandler renders admin user CRUD pages. All routes mounted under
// this handler must be wrapped with RequireAdmin.
type SettingsUsersHandler struct {
	auth   *services.AuthService
	users  *services.UserAdminService
	tmpl   *Templates
	errs   *ErrorPages
	logger *slog.Logger
}

func NewSettingsUsersHandler(auth *services.AuthService, users *services.UserAdminService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *SettingsUsersHandler {
	return &SettingsUsersHandler{auth: auth, users: users, tmpl: tmpl, errs: errs, logger: logger}
}

// usersPageSize matches the spec's "paginated if >50" threshold.
const usersPageSize = 50

type userListRow struct {
	ID        int64
	Email     string
	Name      string
	Role      string
	CreatedAt string
	IsSelf    bool
}

type userListView struct {
	BaseView
	Rows        []userListRow
	Page        int
	TotalPages  int
	Total       int64
	HasPrev     bool
	HasNext     bool
	PrevURL     string
	NextURL     string
	FlashNotice string
}

type userFormView struct {
	BaseView
	IsNew        bool
	FormAction   string
	DeleteURL    string
	Errors       map[string]string
	Email        string
	Name         string
	Role         string
	Roles        []string
	IsSelf       bool
	CanDelete    bool
	GeneralError string
}

// List renders the settings/users index. Paginates when the user count exceeds
// usersPageSize.
func (h *SettingsUsersHandler) List(w http.ResponseWriter, r *http.Request) {
	current, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	offset := int32((page - 1) * usersPageSize)

	users, total, err := h.users.List(r.Context(), usersPageSize, offset)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	rows := make([]userListRow, 0, len(users))
	for _, u := range users {
		rows = append(rows, userListRow{
			ID:        u.ID,
			Email:     u.Email,
			Name:      u.Name,
			Role:      string(u.Role),
			CreatedAt: u.CreatedAt.Format("2006-01-02"),
			IsSelf:    u.ID == current.ID,
		})
	}

	totalPages := 1
	if total > 0 {
		totalPages = int((total + int64(usersPageSize) - 1) / int64(usersPageSize))
	}

	view := userListView{
		BaseView: BaseView{
			Title:     "Users — Settings",
			UserName:  current.Name,
			UserRole:  string(current.Role),
			CSRFToken: token,
		},
		Rows:       rows,
		Page:       page,
		TotalPages: totalPages,
		Total:      total,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
		PrevURL:    "/settings/users?page=" + strconv.Itoa(page-1),
		NextURL:    "/settings/users?page=" + strconv.Itoa(page+1),
	}
	if err := h.tmpl.Render(w, "settings/users/index.html", view); err != nil {
		h.logger.Error("render users list", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// NewForm renders the empty "create user" form.
func (h *SettingsUsersHandler) NewForm(w http.ResponseWriter, r *http.Request) {
	current, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	view := userFormView{
		BaseView: BaseView{
			Title:     "New User — Settings",
			UserName:  current.Name,
			UserRole:  string(current.Role),
			CSRFToken: token,
		},
		IsNew:      true,
		FormAction: "/settings/users",
		Role:       string(repository.UsersRoleUser),
		Roles:      allRoles(),
	}
	if err := h.tmpl.Render(w, "settings/users/form.html", view); err != nil {
		h.logger.Error("render new user form", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// Create handles POST /settings/users.
func (h *SettingsUsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	current, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	input := parseUserInput(r)
	id, err := h.users.Create(r.Context(), input)
	if err != nil {
		var ve *services.ValidationError
		if errors.As(err, &ve) {
			h.renderFormError(w, userFormView{
				BaseView: BaseView{
					Title:     "New User — Settings",
					UserName:  current.Name,
					UserRole:  string(current.Role),
					CSRFToken: token,
				},
				IsNew:      true,
				FormAction: "/settings/users",
				Errors:     ve.Fields,
				Email:      input.Email,
				Name:       input.Name,
				Role:       input.Role,
				Roles:      allRoles(),
			})
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("user created", "id", id, "by", current.ID)
	http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}

// EditForm renders the populated edit form.
func (h *SettingsUsersHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	current, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	u, err := h.users.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}

	isSelf := u.ID == current.ID
	view := userFormView{
		BaseView: BaseView{
			Title:     "Edit User — Settings",
			UserName:  current.Name,
			UserRole:  string(current.Role),
			CSRFToken: token,
		},
		IsNew:      false,
		FormAction: "/settings/users/" + strconv.FormatInt(id, 10),
		DeleteURL:  "/settings/users/" + strconv.FormatInt(id, 10) + "/delete",
		Email:      u.Email,
		Name:       u.Name,
		Role:       string(u.Role),
		Roles:      allRoles(),
		IsSelf:     isSelf,
		CanDelete:  !isSelf,
	}
	if err := h.tmpl.Render(w, "settings/users/form.html", view); err != nil {
		h.logger.Error("render edit user form", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// Update handles POST /settings/users/:id.
func (h *SettingsUsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	current, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	input := parseUserInput(r)
	if err := h.users.Update(r.Context(), id, input); err != nil {
		var ve *services.ValidationError
		if errors.As(err, &ve) {
			h.renderFormError(w, userFormView{
				BaseView: BaseView{
					Title:     "Edit User — Settings",
					UserName:  current.Name,
					UserRole:  string(current.Role),
					CSRFToken: token,
				},
				IsNew:      false,
				FormAction: "/settings/users/" + strconv.FormatInt(id, 10),
				DeleteURL:  "/settings/users/" + strconv.FormatInt(id, 10) + "/delete",
				Errors:     ve.Fields,
				Email:      input.Email,
				Name:       input.Name,
				Role:       input.Role,
				Roles:      allRoles(),
				IsSelf:     id == current.ID,
				CanDelete:  id != current.ID,
			})
			return
		}
		if errors.Is(err, services.ErrUserNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		if errors.Is(err, services.ErrLastAdmin) {
			h.renderFormError(w, userFormView{
				BaseView: BaseView{
					Title:     "Edit User — Settings",
					UserName:  current.Name,
					UserRole:  string(current.Role),
					CSRFToken: token,
				},
				IsNew:        false,
				FormAction:   "/settings/users/" + strconv.FormatInt(id, 10),
				DeleteURL:    "/settings/users/" + strconv.FormatInt(id, 10) + "/delete",
				Email:        input.Email,
				Name:         input.Name,
				Role:         input.Role,
				Roles:        allRoles(),
				IsSelf:       id == current.ID,
				CanDelete:    id != current.ID,
				GeneralError: "Cannot demote the last admin.",
			})
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("user updated", "id", id, "by", current.ID)
	http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}

// Delete removes a user (hard delete; cascades drop their data).
func (h *SettingsUsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	current, _ := apmw.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if id == current.ID {
		http.Error(w, "cannot delete your own account", http.StatusForbidden)
		return
	}
	if err := h.users.Delete(r.Context(), id); err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			h.errs.NotFound(w, r)
			return
		}
		if errors.Is(err, services.ErrLastAdmin) {
			http.Error(w, "cannot delete the last admin", http.StatusConflict)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}
	h.logger.Info("user deleted", "id", id, "by", current.ID)
	http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}

func (h *SettingsUsersHandler) renderFormError(w http.ResponseWriter, view userFormView) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := h.tmpl.Render(w, "settings/users/form.html", view); err != nil {
		h.logger.Error("render user form", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func parseUserInput(r *http.Request) services.UserInput {
	return services.UserInput{
		Email:    r.PostFormValue("email"),
		Name:     r.PostFormValue("name"),
		Role:     r.PostFormValue("role"),
		Password: r.PostFormValue("password"),
	}
}

func allRoles() []string {
	return []string{
		string(repository.UsersRoleUser),
		string(repository.UsersRoleAdmin),
	}
}
