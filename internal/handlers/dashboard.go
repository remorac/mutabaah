package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
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
	auth     *services.AuthService
	tasks    *services.TaskService
	settings *services.AppSettingsService
	tmpl     *Templates
	errs     *ErrorPages
	logger   *slog.Logger
	now      func() time.Time
}

func NewDashboardHandler(auth *services.AuthService, tasks *services.TaskService, settings *services.AppSettingsService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *DashboardHandler {
	return &DashboardHandler{auth: auth, tasks: tasks, settings: settings, tmpl: tmpl, errs: errs, logger: logger, now: time.Now}
}

type taskRowView struct {
	TaskID      int64
	Title       string
	Description string
	Frequency   string
	DueDate     string // YYYY-MM-DD, used in the form action URL
	Status      string // "pending" | "missed" | "completed" | "exempt"
	CSRFToken   string
	Section     string // "today" | "missed"
	RowID       string
}

type missedGroupView struct {
	Date      string // YYYY-MM-DD
	DateLabel string // human label, e.g. "Mon, 25 May 2026"
	Rows      []taskRowView
}

type dashboardStatsView struct {
	TodayTotal   int
	TodayDone    int
	TodayPct     int
	WeeklyTotal  int
	WeeklyDone   int
	WeeklyPct    int
	Streak       int
	PendingCount int
}

type dashboardInnerView struct {
	Today        string
	TodayPending []taskRowView
	TodayDone    []taskRowView
	Missed       []missedGroupView
	Stats        dashboardStatsView
	MissedLabel  string
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
		BaseView: NewBaseView(user, token, "Mutaba'ah Yaumiyah"),
		Inner:    inner,
	}
	if err := h.tmpl.Render(w, "dashboard/index.html", data); err != nil {
		h.errs.ServerError(w, r, err)
	}
}

// ToggleComplete flips the completion state for a single (task, date).
// AJAX/HTMX requests receive the refreshed dashboard inner block; regular
// form posts redirect back to the dashboard.
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

	if !isPartialRequest(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	inner, err := h.buildInner(r, user, token)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}
	if err := h.renderToggleFragments(w, inner, taskID, dueDate, today); err != nil {
		h.errs.ServerError(w, r, err)
	}
}

func (h *DashboardHandler) renderToggleFragments(w http.ResponseWriter, inner dashboardInnerView, taskID int64, dueDate, today time.Time) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.renderStatFragments(w, inner); err != nil {
		return err
	}
	if dateOnly(dueDate).Equal(dateOnly(today)) {
		return h.renderTodayToggleFragments(w, inner, taskID, dueDate)
	}

	rowID := taskRowID(taskID, dueDate)
	for _, group := range inner.Missed {
		for _, row := range group.Rows {
			if row.TaskID == taskID && row.DueDate == dueDate.Format("2006-01-02") {
				return h.tmpl.RenderPartial(w, "dashboard/_task_row.html", row)
			}
		}
	}
	_, err := fmt.Fprintf(w, `<li id="%s" data-swap-target="#%s" hidden></li>`, rowID, rowID)
	return err
}

func (h *DashboardHandler) renderStatFragments(w http.ResponseWriter, inner dashboardInnerView) error {
	for _, name := range []string{
		"dashboard/_stat_today_data.html",
		"dashboard/_stat_pending_data.html",
		"dashboard/_stat_week_data.html",
		"dashboard/_stat_streak_data.html",
	} {
		if err := h.tmpl.RenderPartial(w, name, inner); err != nil {
			return err
		}
	}
	return nil
}

