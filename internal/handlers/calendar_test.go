package handlers

import (
	"database/sql"
	"testing"
	"time"

	"github.com/aldoerianda/tracker/internal/repository"
	"github.com/aldoerianda/tracker/internal/services"
)

func calendarDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestCalendarDayFromOccurrences(t *testing.T) {
	task := repository.Task{
		ID:          7,
		Title:       "Dhikr pagi",
		Description: sql.NullString{String: "dhikr", Valid: true},
		Frequency:   repository.TasksFrequencyDaily,
	}
	occs := []services.TaskOccurrence{
		{Task: task, DueDate: calendarDate(2026, 5, 25), Status: services.StatusCompleted},
		{Task: repository.Task{ID: 8, Title: "Tilawah", Frequency: repository.TasksFrequencyWeekly}, DueDate: calendarDate(2026, 5, 25), Status: services.StatusPending},
	}

	view := calendarDayFromOccurrences(calendarDate(2026, 5, 25), occs)
	if view.Date != "2026-05-25" || view.DateLabel != "Mon, 25 May 2026" {
		t.Fatalf("date labels = %q/%q, want 2026-05-25/Mon, 25 May 2026", view.Date, view.DateLabel)
	}
	if view.Completed != 1 || view.Total != 2 {
		t.Fatalf("totals = %d/%d, want 1/2", view.Completed, view.Total)
	}
	if len(view.Tasks) != 2 || view.Tasks[0].Description != "dhikr" || view.Tasks[1].Description != "" {
		t.Fatalf("tasks = %+v, want description carried only for valid description", view.Tasks)
	}
}
