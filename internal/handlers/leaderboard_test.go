package handlers

import (
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

func leaderboardDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func leaderboardTask(id int64, title string) repository.Task {
	return repository.Task{
		ID:          id,
		Title:       title,
		Description: sql.NullString{String: "practice", Valid: true},
		Frequency:   repository.TasksFrequencyDaily,
		Active:      true,
		StartDate:   leaderboardDate(2026, 5, 1),
	}
}

func leaderboardOcc(taskID int64, due time.Time, status services.OccurrenceStatus) services.TaskOccurrence {
	return services.TaskOccurrence{
		Task:    leaderboardTask(taskID, "Task"),
		DueDate: due,
		Status:  status,
	}
}

func TestParseLeaderboardPeriodDefaultsToCurrentWeek(t *testing.T) {
	today := leaderboardDate(2026, 5, 30)
	period := parseLeaderboardPeriod("bad", "not-a-date", today, time.Saturday)

	if period.Kind != "week" || period.Label != "Week" {
		t.Fatalf("period = %s/%s, want week/Week", period.Kind, period.Label)
	}
	if got := period.Start.Format("2006-01-02"); got != "2026-05-30" {
		t.Fatalf("start = %s, want 2026-05-30", got)
	}
	if got := period.End.Format("2006-01-02"); got != "2026-06-05" {
		t.Fatalf("end = %s, want 2026-06-05", got)
	}
	if got := period.CountUntil.Format("2006-01-02"); got != "2026-05-30" {
		t.Fatalf("countUntil = %s, want today cap 2026-05-30", got)
	}
}

func TestParseLeaderboardPeriodUsesConfiguredWeekStart(t *testing.T) {
	period := parseLeaderboardPeriod("week", "2026-05-30", leaderboardDate(2026, 6, 30), time.Monday)

	if got := period.Start.Format("2006-01-02"); got != "2026-05-25" {
		t.Fatalf("monday week start = %s, want 2026-05-25", got)
	}
	if got := period.End.Format("2006-01-02"); got != "2026-05-31" {
		t.Fatalf("monday week end = %s, want 2026-05-31", got)
	}
	if period.DateValue != "2026-05-25" {
		t.Fatalf("date value = %s, want normalized week start", period.DateValue)
	}
}

func TestParseLeaderboardPeriodMonthBounds(t *testing.T) {
	period := parseLeaderboardPeriod("month", "2026-02-14", leaderboardDate(2026, 12, 31), time.Saturday)

	if period.Kind != "month" || period.Label != "Month" {
		t.Fatalf("period = %s/%s, want month/Month", period.Kind, period.Label)
	}
	if got := period.Start.Format("2006-01-02"); got != "2026-02-01" {
		t.Fatalf("month start = %s, want 2026-02-01", got)
	}
	if got := period.End.Format("2006-01-02"); got != "2026-02-28" {
		t.Fatalf("month end = %s, want 2026-02-28", got)
	}
	if got := period.CountUntil.Format("2006-01-02"); got != "2026-02-28" {
		t.Fatalf("past month countUntil = %s, want full month end", got)
	}
}

func TestBuildLeaderboardRowsPrimaryRanking(t *testing.T) {
	today := leaderboardDate(2026, 5, 31)
	sets := []leaderboardUserOccurrences{
		{
			User: repository.User{ID: 1, Name: "Amina"},
			Occurrences: []services.TaskOccurrence{
				leaderboardOcc(1, leaderboardDate(2026, 5, 25), services.StatusCompleted),
				leaderboardOcc(2, leaderboardDate(2026, 5, 26), services.StatusCompleted),
				leaderboardOcc(3, leaderboardDate(2026, 5, 27), services.StatusMissed),
				leaderboardOcc(4, leaderboardDate(2026, 5, 28), services.StatusExempt),
			},
		},
		{
			User: repository.User{ID: 2, Name: "Bilal"},
			Occurrences: []services.TaskOccurrence{
				leaderboardOcc(1, leaderboardDate(2026, 5, 25), services.StatusCompleted),
				leaderboardOcc(2, leaderboardDate(2026, 5, 26), services.StatusCompleted),
			},
		},
		{
			User: repository.User{ID: 3, Name: "Citra"},
			Occurrences: []services.TaskOccurrence{
				leaderboardOcc(1, leaderboardDate(2026, 5, 25), services.StatusCompleted),
			},
		},
	}

	primary, _, summary := buildLeaderboardRows(sets, 2, leaderboardDate(2026, 5, 25), leaderboardDate(2026, 5, 31), today)

	if len(primary) != 3 {
		t.Fatalf("primary rows = %d, want 3", len(primary))
	}
	if primary[0].UserName != "Bilal" || primary[0].Completed != 2 || primary[0].Due != 2 || primary[0].Percent != 100 {
		t.Fatalf("first row = %+v, want Bilal 2/2 100%%", primary[0])
	}
	if primary[1].UserName != "Amina" || primary[1].Completed != 2 || primary[1].Due != 3 || primary[1].Percent != 67 {
		t.Fatalf("second row = %+v, want Amina 2/3 67%%", primary[1])
	}
	if !primary[0].IsCurrent {
		t.Fatalf("current user flag not set for Bilal: %+v", primary[0])
	}
	if summary.TotalCompleted != 5 || summary.TotalDue != 6 || summary.TopCompleted != 2 {
		t.Fatalf("summary = %+v, want completed 5 due 6 top 2", summary)
	}
}

func TestBuildLeaderboardRowsBestStreakRanking(t *testing.T) {
	today := leaderboardDate(2026, 5, 31)
	sets := []leaderboardUserOccurrences{
		{
			User: repository.User{ID: 1, Name: "Amina"},
			Occurrences: []services.TaskOccurrence{
				leaderboardOcc(1, leaderboardDate(2026, 5, 25), services.StatusCompleted),
				leaderboardOcc(2, leaderboardDate(2026, 5, 26), services.StatusCompleted),
				leaderboardOcc(3, leaderboardDate(2026, 5, 27), services.StatusMissed),
				leaderboardOcc(4, leaderboardDate(2026, 5, 28), services.StatusCompleted),
				leaderboardOcc(5, leaderboardDate(2026, 5, 29), services.StatusCompleted),
				leaderboardOcc(6, leaderboardDate(2026, 5, 30), services.StatusCompleted),
			},
		},
		{
			User: repository.User{ID: 2, Name: "Bilal"},
			Occurrences: []services.TaskOccurrence{
				leaderboardOcc(1, leaderboardDate(2026, 5, 25), services.StatusCompleted),
				leaderboardOcc(2, leaderboardDate(2026, 5, 26), services.StatusCompleted),
				leaderboardOcc(3, leaderboardDate(2026, 5, 27), services.StatusCompleted),
				leaderboardOcc(4, leaderboardDate(2026, 5, 28), services.StatusExempt),
				leaderboardOcc(5, leaderboardDate(2026, 5, 30), services.StatusMissed),
			},
		},
	}

	_, streak, summary := buildLeaderboardRows(sets, 1, leaderboardDate(2026, 5, 25), leaderboardDate(2026, 5, 31), today)

	if streak[0].UserName != "Amina" || streak[0].BestStreak != 3 {
		t.Fatalf("first streak row = %+v, want Amina best streak 3", streak[0])
	}
	if streak[1].UserName != "Bilal" || streak[1].BestStreak != 3 {
		t.Fatalf("second streak row = %+v, want Bilal best streak 3", streak[1])
	}
	if streak[0].Completed <= streak[1].Completed {
		t.Fatalf("streak tie should break by completed count: %+v", streak)
	}
	if summary.TopStreak != 3 {
		t.Fatalf("top streak = %d, want 3", summary.TopStreak)
	}
}

func TestBestLeaderboardStreakBreaksOnNoTaskAndExemptOnlyDays(t *testing.T) {
	stats := map[string]*leaderboardDayStats{
		"2026-05-25": {Completed: 1, Due: 1},
		"2026-05-27": {Completed: 1, Due: 1},
		"2026-05-28": {Completed: 2, Due: 2},
		"2026-05-29": {Completed: 1, Due: 2},
		"2026-05-30": {Completed: 1, Due: 1},
	}

	got := bestLeaderboardStreak(leaderboardDate(2026, 5, 25), leaderboardDate(2026, 5, 31), leaderboardDate(2026, 5, 31), stats)
	if got != 2 {
		t.Fatalf("best streak = %d, want 2", got)
	}
}

func TestLeaderboardTemplateRendersSectionsAndCurrentUser(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	view := leaderboardPageView{
		BaseView: NewBaseView(repository.User{
			ID:   2,
			Name: "Bilal",
			Role: repository.UsersRoleUser,
		}, "csrf-token", "Leaderboard — Mutaba'ah Yaumiyah"),
		Period:         "week",
		PeriodLabel:    "Week",
		DateValue:      "2026-05-30",
		RangeLabel:     "30 May 2026 - 05 Jun 2026",
		PreviousURL:    "/leaderboard?period=week&date=2026-05-23",
		CurrentURL:     "/leaderboard?period=week&date=2026-05-30",
		NextURL:        "/leaderboard?period=week&date=2026-06-06",
		HasOccurrences: true,
		TopCompleted:   2,
		TopStreak:      3,
		TotalCompleted: 3,
		TotalDue:       4,
		AveragePercent: 75,
		PrimaryRows: []leaderboardRowView{
			{Rank: 1, UserName: "Bilal", IsCurrent: true, Completed: 2, Due: 2, Percent: 100, ProgressPct: 100},
		},
		StreakRows: []leaderboardRowView{
			{Rank: 1, UserName: "Amina", BestStreak: 3, Completed: 1, Due: 2, Percent: 50, ProgressPct: 50},
		},
	}
	rec := httptest.NewRecorder()

	if err := tmpl.Render(rec, "leaderboard/index.html", view); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`Leaderboard`,
		`Task Done`,
		`Best Streak`,
		`name="period"`,
		`name="date"`,
		`value="2026-05-30"`,
		`Bilal`,
		`You`,
		`Amina`,
		`3 days`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered leaderboard missing %s: %s", want, body)
		}
	}
}

func TestLeaderboardTemplateRendersEmptyState(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	view := leaderboardPageView{
		BaseView: NewBaseView(repository.User{
			ID:   1,
			Name: "Amina",
			Role: repository.UsersRoleUser,
		}, "csrf-token", "Leaderboard — Mutaba'ah Yaumiyah"),
		Period:         "month",
		PeriodLabel:    "Month",
		DateValue:      "2026-05-01",
		RangeLabel:     "01 May 2026 - 31 May 2026",
		PreviousURL:    "/leaderboard?period=month&date=2026-04-01",
		CurrentURL:     "/leaderboard?period=month&date=2026-05-30",
		NextURL:        "/leaderboard?period=month&date=2026-06-01",
		PrimaryRows:    []leaderboardRowView{{Rank: 1, UserName: "Amina"}},
		StreakRows:     []leaderboardRowView{{Rank: 1, UserName: "Amina"}},
		TopCompleted:   0,
		TopStreak:      0,
		TotalDue:       0,
		AveragePercent: 0,
	}
	rec := httptest.NewRecorder()

	if err := tmpl.Render(rec, "leaderboard/index.html", view); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "No task occurrences found for this period.") {
		t.Fatalf("rendered leaderboard missing empty state: %s", body)
	}
}
