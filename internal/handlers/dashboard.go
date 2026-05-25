package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	apmw "github.com/aldoerianda/tracker/internal/middleware"
	"github.com/aldoerianda/tracker/internal/repository"
	"github.com/aldoerianda/tracker/internal/services"
)

// todayFor returns the app's "today" — the date component of now() evaluated
// in the fixed application timezone.
func todayFor(now func() time.Time) time.Time {
	t := now().In(services.AppLocation)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// DashboardHandler renders the home dashboard and processes task-completion
// toggles. Toggle responses re-render the entire dashboard inner block so
// rows can move between the "Today / Uncompleted" and "Today / Completed"
// columns without a full reload.
type DashboardHandler struct {
	auth   *services.AuthService
	tasks  *services.TaskService
	tmpl   *Templates
	errs   *ErrorPages
	logger *slog.Logger
	now    func() time.Time
}

func NewDashboardHandler(auth *services.AuthService, tasks *services.TaskService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *DashboardHandler {
	return &DashboardHandler{auth: auth, tasks: tasks, tmpl: tmpl, errs: errs, logger: logger, now: time.Now}
}

// missedWindowDays bounds how far back the "Missed" section looks. The
// occurrence engine can answer further back, but the dashboard caps it so
// the page stays scannable.
const missedWindowDays = 30

type taskRowView struct {
	TaskID       int64
	Title        string
	Category     string
	Frequency    string
	DueDate      string // YYYY-MM-DD, used in the form action URL
	DueDateLabel string // human label, e.g. "Today" or "Mon, Jan 15"
	Status       string // "pending" | "missed" | "completed"
	CSRFToken    string
}

type dashboardInnerView struct {
	Today        string
	TodayPending []taskRowView
	TodayDone    []taskRowView
	Missed       []taskRowView
}

type dashboardPageView struct {
	BaseView
	Inner dashboardInnerView
}

// Home renders the full dashboard page.
func (h *DashboardHandler) Home(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	inner, err := h.buildInner(r, user, token)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	data := dashboardPageView{
		BaseView: BaseView{
			Title:     "Mutaba'ah Tracker",
			UserName:  user.Name,
			UserRole:  string(user.Role),
			CSRFToken: token,
		},
		Inner: inner,
	}
	if err := h.tmpl.Render(w, "dashboard/index.html", data); err != nil {
		h.errs.ServerError(w, r, err)
	}
}

// ToggleComplete flips the completion state for a single (task, date) and
// returns the refreshed dashboard inner block as an HTMX partial.
func (h *DashboardHandler) ToggleComplete(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)
	today := todayFor(h.now)

	idStr := chi.URLParam(r, "id")
	taskID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "bad task id", http.StatusBadRequest)
		return
	}
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		dateStr = r.PostFormValue("date")
	}
	dueDate, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
	if err != nil {
		http.Error(w, "bad date", http.StatusBadRequest)
		return
	}

	if _, err := h.tasks.ToggleAsOf(r.Context(), taskID, user.ID, dueDate, today); err != nil {
		if errors.Is(err, services.ErrTaskNotAvailable) || errors.Is(err, services.ErrInvalidOccurrenceDate) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		h.errs.ServerError(w, r, err)
		return
	}

	inner, err := h.buildInner(r, user, token)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}
	if err := h.tmpl.RenderPartial(w, "dashboard/_dashboard_inner.html", inner); err != nil {
		h.errs.ServerError(w, r, err)
	}
}

func (h *DashboardHandler) buildInner(r *http.Request, user repository.User, csrfToken string) (dashboardInnerView, error) {
	today := todayFor(h.now)

	todayOccs, err := h.tasks.OccurrencesOnAsOf(r.Context(), user.ID, today, today)
	if err != nil {
		return dashboardInnerView{}, err
	}
	missedFrom := today.AddDate(0, 0, -missedWindowDays)
	missedTo := today.AddDate(0, 0, -1)
	missedRange, err := h.tasks.OccurrencesBetweenAsOf(r.Context(), user.ID, missedFrom, missedTo, today)
	if err != nil {
		return dashboardInnerView{}, err
	}

	view := dashboardInnerView{
		Today: today.Format("Mon, 02 Jan 2006"),
	}
	for _, occ := range todayOccs {
		row := rowFromOccurrence(occ, today, csrfToken)
		if row.Status == "completed" {
			view.TodayDone = append(view.TodayDone, row)
		} else {
			view.TodayPending = append(view.TodayPending, row)
		}
	}
	for _, occ := range missedRange {
		if occ.Status != services.StatusMissed {
			continue
		}
		view.Missed = append(view.Missed, rowFromOccurrence(occ, today, csrfToken))
	}
	return view, nil
}

func rowFromOccurrence(occ services.TaskOccurrence, today time.Time, csrfToken string) taskRowView {
	label := occ.DueDate.Format("Mon, 02 Jan")
	if occ.DueDate.Equal(today) {
		label = "Today"
	}
	var category string
	if occ.Task.Category.Valid {
		category = occ.Task.Category.String
	}
	return taskRowView{
		TaskID:       occ.Task.ID,
		Title:        occ.Task.Title,
		Category:     category,
		Frequency:    string(occ.Task.Frequency),
		DueDate:      occ.DueDate.Format("2006-01-02"),
		DueDateLabel: label,
		Status:       string(occ.Status),
		CSRFToken:    csrfToken,
	}
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
