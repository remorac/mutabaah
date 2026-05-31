package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

type LeaderboardHandler struct {
	auth     *services.AuthService
	tasks    *services.TaskService
	users    *services.UserAdminService
	settings *services.AppSettingsService
	tmpl     *Templates
	errs     *ErrorPages
	logger   *slog.Logger
	now      func() time.Time
}

func NewLeaderboardHandler(auth *services.AuthService, tasks *services.TaskService, users *services.UserAdminService, settings *services.AppSettingsService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *LeaderboardHandler {
	return &LeaderboardHandler{auth: auth, tasks: tasks, users: users, settings: settings, tmpl: tmpl, errs: errs, logger: logger, now: time.Now}
}

type leaderboardRowView struct {
	Rank        int
	UserID      int64
	UserName    string
	AvatarPath  string
	IsCurrent   bool
	Completed   int
	Due         int
	Percent     int
	BestStreak  int
	ProgressPct int
}

type leaderboardPageView struct {
	BaseView
	Period         string
	PeriodLabel    string
	DateValue      string
	RangeLabel     string
	PreviousURL    string
	CurrentURL     string
	NextURL        string
	PrimaryRows    []leaderboardRowView
	StreakRows     []leaderboardRowView
	HasOccurrences bool
	TotalCompleted int
	TotalDue       int
	AveragePercent int
	TopCompleted   int
	TopStreak      int
}

type leaderboardPeriod struct {
	Kind       string
	Label      string
	Start      time.Time
	End        time.Time
	CountUntil time.Time
	DateValue  string
	PrevDate   string
	NextDate   string
	TodayDate  string
	RangeLabel string
}

// Show renders the two leaderboard rankings for all users.
func (h *LeaderboardHandler) Show(w http.ResponseWriter, r *http.Request) {
	current, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	view, err := h.buildPage(r, current, token)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}
	if err := h.tmpl.Render(w, "leaderboard/index.html", view); err != nil {
		h.errs.ServerError(w, r, err)
	}
}

// ExportPDF writes the currently filtered leaderboard rankings as a PDF.
func (h *LeaderboardHandler) ExportPDF(w http.ResponseWriter, r *http.Request) {
	current, _ := apmw.UserFromContext(r.Context())

	view, err := h.buildPage(r, current, "")
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	pdf, err := buildLeaderboardPDF(view)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	filename := leaderboardPDFFilename(view)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	_, _ = w.Write(pdf)
}

func (h *LeaderboardHandler) buildPage(r *http.Request, current repository.User, csrfToken string) (leaderboardPageView, error) {
	today := todayFor(h.now)
	settings, err := h.settings.Get(r.Context())
	if err != nil {
		return leaderboardPageView{}, err
	}
	period := parseLeaderboardPeriod(r.URL.Query().Get("period"), r.URL.Query().Get("date"), today, settings.WeekStartDay)

	users, _, err := h.users.List(r.Context(), 0, 0)
	if err != nil {
		return leaderboardPageView{}, err
	}

	sets := make([]leaderboardUserOccurrences, 0, len(users))
	for _, user := range users {
		occs, err := h.tasks.OccurrencesBetweenAsOf(r.Context(), user.ID, period.Start, period.CountUntil, today)
		if err != nil {
			return leaderboardPageView{}, err
		}
		sets = append(sets, leaderboardUserOccurrences{User: user, Occurrences: occs})
	}

	primary, streak, summary := buildLeaderboardRows(sets, current.ID, period.Start, period.CountUntil, today)
	return leaderboardPageView{
		BaseView:       NewBaseViewForRequest(r, current, csrfToken, "Leaderboard — Mutaba'ah Yaumiyah"),
		Period:         period.Kind,
		PeriodLabel:    period.Label,
		DateValue:      period.DateValue,
		RangeLabel:     period.RangeLabel,
		PreviousURL:    leaderboardURL(period.Kind, period.PrevDate),
		CurrentURL:     leaderboardURL(period.Kind, period.TodayDate),
		NextURL:        leaderboardURL(period.Kind, period.NextDate),
		PrimaryRows:    primary,
		StreakRows:     streak,
		HasOccurrences: summary.TotalDue > 0,
		TotalCompleted: summary.TotalCompleted,
		TotalDue:       summary.TotalDue,
		AveragePercent: percent(summary.TotalCompleted, summary.TotalDue),
		TopCompleted:   summary.TopCompleted,
		TopStreak:      summary.TopStreak,
	}, nil
}

type leaderboardUserOccurrences struct {
	User        repository.User
	Occurrences []services.TaskOccurrence
}

type leaderboardSummary struct {
	TotalCompleted int
	TotalDue       int
	TopCompleted   int
	TopStreak      int
}

func parseLeaderboardPeriod(kind, rawDate string, today time.Time, weekStartDay time.Weekday) leaderboardPeriod {
	today = dateOnly(today)
	selected := today
	if parsed, err := time.ParseInLocation("2006-01-02", rawDate, time.UTC); err == nil {
		selected = dateOnly(parsed)
	}

	periodKind := "week"
	if kind == "month" {
		periodKind = "month"
	}

	var start, end, prev, next time.Time
	label := "Weekly"
	if periodKind == "month" {
		start = time.Date(selected.Year(), selected.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, -1)
		prev = start.AddDate(0, -1, 0)
		next = start.AddDate(0, 1, 0)
		label = "Monthly"
	} else {
		start = services.WeekStartFor(selected, weekStartDay)
		end = start.AddDate(0, 0, 6)
		prev = start.AddDate(0, 0, -7)
		next = start.AddDate(0, 0, 7)
	}

	countUntil := end
	if countUntil.After(today) {
		countUntil = today
	}

	return leaderboardPeriod{
		Kind:       periodKind,
		Label:      label,
		Start:      start,
		End:        end,
		CountUntil: countUntil,
		DateValue:  start.Format("2006-01-02"),
		PrevDate:   prev.Format("2006-01-02"),
		NextDate:   next.Format("2006-01-02"),
		TodayDate:  today.Format("2006-01-02"),
		RangeLabel: fmt.Sprintf("%s - %s", start.Format("02 Jan 2006"), end.Format("02 Jan 2006")),
	}
}

