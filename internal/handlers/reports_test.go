package handlers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

func reportDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func reportTask(id int64, title string) repository.Task {
	return repository.Task{
		ID:        id,
		Title:     title,
		Frequency: repository.TasksFrequencyDaily,
		Active:    true,
		StartDate: reportDate(2026, 5, 25),
	}
}

func reportOcc(task repository.Task, due time.Time, status services.OccurrenceStatus) services.TaskOccurrence {
	return services.TaskOccurrence{Task: task, DueDate: due, Status: status}
}

func TestBuildReportBars_Weeks(t *testing.T) {
	task := reportTask(1, "Dhikr")
	sets := []reportOccurrenceSet{
		{
			User: repository.User{ID: 1, Name: "Amina"},
			Occurrences: []services.TaskOccurrence{
				reportOcc(task, reportDate(2026, 5, 1), services.StatusCompleted),
				reportOcc(task, reportDate(2026, 5, 3), services.StatusMissed),
				reportOcc(task, reportDate(2026, 5, 4), services.StatusCompleted),
			},
		},
	}

	bars, done, due := buildReportBars(reportDate(2026, 5, 1), reportDate(2026, 5, 31), reportDate(2026, 5, 31), time.Saturday, sets)
	if done != 2 || due != 3 {
		t.Fatalf("totals: got %d/%d, want 2/3", done, due)
	}
	if len(bars) != 6 {
		t.Fatalf("expected 6 Saturday-start weekly bars, got %d", len(bars))
	}
	if bars[0].Label != "Week 1" || bars[0].SubLabel != "01 May - 01 May" || bars[0].Percent != 100 || bars[0].Completed != 1 || bars[0].Total != 1 {
		t.Fatalf("first week bar = %+v, want Week 1 1/1 100%%", bars[0])
	}
	if bars[1].Label != "Week 2" || bars[1].SubLabel != "02 May - 08 May" || bars[1].Percent != 50 || bars[1].Completed != 1 || bars[1].Total != 2 {
		t.Fatalf("second week bar = %+v, want Week 2 1/2 50%%", bars[1])
	}
}

func TestBuildReportBars_PercentageExcludesExempt(t *testing.T) {
	task := reportTask(1, "Dhikr")
	sets := []reportOccurrenceSet{{
		User: repository.User{ID: 1, Name: "Amina"},
		Occurrences: []services.TaskOccurrence{
			reportOcc(task, reportDate(2026, 5, 2), services.StatusCompleted),
			reportOcc(task, reportDate(2026, 5, 3), services.StatusMissed),
			reportOcc(task, reportDate(2026, 5, 4), services.StatusPending),
			reportOcc(task, reportDate(2026, 5, 5), services.StatusExempt),
		},
	}}

	bars, done, due := buildReportBars(reportDate(2026, 5, 1), reportDate(2026, 5, 31), reportDate(2026, 5, 31), time.Saturday, sets)
	if done != 1 || due != 3 {
		t.Fatalf("totals: got %d/%d, want 1/3 with exempt excluded", done, due)
	}
	if bars[1].Completed != 1 || bars[1].Total != 3 || bars[1].Percent != 33 {
		t.Fatalf("week 2 bar = %+v, want 1/3 rounded to 33%%", bars[1])
	}
}

func TestBuildReportBars_PercentageExcludesFutureOccurrences(t *testing.T) {
	task := reportTask(1, "Dhikr")
	sets := []reportOccurrenceSet{{
		User: repository.User{ID: 1, Name: "Amina"},
		Occurrences: []services.TaskOccurrence{
			reportOcc(task, reportDate(2026, 5, 30), services.StatusCompleted),
			reportOcc(task, reportDate(2026, 5, 31), services.StatusPending),
		},
	}}

	bars, done, due := buildReportBars(reportDate(2026, 5, 1), reportDate(2026, 5, 31), reportDate(2026, 5, 30), time.Saturday, sets)
	if done != 1 || due != 1 {
		t.Fatalf("totals: got %d/%d, want only through today 1/1", done, due)
	}
	if bars[5].Completed != 1 || bars[5].Total != 1 || bars[5].Percent != 100 {
		t.Fatalf("future-excluding bar = %+v, want 1/1 100%%", bars[5])
	}
	if bars[5].SubLabel != "30 May - 30 May" {
		t.Fatalf("future-excluding range label = %q, want 30 May - 30 May", bars[5].SubLabel)
	}
}

