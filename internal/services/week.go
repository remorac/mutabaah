package services

import "time"

func WeekStartFor(day time.Time, startDay time.Weekday) time.Time {
	day = dateOnly(day)
	daysSinceStart := (int(day.Weekday()) - int(startDay) + 7) % 7
	return day.AddDate(0, 0, -daysSinceStart)
}

func WeekEndFor(day time.Time, startDay time.Weekday) time.Time {
	return WeekStartFor(day, startDay).AddDate(0, 0, 6)
}

func WeekDatesForHistory(now time.Time, startDay time.Weekday, weeks int, includeToday bool) []time.Time {
	if weeks < MinHistoryWeeks {
		weeks = MinHistoryWeeks
	}
	local := now.In(AppLocation)
	today := dateOnly(local)
	end := today
	if !includeToday {
		end = today.AddDate(0, 0, -1)
	}
	currentWeekStart := WeekStartFor(today, startDay)
	start := currentWeekStart.AddDate(0, 0, -7*(weeks-1))
	if end.Before(start) {
		return nil
	}
	out := make([]time.Time, 0, int(end.Sub(start).Hours()/24)+1)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		out = append(out, d)
	}
	return out
}

func WeekdayLabels(startDay time.Weekday) []string {
	labels := make([]string, 0, 7)
	base := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC) // Sunday
	for i := 0; i < 7; i++ {
		labels = append(labels, base.AddDate(0, 0, int(startDay)+i).Format("Mon"))
	}
	return labels
}
