package handlers

import (
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