func (h *DashboardHandler) renderTodayToggleFragments(w http.ResponseWriter, inner dashboardInnerView, taskID int64, dueDate time.Time) error {
	for _, name := range []string{
		"dashboard/_today_pending_count.html",
		"dashboard/_today_done_count.html",
		"dashboard/_today_pending_empty.html",
		"dashboard/_today_done_empty.html",
	} {
		if err := h.tmpl.RenderPartial(w, name, inner); err != nil {
			return err
		}
	}

	rowID := taskRowID(taskID, dueDate)
	row, listID, beforeID, ok := findTodayRowPlacement(inner, taskID, dueDate)
	if !ok {
		_, err := fmt.Fprintf(w, `<template data-remove-target="#%s"></template>`, rowID)
		return err
	}
	if _, err := fmt.Fprintf(w, `<template data-remove-target="#%s"></template>`, rowID); err != nil {
		return err
	}
	beforeAttr := ""
	if beforeID != "" {
		beforeAttr = fmt.Sprintf(` data-insert-before="#%s"`, beforeID)
	}
	if _, err := fmt.Fprintf(w, `<template data-insert-target="#%s"%s>`, listID, beforeAttr); err != nil {
		return err
	}
	if err := h.tmpl.RenderPartial(w, "dashboard/_task_row.html", row); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, `</template>`)
	return err
}

func findTodayRowPlacement(inner dashboardInnerView, taskID int64, dueDate time.Time) (taskRowView, string, string, bool) {
	dueDateStr := dueDate.Format("2006-01-02")
	if row, beforeID, ok := findRowPlacement(inner.TodayPending, taskID, dueDateStr); ok {
		return row, "today-pending-list", beforeID, true
	}
	if row, beforeID, ok := findRowPlacement(inner.TodayDone, taskID, dueDateStr); ok {
		return row, "today-done-list", beforeID, true
	}
	return taskRowView{}, "", "", false
}

func findRowPlacement(rows []taskRowView, taskID int64, dueDate string) (taskRowView, string, bool) {
	for i, row := range rows {
		if row.TaskID == taskID && row.DueDate == dueDate {
			beforeID := ""
			if i+1 < len(rows) {
				beforeID = rows[i+1].RowID
			}
			return row, beforeID, true
		}
	}
	return taskRowView{}, "", false
}

func (h *DashboardHandler) buildInner(r *http.Request, user repository.User, csrfToken string) (dashboardInnerView, error) {
	today := todayFor(h.now)
	settings, err := h.settings.Get(r.Context())
	if err != nil {
		return dashboardInnerView{}, err
	}

	todayOccs, err := h.tasks.OccurrencesOnAsOf(r.Context(), user.ID, today, today)
	if err != nil {
		return dashboardInnerView{}, err
	}
	missedDates := services.WeekDatesForHistory(h.now(), settings.WeekStartDay, settings.HistoryWeeks, false)
	missedTo := today.AddDate(0, 0, -1)
	var missedRange []services.TaskOccurrence
	if len(missedDates) > 0 {
		missedFrom := missedDates[0]
		missedRange, err = h.tasks.OccurrencesBetweenAsOf(r.Context(), user.ID, missedFrom, missedTo, today)
		if err != nil {
			return dashboardInnerView{}, err
		}
	}

	view := dashboardInnerView{
		Today:       today.Format("Mon, 02 Jan 2006"),
		MissedLabel: missedRangeLabel(settings.HistoryWeeks),
	}
	for _, occ := range todayOccs {
		row := rowFromOccurrence(occ, csrfToken, "today")
		if row.Status == "completed" || row.Status == "exempt" {
			view.TodayDone = append(view.TodayDone, row)
		} else {
			view.TodayPending = append(view.TodayPending, row)
		}
	}
	groupIdx := map[string]int{}
	for _, occ := range missedRange {
		if !showInMissedSection(occ) {
			continue
		}
		key := occ.DueDate.Format("2006-01-02")
		idx, ok := groupIdx[key]
		if !ok {
			view.Missed = append(view.Missed, missedGroupView{
				Date:      key,
				DateLabel: occ.DueDate.Format("Mon, 02 Jan 2006"),
			})
			idx = len(view.Missed) - 1
			groupIdx[key] = idx
		}
		view.Missed[idx].Rows = append(view.Missed[idx].Rows, rowFromOccurrence(occ, csrfToken, "missed"))
	}
	sortMissedGroupsByDateDesc(view.Missed)

	// Compute stats
	view.Stats = h.computeStats(r.Context(), user.ID, today, todayOccs, settings.WeekStartDay)

	return view, nil
}

