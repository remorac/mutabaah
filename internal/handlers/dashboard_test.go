package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aldoerianda/tracker/internal/services"
)

func dashboardDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestShowInMissedSection(t *testing.T) {
	dueDate := dashboardDate(2026, 5, 24)

	tests := []struct {
		name string
		occ  services.TaskOccurrence
		want bool
	}{
		{
			name: "missed occurrence",
			occ: services.TaskOccurrence{
				DueDate: dueDate,
				Status:  services.StatusMissed,
			},
			want: true,
		},
		{
			name: "completed after due date",
			occ: services.TaskOccurrence{
				DueDate:     dueDate,
				Status:      services.StatusCompleted,
				CompletedAt: time.Date(2026, 5, 25, 8, 0, 0, 0, services.AppLocation),
			},
			want: true,
		},
		{
			name: "completed after due date in app timezone",
			occ: services.TaskOccurrence{
				DueDate:     dueDate,
				Status:      services.StatusCompleted,
				CompletedAt: time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC),
			},
			want: true,
		},
		{
			name: "completed on due date",
			occ: services.TaskOccurrence{
				DueDate:     dueDate,
				Status:      services.StatusCompleted,
				CompletedAt: time.Date(2026, 5, 24, 23, 0, 0, 0, services.AppLocation),
			},
			want: false,
		},
		{
			name: "pending occurrence",
			occ: services.TaskOccurrence{
				DueDate: dueDate,
				Status:  services.StatusPending,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := showInMissedSection(tt.occ); got != tt.want {
				t.Fatalf("showInMissedSection() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSaturdayWeekStart(t *testing.T) {
	tests := []struct {
		name string
		day  time.Time
		want time.Time
	}{
		{
			name: "saturday starts same day",
			day:  dashboardDate(2026, 5, 23),
			want: dashboardDate(2026, 5, 23),
		},
		{
			name: "monday starts previous saturday",
			day:  dashboardDate(2026, 5, 25),
			want: dashboardDate(2026, 5, 23),
		},
		{
			name: "friday starts previous saturday",
			day:  dashboardDate(2026, 5, 29),
			want: dashboardDate(2026, 5, 23),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := saturdayWeekStart(tt.day); !got.Equal(tt.want) {
				t.Fatalf("saturdayWeekStart() = %s, want %s", got.Format("2006-01-02"), tt.want.Format("2006-01-02"))
			}
		})
	}
}

func TestSortMissedGroupsByDateDesc(t *testing.T) {
	groups := []missedGroupView{
		{Date: "2026-05-23"},
		{Date: "2026-05-25"},
		{Date: "2026-05-24"},
	}

	sortMissedGroupsByDateDesc(groups)

	want := []string{"2026-05-25", "2026-05-24", "2026-05-23"}
	for i, group := range groups {
		if group.Date != want[i] {
			t.Fatalf("groups[%d].Date = %s, want %s", i, group.Date, want[i])
		}
	}
}

func TestRenderToggleFragmentsForToday(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	handler := &DashboardHandler{tmpl: tmpl}
	today := dashboardDate(2026, 5, 25)
	inner := dashboardInnerView{
		Today: "Mon, 25 May 2026",
		TodayPending: []taskRowView{{
			TaskID:    1,
			Title:     "Read",
			Frequency: "daily",
			DueDate:   "2026-05-25",
			Status:    "pending",
			Section:   "today",
			RowID:     "task-row-1-2026-05-25",
		}},
		Stats: dashboardStatsView{TodayTotal: 1, PendingCount: 1, WeeklyTotal: 1},
	}
	rec := httptest.NewRecorder()

	if err := handler.renderToggleFragments(rec, inner, 1, today, today); err != nil {
		t.Fatalf("renderToggleFragments() error = %v", err)
	}

	body := rec.Body.String()
	for _, unwanted := range []string{`id="dashboard-inner"`, `data-swap-target="#dashboard-stats"`, `data-swap-target="#dashboard-today-cards"`} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("today fragment response included %s: %s", unwanted, body)
		}
	}
	for _, want := range []string{
		`data-swap-target="#stat-today-content"`,
		`data-swap-target="#stat-pending-content"`,
		`data-swap-target="#stat-week-content"`,
		`data-swap-target="#stat-streak-content"`,
		`data-swap-target="#today-pending-content"`,
		`data-swap-target="#today-done-content"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("today fragment response missing %s: %s", want, body)
		}
	}
}

func TestRenderToggleFragmentsForMissed(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	handler := &DashboardHandler{tmpl: tmpl}
	today := dashboardDate(2026, 5, 25)
	missedDate := dashboardDate(2026, 5, 24)
	inner := dashboardInnerView{
		Today: "Mon, 25 May 2026",
		Missed: []missedGroupView{{
			Date:      "2026-05-24",
			DateLabel: "Sun, 24 May 2026",
			Rows: []taskRowView{{
				TaskID:    2,
				Title:     "Review",
				Frequency: "daily",
				DueDate:   "2026-05-24",
				Status:    "missed",
				Section:   "missed",
				RowID:     "task-row-2-2026-05-24",
			}},
		}},
		Stats: dashboardStatsView{TodayTotal: 1, PendingCount: 1, WeeklyTotal: 2},
	}
	rec := httptest.NewRecorder()

	if err := handler.renderToggleFragments(rec, inner, 2, missedDate, today); err != nil {
		t.Fatalf("renderToggleFragments() error = %v", err)
	}

	body := rec.Body.String()
	for _, unwanted := range []string{`id="dashboard-inner"`, `data-swap-target="#dashboard-stats"`, `data-swap-target="#dashboard-today-cards"`, `data-swap-target="#today-pending-content"`, `data-swap-target="#today-done-content"`} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("missed fragment response included %s: %s", unwanted, body)
		}
	}
	for _, want := range []string{
		`data-swap-target="#stat-today-content"`,
		`data-swap-target="#stat-pending-content"`,
		`data-swap-target="#stat-week-content"`,
		`data-swap-target="#stat-streak-content"`,
		`data-swap-target="#task-row-2-2026-05-24"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missed fragment response missing %s: %s", want, body)
		}
	}
}