func TestReportChartRangeLabel_EmptyForFutureOnlyBucket(t *testing.T) {
	got := reportChartRangeLabel(reportDate(2026, 6, 1), reportDate(2026, 6, 5), reportDate(2026, 5, 30))
	if got != "" {
		t.Fatalf("future-only chart range label = %q, want empty", got)
	}
}

func TestReportPDFFilenameIncludesSanitizedUserName(t *testing.T) {
	tests := []struct {
		name     string
		userName string
		want     string
	}{
		{
			name:     "simple name",
			userName: "Amina",
			want:     "amina-2026-W22.pdf",
		},
		{
			name:     "spaced name",
			userName: "Amina Sari",
			want:     "amina-sari-2026-W22.pdf",
		},
		{
			name:     "punctuation and whitespace",
			userName: "  Amina   Sari!! 2026  ",
			want:     "amina-sari-2026-2026-W22.pdf",
		},
		{
			name:     "empty name",
			userName: "",
			want:     "user-2026-W22.pdf",
		},
		{
			name:     "fully invalid name",
			userName: "!! --",
			want:     "user-2026-W22.pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reportPDFFilename(reportData{SelectedUserName: tt.userName, WeekValue: "2026-W22"})
			if got != tt.want {
				t.Fatalf("reportPDFFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildReportBars_WeekLabelsRestartByMonth(t *testing.T) {
	task := reportTask(1, "Dhikr")
	sets := []reportOccurrenceSet{{
		User:        repository.User{ID: 1, Name: "Amina"},
		Occurrences: []services.TaskOccurrence{reportOcc(task, reportDate(2026, 2, 1), services.StatusCompleted)},
	}}

	janBars, _, _ := buildReportBars(reportDate(2026, 1, 1), reportDate(2026, 1, 31), reportDate(2026, 12, 31), time.Saturday, sets)
	febBars, _, _ := buildReportBars(reportDate(2026, 2, 1), reportDate(2026, 2, 28), reportDate(2026, 12, 31), time.Saturday, sets)
	if janBars[0].Label != "Week 1" || febBars[0].Label != "Week 1" {
		t.Fatalf("month labels = January %q February %q, want both Week 1", janBars[0].Label, febBars[0].Label)
	}
	if febBars[0].SubLabel != "01 Feb - 06 Feb" {
		t.Fatalf("first partial week = %q, want 01 Feb - 06 Feb", febBars[0].SubLabel)
	}
}

func TestAggregateReportPDFBars_RecomputesPercentFromTotals(t *testing.T) {
	bars := aggregateReportPDFBars([]reportBarView{
		{Label: "Week 2", SubLabel: "02 May - 08 May", UserName: "Amina", Completed: 1, Total: 2, Percent: 50},
		{Label: "Week 2", SubLabel: "02 May - 08 May", UserName: "Bilal", Completed: 2, Total: 2, Percent: 100},
	})
	if len(bars) != 1 {
		t.Fatalf("aggregated bars = %d, want 1", len(bars))
	}
	if bars[0].Completed != 3 || bars[0].Total != 4 || bars[0].Percent != 75 {
		t.Fatalf("aggregated bar = %+v, want 3/4 75%%", bars[0])
	}
}

func TestReportChartJSONGroupsByUserOnly(t *testing.T) {
	js := reportChartJSON([]reportBarView{
		{Label: "Week 4", SubLabel: "18 May - 24 May", UserName: "Amina", Completed: 1, Total: 2},
		{Label: "Week 4", SubLabel: "18 May - 24 May", UserName: "Bilal", Completed: 2, Total: 2},
	})

	var data reportChartData
	if err := json.Unmarshal([]byte(js), &data); err != nil {
		t.Fatalf("chart json unmarshal: %v", err)
	}
	if len(data.Labels) != 1 || data.Labels[0] != "Week 4" {
		t.Fatalf("labels = %+v, want one weekly label", data.Labels)
	}
	if len(data.Users) != 2 || data.Users[0].Name != "Amina" || data.Users[1].Name != "Bilal" {
		t.Fatalf("series = %+v, want user series", data.Users)
	}
	if data.Users[0].Data[0] != 50 || data.Users[0].Percentage[0] != 50 || data.Users[0].Completed[0] != 1 || data.Users[0].Total[0] != 2 {
		t.Fatalf("Amina chart data = %+v, want 50%% with 1/2 metadata", data.Users[0])
	}
	if data.Users[1].Data[0] != 100 || data.Users[1].Percentage[0] != 100 || data.Users[1].Completed[0] != 2 || data.Users[1].Total[0] != 2 {
		t.Fatalf("Bilal chart data = %+v, want 100%% with 2/2 metadata", data.Users[1])
	}
}

func TestParseReportFallbacks(t *testing.T) {
	start, value := parseReportWeek("not-a-week", reportDate(2026, 5, 27), time.Saturday)
	if got := start.Format("2006-01-02"); got != "2026-05-23" {
		t.Fatalf("fallback week start = %s, want 2026-05-23", got)
	}
	if value != "2026-W22" {
		t.Fatalf("fallback week value = %s, want 2026-W22", value)
	}

	start, value = parseReportWeek("not-a-week", reportDate(2026, 5, 30), time.Saturday)
	if got := start.Format("2006-01-02"); got != "2026-05-30" {
		t.Fatalf("Saturday fallback week start = %s, want 2026-05-30", got)
	}
	if value != "2026-W23" {
		t.Fatalf("Saturday fallback week value = %s, want 2026-W23", value)
	}
}

func TestSelectedReportUsers(t *testing.T) {
	users := []repository.User{
		{ID: 1, Name: "Amina"},
		{ID: 2, Name: "Bilal"},
	}

	selected, options, hasSelected := selectedReportUsers(users, nil, 1)
	if !hasSelected || len(selected) != 1 || selected[0].ID != 1 {
		t.Fatalf("empty user filter selected %+v hasSelected=%v, want fallback Amina", selected, hasSelected)
	}
	if !options[0].Selected || options[1].Selected {
		t.Fatalf("options = %+v, want fallback Amina selected", options)
	}

	selected, options, hasSelected = selectedReportUsers(users, nil, 99)
	if hasSelected || len(selected) != 0 {
		t.Fatalf("unknown fallback selected %d hasSelected=%v, want no users", len(selected), hasSelected)
	}
	if options[0].Selected || options[1].Selected {
		t.Fatalf("options = %+v, want no single selected option", options)
	}

	selected, options, hasSelected = selectedReportUsers(users, []string{"2"}, 1)
	if !hasSelected || len(selected) != 1 || selected[0].ID != 2 {
		t.Fatalf("selected = %+v hasSelected=%v, want only Bilal", selected, hasSelected)
	}
	if options[0].Selected || !options[1].Selected {
		t.Fatalf("options = %+v, want Bilal selected", options)
	}
}

func TestSelectedReportUsersForCurrent_AdminHonorsFilter(t *testing.T) {
	current := repository.User{ID: 1, Name: "Amina", Role: repository.UsersRoleAdmin}
	users := []repository.User{
		current,
		{ID: 2, Name: "Bilal", Role: repository.UsersRoleUser},
	}

	selected, options, hasSelected := selectedReportUsersForCurrent(current, users, []string{"2"})
	if !hasSelected || len(selected) != 1 || selected[0].ID != 2 {
		t.Fatalf("selected = %+v hasSelected=%v, want admin-selected Bilal", selected, hasSelected)
	}
	if len(options) != 2 || options[0].Selected || !options[1].Selected {
		t.Fatalf("options = %+v, want Bilal selected", options)
	}
}

func TestSelectedReportUsersForCurrent_AdminFallsBackToSelf(t *testing.T) {
	current := repository.User{ID: 1, Name: "Amina", Role: repository.UsersRoleAdmin}
	users := []repository.User{
		current,
		{ID: 2, Name: "Bilal", Role: repository.UsersRoleUser},
	}

	selected, options, hasSelected := selectedReportUsersForCurrent(current, users, nil)
	if !hasSelected || len(selected) != 1 || selected[0].ID != 1 {
		t.Fatalf("selected = %+v hasSelected=%v, want admin fallback Amina", selected, hasSelected)
	}
	if len(options) != 2 || !options[0].Selected || options[1].Selected {
		t.Fatalf("options = %+v, want Amina selected", options)
	}
}

func TestSelectedReportUsersForCurrent_UserUsesOwnData(t *testing.T) {
	current := repository.User{ID: 1, Name: "Amina", Role: repository.UsersRoleUser}
	users := []repository.User{
		current,
		{ID: 2, Name: "Bilal", Role: repository.UsersRoleUser},
	}

	selected, options, hasSelected := selectedReportUsersForCurrent(current, users, nil)
	if !hasSelected || len(selected) != 1 || selected[0].ID != current.ID {
		t.Fatalf("selected = %+v hasSelected=%v, want current user only", selected, hasSelected)
	}
	if len(options) != 0 {
		t.Fatalf("options = %+v, want no user filter options", options)
	}
}

func TestSelectedReportUsersForCurrent_UserIgnoresUserID(t *testing.T) {
	current := repository.User{ID: 1, Name: "Amina", Role: repository.UsersRoleUser}
	users := []repository.User{
		current,
		{ID: 2, Name: "Bilal", Role: repository.UsersRoleUser},
	}

	selected, options, hasSelected := selectedReportUsersForCurrent(current, users, []string{"2"})
	if !hasSelected || len(selected) != 1 || selected[0].ID != current.ID {
		t.Fatalf("selected = %+v hasSelected=%v, want current user only", selected, hasSelected)
	}
	if len(options) != 0 {
		t.Fatalf("options = %+v, want no user filter options", options)
	}
}

func TestParseReportWeekValid(t *testing.T) {
	start, value := parseReportWeek("2026-W01", reportDate(2026, 5, 27), time.Saturday)
	if got := start.Format("2006-01-02"); got != "2025-12-27" {
		t.Fatalf("week start = %s, want 2025-12-27", got)
	}
	if value != "2026-W01" {
		t.Fatalf("week value = %s, want 2026-W01", value)
	}

	start, value = parseReportWeek("2026-05-26", reportDate(2026, 1, 1), time.Saturday)
	if got := start.Format("2006-01-02"); got != "2026-05-23" {
		t.Fatalf("date-selected week start = %s, want 2026-05-23", got)
	}
	if value != "2026-W22" {
		t.Fatalf("date-selected week value = %s, want 2026-W22", value)
	}

	start, value = parseReportWeek("2026-05-30", reportDate(2026, 1, 1), time.Saturday)
	if got := start.Format("2006-01-02"); got != "2026-05-30" {
		t.Fatalf("Saturday-selected week start = %s, want 2026-05-30", got)
	}
	if value != "2026-W23" {
		t.Fatalf("Saturday-selected week value = %s, want 2026-W23", value)
	}
}

func TestParseReportWeekUsesConfiguredStartDay(t *testing.T) {
	start, value := parseReportWeek("2026-05-30", reportDate(2026, 1, 1), time.Monday)
	if got := start.Format("2006-01-02"); got != "2026-05-25" {
		t.Fatalf("monday-start selected week = %s, want 2026-05-25", got)
	}
	if value != "2026-W22" {
		t.Fatalf("monday-start selected value = %s, want 2026-W22", value)
	}
}

func TestReportDateInputMinAlignsWithWeekStart(t *testing.T) {
	tests := []struct {
		name  string
		start time.Weekday
		want  string
	}{
		{name: "sunday", start: time.Sunday, want: "1970-01-04"},
		{name: "monday", start: time.Monday, want: "1970-01-05"},
		{name: "saturday", start: time.Saturday, want: "1970-01-10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reportDateInputMin(tt.start); got != tt.want {
				t.Fatalf("reportDateInputMin(%s) = %s, want %s", tt.start, got, tt.want)
			}
		})
	}
}

func TestReportTemplateRendersWeekStartDateConstraints(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	view := reportPageView{
		BaseView: NewBaseView(repository.User{
			ID:   1,
			Name: "Admin",
			Role: repository.UsersRoleAdmin,
		}, "csrf-token", "Report — Mutaba'ah Yaumiyah"),
		WeekDateValue: "2026-05-30",
		WeekDateMin:   reportDateInputMin(time.Saturday),
		WeekDateStep:  7,
		WeekLabel:     "Week 5",
		MonthLabel:    "May 2026",
	}
	rec := httptest.NewRecorder()

	if err := tmpl.Render(rec, "reports/index.html", view); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`type="date"`,
		`name="week_start"`,
		`value="2026-05-30"`,
		`min="1970-01-10"`,
		`step="7"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered report missing %s: %s", want, body)
		}
	}
}

func TestReportTemplateRendersUserFilterForAdmin(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	view := reportPageView{
		BaseView: NewBaseView(repository.User{
			ID:   1,
			Name: "Admin",
			Role: repository.UsersRoleAdmin,
		}, "csrf-token", "Report — Mutaba'ah Yaumiyah"),
		WeekDateValue:     "2026-05-30",
		WeekDateMin:       reportDateInputMin(time.Saturday),
		WeekDateStep:      7,
		WeekLabel:         "Week 5",
		MonthLabel:        "May 2026",
		CanFilterUsers:    true,
		HasSelectedUser:   true,
		ExportCacheBuster: 1777777777000,
		ChartJSON:         template.JS("{}"),
		UserOptions: []reportUserOption{
			{ID: 1, Name: "Admin", Selected: true},
			{ID: 2, Name: "User"},
		},
	}
	rec := httptest.NewRecorder()

	if err := tmpl.Render(rec, "reports/index.html", view); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`name="user_id"`,
		`<span class="label-text font-medium">User</span>`,
		`/reports/export.pdf?week_start=2026-05-30&user_id=1&ts=1777777777000`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered admin report missing %s: %s", want, body)
		}
	}
}

func TestReportTemplateHidesUserFilterForRegularUser(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	view := reportPageView{
		BaseView: NewBaseView(repository.User{
			ID:   1,
			Name: "Amina",
			Role: repository.UsersRoleUser,
		}, "csrf-token", "Report — Mutaba'ah Yaumiyah"),
		WeekDateValue:     "2026-05-30",
		WeekDateMin:       reportDateInputMin(time.Saturday),
		WeekDateStep:      7,
		WeekLabel:         "Week 5",
		MonthLabel:        "May 2026",
		HasSelectedUser:   true,
		ExportCacheBuster: 1777777777000,
		ChartJSON:         template.JS("{}"),
	}
	rec := httptest.NewRecorder()

	if err := tmpl.Render(rec, "reports/index.html", view); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`name="week_start"`,
		`Export PDF`,
		`href="/reports"`,
		`/reports/export.pdf?week_start=2026-05-30&ts=1777777777000"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered user report missing %s: %s", want, body)
		}
	}
	for _, unwanted := range []string{
		`name="user_id"`,
		`<span class="label-text font-medium">User</span>`,
		`user_id=`,
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("rendered user report contains %s: %s", unwanted, body)
		}
	}
}

