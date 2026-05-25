package handlers

import (
	"log/slog"
	"net/http"
	"time"

	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/services"
)

// CalendarHandler renders a monthly calendar grid and per-day detail panels.
//
// The grid view fetches occurrences for the full visible window (which may
// extend a few days into the previous/next month so weeks line up) in a single
// service call, then buckets them into day cells. Each cell carries a
// completion ratio used to render a heatmap-style intensity.
type CalendarHandler struct {
	auth   *services.AuthService
	tasks  *services.TaskService
	tmpl   *Templates
	errs   *ErrorPages
	logger *slog.Logger
	now    func() time.Time
}

func NewCalendarHandler(auth *services.AuthService, tasks *services.TaskService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *CalendarHandler {
	return &CalendarHandler{auth: auth, tasks: tasks, tmpl: tmpl, errs: errs, logger: logger, now: time.Now}
}

type calendarCellView struct {
	Date         string // YYYY-MM-DD
	DayNum       int
	InMonth      bool
	IsToday      bool
	IsFuture     bool
	Completed    int
	Total        int
	IntensityCls string // tailwind bg class for heatmap shading
}

type calendarGridView struct {
	MonthLabel string
	PrevMonth  string // YYYY-MM
	NextMonth  string // YYYY-MM
	TodayMonth string // YYYY-MM
	Selected   string // YYYY-MM-DD
	Weekdays   []string
	Weeks      [][]calendarCellView
}

type calendarPageView struct {
	BaseView
	Grid calendarGridView
	Day  calendarDayView
}

type calendarDayTaskView struct {
	TaskID      int64
	Title       string
	Description string
	Frequency   string
	Status      string
}

type calendarDayView struct {
	Date      string // YYYY-MM-DD
	DateLabel string
	Tasks     []calendarDayTaskView
	Completed int
	Total     int
}

// Month renders the full calendar page for the requested month. ?month=YYYY-MM
// defaults to the current month when missing or unparseable.
func (h *CalendarHandler) Month(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	today := todayFor(h.now)
	month := parseMonth(r.URL.Query().Get("month"), today)

	grid, err := h.buildGrid(r, user.ID, month, today)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}
	day, err := h.buildDay(r, user.ID, today, today)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	data := calendarPageView{
		BaseView: NewBaseView(user, token, "Calendar — Mutaba'ah Tracker"),
		Grid:     grid,
		Day:      day,
	}
	if err := h.tmpl.Render(w, "calendar/index.html", data); err != nil {
		h.errs.ServerError(w, r, err)
	}
}

// Day returns an HTMX partial listing all occurrences for a single date with
// their statuses. Used to populate the side panel when a cell is clicked.
func (h *CalendarHandler) Day(w http.ResponseWriter, r *http.Request) {
	user, _ := apmw.UserFromContext(r.Context())

	dateStr := r.URL.Query().Get("date")
	date, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
	if err != nil {
		http.Error(w, "bad date", http.StatusBadRequest)
		return
	}
	date = dateOnly(date)

	today := todayFor(h.now)
	view, err := h.buildDay(r, user.ID, date, today)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	if err := h.tmpl.RenderPartial(w, "calendar/_day.html", view); err != nil {
		h.errs.ServerError(w, r, err)
	}
}

func (h *CalendarHandler) buildDay(r *http.Request, userID int64, date, today time.Time) (calendarDayView, error) {
	occs, err := h.tasks.OccurrencesOnAsOf(r.Context(), userID, date, today)
	if err != nil {
		return calendarDayView{}, err
	}
	return calendarDayFromOccurrences(date, occs), nil
}

