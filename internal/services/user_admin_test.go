package services

import (
	"testing"
	"time"
)

func TestWeekDatesSinceSaturday(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want []string
	}{
		{
			name: "saturday returns only today",
			now:  time.Date(2026, 5, 23, 10, 0, 0, 0, AppLocation),
			want: []string{"2026-05-23"},
		},
		{
			name: "monday returns saturday through monday",
			now:  time.Date(2026, 5, 25, 10, 0, 0, 0, AppLocation),
			want: []string{"2026-05-23", "2026-05-24", "2026-05-25"},
		},
		{
			name: "friday returns saturday through friday",
			now:  time.Date(2026, 5, 29, 10, 0, 0, 0, AppLocation),
			want: []string{
				"2026-05-23",
				"2026-05-24",
				"2026-05-25",
				"2026-05-26",
				"2026-05-27",
				"2026-05-28",
				"2026-05-29",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WeekDatesSinceSaturday(tt.now)
			if len(got) != len(tt.want) {
				t.Fatalf("WeekDatesSinceSaturday() returned %d dates, want %d: %v", len(got), len(tt.want), got)
			}
			for i, d := range got {
				if gotDate := d.Format("2006-01-02"); gotDate != tt.want[i] {
					t.Fatalf("WeekDatesSinceSaturday()[%d] = %s, want %s", i, gotDate, tt.want[i])
				}
			}
		})
	}
}