func TestBuildReportMatrix_SelectedWeekStatusesAndBlankCells(t *testing.T) {
	dhikr := reportTask(1, "Dhikr")
	dhikr.Sequence = 2
	fajr := reportTask(2, "Fajr")
	fajr.Sequence = 1
	sets := []reportOccurrenceSet{{
		User: repository.User{ID: 1, Name: "Amina"},
		Occurrences: []services.TaskOccurrence{
			reportOcc(dhikr, reportDate(2026, 5, 24), services.StatusCompleted),
			reportOcc(dhikr, reportDate(2026, 5, 25), services.StatusCompleted),
			reportOcc(dhikr, reportDate(2026, 5, 26), services.StatusMissed),
			reportOcc(dhikr, reportDate(2026, 5, 27), services.StatusPending),
			reportOcc(dhikr, reportDate(2026, 5, 28), services.StatusExempt),
			reportOcc(dhikr, reportDate(2026, 5, 29), services.StatusPending),
			reportOcc(fajr, reportDate(2026, 5, 26), services.StatusCompleted),
			reportOcc(dhikr, reportDate(2026, 6, 1), services.StatusCompleted),
		},
	}}

	days, rows, done, due := buildReportMatrix(reportDate(2026, 5, 23), reportDate(2026, 5, 29), reportDate(2026, 5, 27), sets)
	if len(days) != 7 || days[0].ShortDate != "23 May" || days[0].Label != "Sat" || days[6].ShortDate != "29 May" || days[6].Label != "Fri" {
		t.Fatalf("days = %+v, want selected week date columns", days)
	}
	if done != 3 || due != 5 {
		t.Fatalf("selected week totals = %d/%d, want 3/5 with future and exempt excluded", done, due)
	}
	if len(rows) != 2 {
		t.Fatalf("matrix rows = %d, want 2", len(rows))
	}
	if rows[0].TaskName != "Fajr" || rows[1].TaskName != "Dhikr" {
		t.Fatalf("row order = %q, %q; want sequence order Fajr, Dhikr", rows[0].TaskName, rows[1].TaskName)
	}
	if rows[0].Cells[0].Scheduled || rows[0].Cells[1].Scheduled || rows[0].Cells[2].Scheduled || !rows[0].Cells[3].Scheduled || rows[0].Cells[3].Status != "completed" {
		t.Fatalf("Fajr cells = %+v, want blank Saturday-Monday and completed Tuesday", rows[0].Cells)
	}
	wantStatuses := map[int]string{
		1: "completed",
		2: "completed",
		3: "missed",
		4: "pending",
		5: "exempt",
		6: "pending",
	}
	for i, want := range wantStatuses {
		if !rows[1].Cells[i].Scheduled || rows[1].Cells[i].Status != want {
			t.Fatalf("Dhikr cell %d = %+v, want %s", i, rows[1].Cells[i], want)
		}
	}
	if rows[1].Cells[0].Scheduled {
		t.Fatalf("Dhikr edge cells = %+v, want blanks for non-scheduled dates", rows[1].Cells)
	}
}

