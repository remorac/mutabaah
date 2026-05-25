package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aldoerianda/tracker/internal/repository"
	"github.com/aldoerianda/tracker/internal/services"
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

	bars, done, due := buildReportBars(false, reportDate(2026, 5, 1), reportDate(2026, 5, 31), sets)
	if done != 2 || due != 3 {
		t.Fatalf("totals: got %d/%d, want 2/3", done, due)
	}
	if len(bars) != 5 {
		t.Fatalf("expected 5 weekly bars, got %d", len(bars))
	}
	if bars[0].Label != "Week 18" || bars[0].Percent != 50 || bars[0].Completed != 1 || bars[0].Total != 2 {
		t.Fatalf("first week bar = %+v, want Week 18 1/2 50%%", bars[0])
	}
	if bars[1].Label != "Week 19" || bars[1].Percent != 100 || bars[1].Completed != 1 || bars[1].Total != 1 {
		t.Fatalf("second week bar = %+v, want Week 19 1/1 100%%", bars[1])
	}
}

func TestBuildReportBars_WeeksWithTaskSubgroup(t *testing.T) {
	dhikr := reportTask(1, "Dhikr")
	quran := reportTask(2, "Quran")
	sets := []reportOccurrenceSet{
		{
			User: repository.User{ID: 1, Name: "Amina"},
			Occurrences: []services.TaskOccurrence{
				reportOcc(quran, reportDate(2026, 5, 25), services.StatusCompleted),
				reportOcc(dhikr, reportDate(2026, 5, 25), services.StatusMissed),
			},
		},
		{
			User: repository.User{ID: 2, Name: "Bilal"},
			Occurrences: []services.TaskOccurrence{
				reportOcc(quran, reportDate(2026, 5, 25), services.StatusMissed),
				reportOcc(dhikr, reportDate(2026, 5, 25), services.StatusCompleted),
			},
		},
	}

	bars, done, due := buildReportBars(true, reportDate(2026, 5, 1), reportDate(2026, 5, 31), sets)
	if done != 2 || due != 4 {
		t.Fatalf("totals: got %d/%d, want 2/4", done, due)
	}
	if len(bars) != 4 {
		t.Fatalf("expected 4 bars, got %d", len(bars))
	}
	if bars[0].Label != "Week 22" || bars[0].TaskName != "Dhikr" || bars[0].UserName != "Amina" || bars[0].Percent != 0 {
		t.Fatalf("first task subgroup bar = %+v, want Week 22/Dhikr/Amina 0%%", bars[0])
	}
	if bars[1].Label != "Week 22" || bars[1].TaskName != "Quran" || bars[1].UserName != "Amina" || bars[1].Percent != 100 {
		t.Fatalf("second task subgroup bar = %+v, want Week 22/Quran/Amina 100%%", bars[1])
	}
	if bars[2].Label != "Week 22" || bars[2].TaskName != "Dhikr" || bars[2].UserName != "Bilal" || bars[2].Percent != 100 {
		t.Fatalf("third task subgroup bar = %+v, want Week 22/Dhikr/Bilal 100%%", bars[2])
	}
	if bars[3].Label != "Week 22" || bars[3].TaskName != "Quran" || bars[3].UserName != "Bilal" || bars[3].Percent != 0 {
		t.Fatalf("fourth task subgroup bar = %+v, want Week 22/Quran/Bilal 0%%", bars[3])
	}
}

func TestReportChartJSONGroupsTasksInsideWeeks(t *testing.T) {
	js := reportChartJSON([]reportBarView{
		{Label: "Week 22", SubLabel: "25 May - 31 May", TaskName: "Dhikr", UserName: "Amina", Completed: 1, Total: 2},
		{Label: "Week 22", SubLabel: "25 May - 31 May", TaskName: "Quran", UserName: "Amina", Completed: 2, Total: 2},
	}, true)

	var data reportChartData
	if err := json.Unmarshal([]byte(js), &data); err != nil {
		t.Fatalf("chart json unmarshal: %v", err)
	}
	if len(data.Labels) != 1 || data.Labels[0] != "Week 22" {
		t.Fatalf("labels = %+v, want one weekly label", data.Labels)
	}
	if len(data.Users) != 2 || data.Users[0].Name != "Dhikr" || data.Users[1].Name != "Quran" {
		t.Fatalf("series = %+v, want task series inside the week", data.Users)
	}
}

func TestParseReportFallbacks(t *testing.T) {
	start, value := parseReportMonth("not-a-month", reportDate(2026, 5, 27))
	if got := start.Format("2006-01-02"); got != "2026-05-01" {
		t.Fatalf("fallback month start = %s, want 2026-05-01", got)
	}
	if value != "2026-05" {
		t.Fatalf("fallback month value = %s, want 2026-05", value)
	}
	req := httptest.NewRequest("GET", "/reports?group_by_tasks=1", nil)
	if !parseReportGroupByTasks(req) {
		t.Fatal("group_by_tasks checkbox was not parsed")
	}
	req = httptest.NewRequest("GET", "/reports?subgroup=tasks", nil)
	if !parseReportGroupByTasks(req) {
		t.Fatal("legacy subgroup tasks parameter was not parsed")
	}
	req = httptest.NewRequest("GET", "/reports", nil)
	if parseReportGroupByTasks(req) {
		t.Fatal("empty group_by_tasks parsed as selected")
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

func TestParseReportMonthValid(t *testing.T) {
	start, value := parseReportMonth("2026-01", reportDate(2026, 5, 27))
	if got := start.Format("2006-01-02"); got != "2026-01-01" {
		t.Fatalf("month start = %s, want 2026-01-01", got)
	}
	if value != "2026-01" {
		t.Fatalf("month value = %s, want 2026-01", value)
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

func TestBuildReportPDFContainsChartAndTabulation(t *testing.T) {
	pdf, err := buildReportPDF(reportData{
		MonthValue:       "2026-05",
		MonthLabel:       "May 2026",
		GroupByTasks:     true,
		SelectedUserName: "Amina",
		TotalDone:        3,
		TotalDue:         4,
		TotalPct:         75,
		Bars: []reportBarView{
			{Label: "Week 21", SubLabel: "25 May - 31 May", TaskName: "Dhikr", UserName: "Amina", Completed: 3, Total: 4, Percent: 75},
		},
	})
	if err != nil {
		t.Fatalf("buildReportPDF() error = %v", err)
	}
	for _, want := range [][]byte{
		[]byte("%PDF-1.4"),
		[]byte("Chart"),
		[]byte("Tabulation"),
		[]byte("Week 21"),
		[]byte("Dhikr"),
		[]byte("Amina"),
		[]byte("User: Amina"),
	} {
		if !bytes.Contains(pdf, want) {
			t.Fatalf("PDF does not contain %q", want)
		}
	}
}