func (h *DashboardHandler) computeStats(ctx context.Context, userID int64, today time.Time, todayOccs []services.TaskOccurrence, weekStartDay time.Weekday) dashboardStatsView {
	stats := dashboardStatsView{}

	// Today stats — exempt occurrences are excluded from numerator AND denominator.
	for _, occ := range todayOccs {
		if occ.Status == services.StatusExempt {
			continue
		}
		stats.TodayTotal++
		if occ.Status == services.StatusCompleted {
			stats.TodayDone++
		}
	}
	stats.PendingCount = stats.TodayTotal - stats.TodayDone
	if stats.TodayTotal > 0 {
		stats.TodayPct = (stats.TodayDone * 100) / stats.TodayTotal
	}

	// Weekly stats (configured week start to today inclusive) — also excludes exempt.
	weekStart := services.WeekStartFor(today, weekStartDay)
	weekOccs, err := h.tasks.OccurrencesBetweenAsOf(ctx, userID, weekStart, today, today)
	if err == nil {
		for _, occ := range weekOccs {
			if occ.Status == services.StatusExempt {
				continue
			}
			stats.WeeklyTotal++
			if occ.Status == services.StatusCompleted {
				stats.WeeklyDone++
			}
		}
		if stats.WeeklyTotal > 0 {
			stats.WeeklyPct = (stats.WeeklyDone * 100) / stats.WeeklyTotal
		}
	}

	// Streak: consecutive days of 100% completion (looking backwards)
	for offset := 0; offset < 365; offset++ {
		d := today.AddDate(0, 0, -offset)
		occs, err := h.tasks.OccurrencesOnAsOf(ctx, userID, d, today)
		if err != nil {
			break
		}
		if len(occs) == 0 {
			if offset == 0 {
				continue // today has no tasks, keep checking
			}
			break
		}
		allDone := true
		for _, occ := range occs {
			if occ.Status != services.StatusCompleted && occ.Status != services.StatusExempt {
				allDone = false
				break
			}
		}
		if allDone {
			stats.Streak++
		} else if offset == 0 {
			// today not done yet, don't count but keep checking yesterday
			continue
		} else {
			break
		}
	}

	return stats
}

func sortMissedGroupsByDateDesc(groups []missedGroupView) {
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].Date > groups[j].Date
	})
}

func showInMissedSection(occ services.TaskOccurrence) bool {
	if occ.Status == services.StatusMissed {
		return true
	}
	if occ.Status != services.StatusCompleted {
		return false
	}

	completed := occ.CompletedAt.In(services.AppLocation)
	completedDate := time.Date(completed.Year(), completed.Month(), completed.Day(), 0, 0, 0, 0, time.UTC)
	return completedDate.After(dateOnly(occ.DueDate))
}

func missedRangeLabel(historyWeeks int) string {
	if historyWeeks <= 1 {
		return "this week"
	}
	return fmt.Sprintf("last %d weeks", historyWeeks)
}

func rowFromOccurrence(occ services.TaskOccurrence, csrfToken, section string) taskRowView {
	var desc string
	if occ.Task.Description.Valid {
		desc = occ.Task.Description.String
	}
	return taskRowView{
		TaskID:      occ.Task.ID,
		Title:       occ.Task.Title,
		Description: desc,
		Frequency:   string(occ.Task.Frequency),
		DueDate:     occ.DueDate.Format("2006-01-02"),
		Status:      string(occ.Status),
		CSRFToken:   csrfToken,
		Section:     section,
		RowID:       taskRowID(occ.Task.ID, occ.DueDate),
	}
}

func taskRowID(taskID int64, dueDate time.Time) string {
	return fmt.Sprintf("task-row-%d-%s", taskID, dateOnly(dueDate).Format("2006-01-02"))
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