func TestLoadTemplatesIncludesReports(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	if _, ok := tmpl.pages["reports/index.html"]; !ok {
		t.Fatal("reports/index.html was not loaded")
	}
}

func TestBuildReportPDFContainsPercentageChartAndStatusMatrix(t *testing.T) {
	pdf, err := buildReportPDF(reportData{
		WeekValue:        "2026-W22",
		WeekLabel:        "Week 5",
		WeekRangeLabel:   "25 May - 31 May",
		MonthLabel:       "May 2026",
		SelectedUserName: "Amina",
		TotalDone:        1,
		TotalDue:         2,
		TotalPct:         50,
		Bars: []reportBarView{
			{Label: "Week 5", SubLabel: "25 May - 31 May", UserName: "Amina", Completed: 1, Total: 2, Percent: 50},
		},
		WeekDays: []reportWeekDayView{
			{Label: "Mon", ShortDate: "25 May"},
			{Label: "Tue", ShortDate: "26 May"},
			{Label: "Wed", ShortDate: "27 May"},
			{Label: "Thu", ShortDate: "28 May"},
			{Label: "Fri", ShortDate: "29 May"},
			{Label: "Sat", ShortDate: "30 May"},
			{Label: "Sun", ShortDate: "31 May"},
		},
		TaskRows: []reportTaskRowView{
			{
				TaskName:    "Dhikr",
				Description: "Morning adhkar",
				Cells: []reportMatrixCellView{
					{Scheduled: true, Status: "completed", StatusLabel: "completed"},
					{Scheduled: true, Status: "missed", StatusLabel: "missed"},
					{Scheduled: true, Status: "exempt", StatusLabel: "exempt"},
					{Scheduled: true, Status: "pending", StatusLabel: "pending"},
					{},
					{},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildReportPDF() error = %v", err)
	}
	for _, want := range [][]byte{
		[]byte("%PDF-1."),
		[]byte("/BaseFont /utf8manrope"),
		[]byte("/BaseFont /utf8manropeB"),
		[]byte("/FontFile2"),
	} {
		if !bytes.Contains(pdf, want) {
			t.Fatalf("PDF does not contain %q", want)
		}
	}
	if bytes.Contains(pdf, []byte("/BaseFont /Helvetica")) {
		t.Fatal("PDF contains Helvetica instead of embedded Manrope")
	}
	if bytes.Contains(pdf, []byte("Monthly Comparison")) {
		t.Fatal("PDF contains removed Monthly Comparison table heading")
	}
	if bytes.Contains(pdf, []byte("completion %")) {
		t.Fatal("PDF contains removed chart legend")
	}
	if bytes.Contains(pdf, []byte("User: Amina")) {
		t.Fatal("PDF contains removed user label")
	}
	for _, removed := range [][]byte{
		[]byte("(D) Tj"),
		[]byte("(M) Tj"),
		[]byte("(E) Tj"),
		[]byte("(P) Tj"),
	} {
		if bytes.Contains(pdf, removed) {
			t.Fatalf("PDF contains removed status letter text %q", removed)
		}
	}
	for _, wantVector := range [][]byte{
		[]byte("0.122 0.549 0.220 RG"),
		[]byte("0.722 0.122 0.122 RG"),
		[]byte("0.702 0.180 0.451 RG"),
		[]byte("0.388 0.451 0.549 RG"),
		[]byte(" c "),
		[]byte(" l S"),
	} {
		if !bytes.Contains(pdf, wantVector) {
			t.Fatalf("PDF does not contain status icon vector command %q", wantVector)
		}
	}
}
