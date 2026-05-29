package services

import (
	"testing"
	"time"
)

func TestWeekDatesForHistory(t *testing.T) {
	tests := []struct {
		name         string
		now          time.Time
		startDay     time.Weekday
		weeks        int
		includeToday bool
		want         []string
	}{
		{
			name:         "saturday returns only today",
			now:          time.Date(2026, 5, 23, 10, 0, 0, 0, AppLocation),
			startDay:     time.Saturday,
			weeks:        1,
			includeToday: true,
			want:         []string{"2026-05-23"},
		},
		{
			name:         "monday returns saturday through monday",
			now:          time.Date(2026, 5, 25, 10, 0, 0, 0, AppLocation),
			startDay:     time.Saturday,
			weeks:        1,
			includeToday: true,
			want:         []string{"2026-05-23", "2026-05-24", "2026-05-25"},
		},
		{
			name:         "friday returns saturday through friday",
			now:          time.Date(2026, 5, 29, 10, 0, 0, 0, AppLocation),
			startDay:     time.Saturday,
			weeks:        1,
			includeToday: true,
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
		{
			name:         "monday start excludes today for dashboard missed",
			now:          time.Date(2026, 5, 25, 10, 0, 0, 0, AppLocation),
			startDay:     time.Monday,
			weeks:        1,
			includeToday: false,
			want:         nil,
		},
		{
			name:         "two weeks includes previous full configured week",
			now:          time.Date(2026, 5, 25, 10, 0, 0, 0, AppLocation),
			startDay:     time.Saturday,
			weeks:        2,
			includeToday: false,
			want: []string{
				"2026-05-16",
				"2026-05-17",
				"2026-05-18",
				"2026-05-19",
				"2026-05-20",
				"2026-05-21",
				"2026-05-22",
				"2026-05-23",
				"2026-05-24",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WeekDatesForHistory(tt.now, tt.startDay, tt.weeks, tt.includeToday)
			if len(got) != len(tt.want) {
				t.Fatalf("WeekDatesForHistory() returned %d dates, want %d: %v", len(got), len(tt.want), got)
			}
			for i, d := range got {
				if gotDate := d.Format("2006-01-02"); gotDate != tt.want[i] {
					t.Fatalf("WeekDatesForHistory()[%d] = %s, want %s", i, gotDate, tt.want[i])
				}
			}
		})
	}
}

func TestParseAppSettingsInput(t *testing.T) {
	if _, err := ParseAppSettingsInput(AppSettingsInput{WeekStartDay: "6", HistoryWeeks: "4"}); err != nil {
		t.Fatalf("ParseAppSettingsInput() valid input error = %v", err)
	}
	for _, in := range []AppSettingsInput{
		{WeekStartDay: "-1", HistoryWeeks: "1"},
		{WeekStartDay: "7", HistoryWeeks: "1"},
		{WeekStartDay: "6", HistoryWeeks: "0"},
		{WeekStartDay: "6", HistoryWeeks: "5"},
	} {
		if _, err := ParseAppSettingsInput(in); err == nil {
			t.Fatalf("ParseAppSettingsInput(%+v) error = nil, want validation error", in)
		}
	}
}
