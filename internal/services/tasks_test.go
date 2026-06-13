package services

import (
	"database/sql"
	"testing"
	"time"

	"github.com/remorac/mutabaah/internal/repository"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func task(id int64, title string, freq repository.TasksFrequency, start time.Time, end *time.Time) repository.Task {
	t := repository.Task{ID: id, Title: title, Frequency: freq, StartDate: start, Active: true}
	if end != nil {
		t.EndDate = sql.NullTime{Time: *end, Valid: true}
	}
	return t
}

func datesOf(occs []time.Time) []string {
	out := make([]string, len(occs))
	for i, d := range occs {
		out[i] = d.Format("2006-01-02")
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("at %d: got %s, want %s (full got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}

func TestOccurrenceDates_Daily(t *testing.T) {
	tk := task(1, "dhikr", repository.TasksFrequencyDaily, date(2026, 1, 1), nil)
	got := datesOf(occurrenceDates(tk, date(2026, 1, 3), date(2026, 1, 5)))
	eq(t, got, []string{"2026-01-03", "2026-01-04", "2026-01-05"})
}

func TestOccurrenceDates_Daily_ClampsToStart(t *testing.T) {
	tk := task(1, "dhikr", repository.TasksFrequencyDaily, date(2026, 1, 4), nil)
	got := datesOf(occurrenceDates(tk, date(2026, 1, 1), date(2026, 1, 5)))
	eq(t, got, []string{"2026-01-04", "2026-01-05"})
}

func TestOccurrenceDates_Daily_HonoursEndDate(t *testing.T) {
	end := date(2026, 1, 3)
	tk := task(1, "dhikr", repository.TasksFrequencyDaily, date(2026, 1, 1), &end)
	got := datesOf(occurrenceDates(tk, date(2026, 1, 1), date(2026, 1, 10)))
	eq(t, got, []string{"2026-01-01", "2026-01-02", "2026-01-03"})
}

func TestOccurrenceDates_Weekly(t *testing.T) {
	tk := task(1, "kahf", repository.TasksFrequencyWeekly, date(2026, 1, 2), nil) // Friday
	got := datesOf(occurrenceDates(tk, date(2026, 1, 1), date(2026, 2, 1)))
	eq(t, got, []string{"2026-01-02", "2026-01-09", "2026-01-16", "2026-01-23", "2026-01-30"})
}

func TestOccurrenceDates_Weekly_RangeOffsetFromStart(t *testing.T) {
	tk := task(1, "kahf", repository.TasksFrequencyWeekly, date(2026, 1, 2), nil)
	// Range starts mid-cycle; first occurrence in range should snap forward.
	got := datesOf(occurrenceDates(tk, date(2026, 1, 5), date(2026, 1, 20)))
	eq(t, got, []string{"2026-01-09", "2026-01-16"})
}

func TestOccurrenceDates_Monthly_NormalDay(t *testing.T) {
	tk := task(1, "khatam", repository.TasksFrequencyMonthly, date(2026, 1, 15), nil)
	got := datesOf(occurrenceDates(tk, date(2026, 1, 1), date(2026, 4, 30)))
	eq(t, got, []string{"2026-01-15", "2026-02-15", "2026-03-15", "2026-04-15"})
}

func TestOccurrenceDates_Monthly_SkipsShortMonths(t *testing.T) {
	tk := task(1, "report", repository.TasksFrequencyMonthly, date(2026, 1, 31), nil)
	got := datesOf(occurrenceDates(tk, date(2026, 1, 1), date(2026, 6, 30)))
	// Feb, Apr, Jun have no 31st; should be skipped.
	eq(t, got, []string{"2026-01-31", "2026-03-31", "2026-05-31"})
}

func TestOccurrenceDates_Yearly_LeapDay(t *testing.T) {
	tk := task(1, "leap", repository.TasksFrequencyYearly, date(2024, 2, 29), nil) // 2024 is leap
	got := datesOf(occurrenceDates(tk, date(2024, 1, 1), date(2029, 12, 31)))
	// 2024 leap, skip 2025/26/27, fire 2028, skip 2029.
	eq(t, got, []string{"2024-02-29", "2028-02-29"})
}

func TestOccurrenceDates_Yearly_NormalDay(t *testing.T) {
	tk := task(1, "anniv", repository.TasksFrequencyYearly, date(2026, 6, 10), nil)
	got := datesOf(occurrenceDates(tk, date(2026, 1, 1), date(2028, 12, 31)))
	eq(t, got, []string{"2026-06-10", "2027-06-10", "2028-06-10"})
}

func TestOccurrenceDates_BeforeStart_Empty(t *testing.T) {
	tk := task(1, "x", repository.TasksFrequencyDaily, date(2026, 6, 1), nil)
	got := datesOf(occurrenceDates(tk, date(2026, 1, 1), date(2026, 5, 31)))
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestBuildOccurrences_StatusAssignment(t *testing.T) {
	today := date(2026, 5, 25)
	tk := task(1, "dhikr", repository.TasksFrequencyDaily, date(2026, 5, 23), nil)
	completions := []repository.TaskCompletion{
		{TaskID: 1, UserID: 1, DueDate: date(2026, 5, 23), CompletedAt: date(2026, 5, 23)},
	}
	got := buildOccurrences([]repository.Task{tk}, completions, nil, today, date(2026, 5, 23), date(2026, 5, 26))
	if len(got) != 4 {
		t.Fatalf("expected 4 occurrences, got %d", len(got))
	}
	wantStatuses := []OccurrenceStatus{StatusCompleted, StatusMissed, StatusPending, StatusPending}
	for i, occ := range got {
		if occ.Status != wantStatuses[i] {
			t.Errorf("occurrence %d (%s): got %s, want %s",
				i, occ.DueDate.Format("2006-01-02"), occ.Status, wantStatuses[i])
		}
	}
}

func TestBuildOccurrences_ExemptOverridesCompletionInPeriod(t *testing.T) {
	today := date(2026, 5, 30)
	// Exempt task, completed on the 25th; a period later entered covers 24th-26th.
	exempt := task(1, "dhikr", repository.TasksFrequencyDaily, date(2026, 5, 24), nil)
	exempt.ExemptDuringMenses = true
	// Non-exempt task, also completed on the 25th, inside the same period.
	regular := task(2, "salah", repository.TasksFrequencyDaily, date(2026, 5, 24), nil)
	completions := []repository.TaskCompletion{
		{TaskID: 1, UserID: 1, DueDate: date(2026, 5, 25), CompletedAt: date(2026, 5, 25)},
		{TaskID: 1, UserID: 1, DueDate: date(2026, 5, 27), CompletedAt: date(2026, 5, 27)},
		{TaskID: 2, UserID: 1, DueDate: date(2026, 5, 25), CompletedAt: date(2026, 5, 25)},
	}
	periods := []repository.MensesPeriod{
		{StartDate: date(2026, 5, 24), EndDate: sql.NullTime{Time: date(2026, 5, 26), Valid: true}},
	}
	got := buildOccurrences([]repository.Task{exempt, regular}, completions, periods, today, date(2026, 5, 25), date(2026, 5, 27))

	type want struct {
		taskID int64
		day    string
		status OccurrenceStatus
	}
	wants := []want{
		{1, "2026-05-25", StatusExempt},    // exempt + completed + in period -> exempt
		{1, "2026-05-27", StatusCompleted}, // exempt + completed, outside period -> completed
		{2, "2026-05-25", StatusCompleted}, // non-exempt completed in period -> completed
	}
	for _, w := range wants {
		var found *TaskOccurrence
		for i := range got {
			if got[i].Task.ID == w.taskID && got[i].DueDate.Format("2006-01-02") == w.day {
				found = &got[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("no occurrence for task %d on %s", w.taskID, w.day)
		}
		if found.Status != w.status {
			t.Errorf("task %d on %s: got %s, want %s", w.taskID, w.day, found.Status, w.status)
		}
	}
}

func TestBuildOccurrences_SortedByDateThenTitle(t *testing.T) {
	today := date(2026, 5, 25)
	a := task(1, "zikr", repository.TasksFrequencyDaily, date(2026, 5, 25), nil)
	b := task(2, "alfatihah", repository.TasksFrequencyDaily, date(2026, 5, 25), nil)
	got := buildOccurrences([]repository.Task{a, b}, nil, nil, today, date(2026, 5, 25), date(2026, 5, 25))
	if len(got) != 2 || got[0].Task.Title != "alfatihah" || got[1].Task.Title != "zikr" {
		t.Fatalf("expected alfatihah before zikr, got %+v", got)
	}
}