func calendarDayFromOccurrences(date time.Time, occs []services.TaskOccurrence) calendarDayView {
	view := calendarDayView{
		Date:      date.Format("2006-01-02"),
		DateLabel: date.Format("Mon, 02 Jan 2006"),
	}
	for _, occ := range occs {
		var desc string
		if occ.Task.Description.Valid {
			desc = occ.Task.Description.String
		}
		view.Tasks = append(view.Tasks, calendarDayTaskView{
			TaskID:      occ.Task.ID,
			Title:       occ.Task.Title,
			Description: desc,
			Frequency:   string(occ.Task.Frequency),
			Status:      string(occ.Status),
		})
		if occ.Status == services.StatusExempt {
			continue
		}
		view.Total++
		if occ.Status == services.StatusCompleted {
			view.Completed++
		}
	}
	return view
}

func (h *CalendarHandler) buildGrid(r *http.Request, userID int64, month, today time.Time) (calendarGridView, error) {
	monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, -1)

	// Visible window: pad to whole weeks (Sun..Sat).
	leading := int(monthStart.Weekday()) // Sunday = 0
	gridStart := monthStart.AddDate(0, 0, -leading)
	trailing := 6 - int(monthEnd.Weekday())
	gridEnd := monthEnd.AddDate(0, 0, trailing)

	occs, err := h.tasks.OccurrencesBetweenAsOf(r.Context(), userID, gridStart, gridEnd, today)
	if err != nil {
		return calendarGridView{}, err
	}

	type tally struct{ done, total int }
	byDay := make(map[string]*tally, 42)
	for _, occ := range occs {
		if occ.Status == services.StatusExempt {
			continue
		}
		k := occ.DueDate.Format("2006-01-02")
		t, ok := byDay[k]
		if !ok {
			t = &tally{}
			byDay[k] = t
		}
		t.total++
		if occ.Status == services.StatusCompleted {
			t.done++
		}
	}

	prev := monthStart.AddDate(0, -1, 0)
	next := monthStart.AddDate(0, 1, 0)
	grid := calendarGridView{
		MonthLabel: monthStart.Format("January 2006"),
		PrevMonth:  prev.Format("2006-01"),
		NextMonth:  next.Format("2006-01"),
		TodayMonth: today.Format("2006-01"),
		Selected:   today.Format("2006-01-02"),
		Weekdays:   []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
	}

	var week []calendarCellView
	for d := gridStart; !d.After(gridEnd); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		t := byDay[key]
		var done, total int
		if t != nil {
			done, total = t.done, t.total
		}
		cell := calendarCellView{
			Date:         key,
			DayNum:       d.Day(),
			InMonth:      d.Month() == monthStart.Month(),
			IsToday:      d.Equal(today),
			IsFuture:     d.After(today),
			Completed:    done,
			Total:        total,
			IntensityCls: heatmapClass(done, total, d.After(today)),
		}
		week = append(week, cell)
		if len(week) == 7 {
			grid.Weeks = append(grid.Weeks, week)
			week = nil
		}
	}
	return grid, nil
}

// heatmapClass returns a Tailwind background class for a cell based on how
// many of its due occurrences were completed. Future dates render flat so
// they don't look like missed days.
func heatmapClass(done, total int, future bool) string {
	if total == 0 {
		return ""
	}
	if future {
		return "bg-base-200/70"
	}
	if done == 0 {
		return "bg-error/10 text-error"
	}
	ratio := float64(done) / float64(total)
	switch {
	case ratio >= 1.0:
		return "bg-success/30 text-success"
	case ratio >= 0.75:
		return "bg-success/22 text-success/80"
	case ratio >= 0.5:
		return "bg-success/16 text-success/70"
	case ratio >= 0.25:
		return "bg-success/10 text-success/60"
	default:
		return "bg-success/6 text-success/50"
	}
}

// parseMonth accepts "YYYY-MM" and falls back to fallback's month on error.
func parseMonth(s string, fallback time.Time) time.Time {
	if s == "" {
		return time.Date(fallback.Year(), fallback.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	t, err := time.ParseInLocation("2006-01", s, time.UTC)
	if err != nil {
		return time.Date(fallback.Year(), fallback.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return t
}