func leaderboardURL(period, date string) string {
	return fmt.Sprintf("/leaderboard?period=%s&date=%s", period, date)
}

func buildLeaderboardRows(sets []leaderboardUserOccurrences, currentUserID int64, start, end, today time.Time) ([]leaderboardRowView, []leaderboardRowView, leaderboardSummary) {
	rows := make([]leaderboardRowView, 0, len(sets))
	summary := leaderboardSummary{}

	for _, set := range sets {
		row := leaderboardRowFromOccurrences(set.User, currentUserID, set.Occurrences, start, end, today)
		rows = append(rows, row)
		summary.TotalCompleted += row.Completed
		summary.TotalDue += row.Due
		if row.Completed > summary.TopCompleted {
			summary.TopCompleted = row.Completed
		}
		if row.BestStreak > summary.TopStreak {
			summary.TopStreak = row.BestStreak
		}
	}

	primary := append([]leaderboardRowView(nil), rows...)
	sort.SliceStable(primary, func(i, j int) bool {
		if primary[i].Completed != primary[j].Completed {
			return primary[i].Completed > primary[j].Completed
		}
		return leaderboardTieLess(primary[i], primary[j])
	})
	assignLeaderboardRanks(primary, func(a, b leaderboardRowView) bool {
		return a.Completed == b.Completed && a.Percent == b.Percent
	})

	streak := append([]leaderboardRowView(nil), rows...)
	sort.SliceStable(streak, func(i, j int) bool {
		if streak[i].BestStreak != streak[j].BestStreak {
			return streak[i].BestStreak > streak[j].BestStreak
		}
		if streak[i].Completed != streak[j].Completed {
			return streak[i].Completed > streak[j].Completed
		}
		return leaderboardTieLess(streak[i], streak[j])
	})
	assignLeaderboardRanks(streak, func(a, b leaderboardRowView) bool {
		return a.BestStreak == b.BestStreak && a.Completed == b.Completed && a.Percent == b.Percent
	})

	return primary, streak, summary
}

func leaderboardRowFromOccurrences(user repository.User, currentUserID int64, occs []services.TaskOccurrence, start, end, today time.Time) leaderboardRowView {
	row := leaderboardRowView{
		UserID:     user.ID,
		UserName:   user.Name,
		AvatarPath: leaderboardAvatarPath(user),
		IsCurrent:  user.ID == currentUserID,
	}

	dayStats := map[string]*leaderboardDayStats{}
	for _, occ := range occs {
		dueDate := dateOnly(occ.DueDate)
		if dueDate.Before(start) || dueDate.After(end) || dueDate.After(today) || occ.Status == services.StatusExempt {
			continue
		}

		row.Due++
		if occ.Status == services.StatusCompleted {
			row.Completed++
		}

		key := dueDate.Format("2006-01-02")
		stats, ok := dayStats[key]
		if !ok {
			stats = &leaderboardDayStats{}
			dayStats[key] = stats
		}
		stats.Due++
		if occ.Status == services.StatusCompleted {
			stats.Completed++
		}
	}

	row.Percent = percent(row.Completed, row.Due)
	row.ProgressPct = row.Percent
	row.BestStreak = bestLeaderboardStreak(start, end, today, dayStats)
	return row
}

type leaderboardDayStats struct {
	Completed int
	Due       int
}

func bestLeaderboardStreak(start, end, today time.Time, dayStats map[string]*leaderboardDayStats) int {
	if end.After(today) {
		end = today
	}

	best := 0
	current := 0
	for d := dateOnly(start); !d.After(end); d = d.AddDate(0, 0, 1) {
		stats := dayStats[d.Format("2006-01-02")]
		if stats != nil && stats.Due > 0 && stats.Completed == stats.Due {
			current++
			if current > best {
				best = current
			}
			continue
		}
		current = 0
	}
	return best
}

func leaderboardTieLess(a, b leaderboardRowView) bool {
	if a.Percent != b.Percent {
		return a.Percent > b.Percent
	}
	if a.UserName != b.UserName {
		return strings.ToLower(a.UserName) < strings.ToLower(b.UserName)
	}
	return a.UserID < b.UserID
}

func assignLeaderboardRanks(rows []leaderboardRowView, sameScore func(a, b leaderboardRowView) bool) {
	for i := range rows {
		if i > 0 && sameScore(rows[i], rows[i-1]) {
			rows[i].Rank = rows[i-1].Rank
			continue
		}
		rows[i].Rank = i + 1
	}
}

func leaderboardAvatarPath(user repository.User) string {
	if !user.AvatarPath.Valid || user.AvatarPath.String == "" {
		return ""
	}
	name := user.AvatarPath.String
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return "/static/avatars/thumb_" + base + ".jpg"
}
