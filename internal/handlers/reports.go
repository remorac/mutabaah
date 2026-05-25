package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

type ReportHandler struct {
	auth   *services.AuthService
	tasks  *services.TaskService
	users  *services.UserAdminService
	tmpl   *Templates
	errs   *ErrorPages
	logger *slog.Logger
	now    func() time.Time
}

func NewReportHandler(auth *services.AuthService, tasks *services.TaskService, users *services.UserAdminService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *ReportHandler {
	return &ReportHandler{auth: auth, tasks: tasks, users: users, tmpl: tmpl, errs: errs, logger: logger, now: time.Now}
}

type reportUserOption struct {
	ID       int64
	Name     string
	Selected bool
}

type reportBarView struct {
	Label     string
	SubLabel  string
	UserName  string
	TaskName  string
	Completed int
	Total     int
	Percent   int
}

type reportPageView struct {
	BaseView
	MonthValue      string
	MonthLabel      string
	GroupByTasks    bool
	HasSelectedUser bool
	UserOptions     []reportUserOption
	Bars            []reportBarView
	ChartJSON       template.JS
	TotalDone       int
	TotalDue        int
	TotalPct        int
}

type reportChartData struct {
	Labels    []string                `json:"labels"`
	SubLabels []string                `json:"subLabels"`
	Users     []reportChartUserSeries `json:"users"`
}

type reportChartUserSeries struct {
	Name       string `json:"name"`
	Completed  []int  `json:"completed"`
	Missed     []int  `json:"missed"`
	Total      []int  `json:"total"`
	Percentage []int  `json:"percentage"`
}

type reportOccurrenceSet struct {
	User        repository.User
	Occurrences []services.TaskOccurrence
}

type reportData struct {
	MonthStart       time.Time
	MonthValue       string
	MonthLabel       string
	GroupByTasks     bool
	HasSelectedUser  bool
	SelectedUserName string
	UserOptions      []reportUserOption
	Bars             []reportBarView
	TotalDone        int
	TotalDue         int
	TotalPct         int
}

// Show renders the admin reports page.
func (h *ReportHandler) Show(w http.ResponseWriter, r *http.Request) {
	current, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	report, err := h.buildReportData(r)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}
	view := reportPageView{
		BaseView: BaseView{
			Title:     "Report — Mutaba'ah Tracker",
			UserName:  current.Name,
			UserRole:  string(current.Role),
			CSRFToken: token,
		},
		MonthValue:      report.MonthValue,
		MonthLabel:      report.MonthLabel,
		GroupByTasks:    report.GroupByTasks,
		HasSelectedUser: report.HasSelectedUser,
		UserOptions:     report.UserOptions,
		Bars:            report.Bars,
		ChartJSON:       reportChartJSON(report.Bars, report.GroupByTasks),
		TotalDone:       report.TotalDone,
		TotalDue:        report.TotalDue,
		TotalPct:        report.TotalPct,
	}
	if err := h.tmpl.Render(w, "reports/index.html", view); err != nil {
		h.errs.ServerError(w, r, err)
	}
}

// ExportPDF writes the currently filtered report as a PDF containing a chart
// summary and tabulation table.
func (h *ReportHandler) ExportPDF(w http.ResponseWriter, r *http.Request) {
	report, err := h.buildReportData(r)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	pdf, err := buildReportPDF(report)
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}

	filename := fmt.Sprintf("mutabaah-report-%s.pdf", report.MonthValue)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	_, _ = w.Write(pdf)
}

func (h *ReportHandler) buildReportData(r *http.Request) (reportData, error) {
	today := todayFor(h.now)
	monthStart, monthValue := parseReportMonth(r.URL.Query().Get("month"), today)
	monthEnd := monthStart.AddDate(0, 1, -1)
	groupByTasks := parseReportGroupByTasks(r)

	allUsers, _, err := h.users.List(r.Context(), 0, 0)
	if err != nil {
		return reportData{}, err
	}
	current, _ := apmw.UserFromContext(r.Context())
	selectedUsers, options, hasSelectedUser := selectedReportUsers(allUsers, r.URL.Query()["user_id"], current.ID)
	var selectedUserName string
	if len(selectedUsers) > 0 {
		selectedUserName = selectedUsers[0].Name
	}

	sets := make([]reportOccurrenceSet, 0, len(selectedUsers))
	for _, u := range selectedUsers {
		occs, err := h.tasks.OccurrencesBetweenAsOf(r.Context(), u.ID, monthStart, monthEnd, today)
		if err != nil {
			return reportData{}, err
		}
		sets = append(sets, reportOccurrenceSet{User: u, Occurrences: occs})
	}

	bars, done, due := buildReportBars(groupByTasks, monthStart, monthEnd, sets)
	return reportData{
		MonthStart:       monthStart,
		MonthValue:       monthValue,
		MonthLabel:       monthStart.Format("January 2006"),
		GroupByTasks:     groupByTasks,
		HasSelectedUser:  hasSelectedUser,
		SelectedUserName: selectedUserName,
		UserOptions:      options,
		Bars:             bars,
		TotalDone:        done,
		TotalDue:         due,
		TotalPct:         percent(done, due),
	}, nil
}

func reportChartJSON(bars []reportBarView, groupByTasks bool) template.JS {
	data := reportChartData{
		Labels:    make([]string, 0, len(bars)),
		SubLabels: make([]string, 0, len(bars)),
	}

	labelIndexes := map[string]int{}
	userIndexes := map[string]int{}
	for _, bar := range bars {
		label := bar.Label
		key := label + "\x00" + bar.SubLabel
		labelIndex, ok := labelIndexes[key]
		if !ok {
			labelIndex = len(data.Labels)
			labelIndexes[key] = labelIndex
			data.Labels = append(data.Labels, label)
			data.SubLabels = append(data.SubLabels, bar.SubLabel)
			for i := range data.Users {
				data.Users[i].Completed = append(data.Users[i].Completed, 0)
				data.Users[i].Missed = append(data.Users[i].Missed, 0)
				data.Users[i].Total = append(data.Users[i].Total, 0)
				data.Users[i].Percentage = append(data.Users[i].Percentage, 0)
			}
		}

		seriesName := bar.UserName
		if groupByTasks && bar.TaskName != "" {
			seriesName = bar.TaskName
		}
		userIndex, ok := userIndexes[seriesName]
		if !ok {
			userIndex = len(data.Users)
			userIndexes[seriesName] = userIndex
			data.Users = append(data.Users, reportChartUserSeries{
				Name:       seriesName,
				Completed:  make([]int, len(data.Labels)),
				Missed:     make([]int, len(data.Labels)),
				Total:      make([]int, len(data.Labels)),
				Percentage: make([]int, len(data.Labels)),
			})
		}

		data.Users[userIndex].Completed[labelIndex] += bar.Completed
		data.Users[userIndex].Missed[labelIndex] += bar.Total - bar.Completed
		data.Users[userIndex].Total[labelIndex] += bar.Total
		data.Users[userIndex].Percentage[labelIndex] = percent(data.Users[userIndex].Completed[labelIndex], data.Users[userIndex].Total[labelIndex])
	}
	b, err := json.Marshal(data)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(b)
}

func parseReportGroupByTasks(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("group_by_tasks") != "" || q.Get("subgroup") == "tasks"
}

func parseReportMonth(s string, fallback time.Time) (time.Time, string) {
	if t, err := time.ParseInLocation("2006-01", s, time.UTC); err == nil {
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.Format("2006-01")
	}
	fallback = fallback.In(services.AppLocation)
	start := time.Date(fallback.Year(), fallback.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.Format("2006-01")
}

func selectedReportUsers(all []repository.User, raw []string, fallbackID int64) ([]repository.User, []reportUserOption, bool) {
	var selectedID int64
	for _, s := range raw {
		id, err := strconv.ParseInt(s, 10, 64)
		if err == nil && id > 0 {
			selectedID = id
			break
		}
	}
	if selectedID == 0 {
		selectedID = fallbackID
	}

	selected := make([]repository.User, 0, len(all))
	options := make([]reportUserOption, 0, len(all))
	hasSelected := false
	for _, u := range all {
		isSelected := selectedID == u.ID
		options = append(options, reportUserOption{ID: u.ID, Name: u.Name, Selected: isSelected})
		if isSelected {
			selected = append(selected, u)
			hasSelected = true
		}
	}
	return selected, options, hasSelected
}

func buildReportBars(groupByTasks bool, periodStart, periodEnd time.Time, sets []reportOccurrenceSet) ([]reportBarView, int, int) {
	if groupByTasks {
		return buildWeeklyTaskReportBars(periodStart, periodEnd, sets)
	}
	return buildWeeklyReportBars(periodStart, periodEnd, sets)
}

func buildWeeklyReportBars(periodStart, periodEnd time.Time, sets []reportOccurrenceSet) ([]reportBarView, int, int) {
	type weekBar struct {
		start time.Time
		end   time.Time
		bar   reportBarView
	}

	var weeks []weekBar
	for start := periodStart; !start.After(periodEnd); {
		end := start.AddDate(0, 0, 6-int(start.Weekday()+6)%7)
		if end.After(periodEnd) {
			end = periodEnd
		}
		_, isoWeek := start.ISOWeek()
		weeks = append(weeks, weekBar{
			start: start,
			end:   end,
			bar: reportBarView{
				Label:    fmt.Sprintf("Week %02d", isoWeek),
				SubLabel: fmt.Sprintf("%s - %s", start.Format("02 Jan"), end.Format("02 Jan")),
			},
		})
		start = end.AddDate(0, 0, 1)
	}

	bars := make([]reportBarView, 0, len(weeks)*len(sets))
	for _, set := range sets {
		userWeeks := make([]weekBar, len(weeks))
		copy(userWeeks, weeks)
		for i := range userWeeks {
			userWeeks[i].bar.UserName = set.User.Name
		}

		for _, occ := range set.Occurrences {
			due := dateOnly(occ.DueDate)
			for i := range userWeeks {
				if !due.Before(userWeeks[i].start) && !due.After(userWeeks[i].end) {
					addOccurrence(&userWeeks[i].bar, occ)
					break
				}
			}
		}
		for _, week := range userWeeks {
			bars = append(bars, week.bar)
		}
	}
	return finalizeReportBars(bars)
}

func buildWeeklyTaskReportBars(periodStart, periodEnd time.Time, sets []reportOccurrenceSet) ([]reportBarView, int, int) {
	type weekRange struct {
		start    time.Time
		end      time.Time
		label    string
		subLabel string
	}

	var weeks []weekRange
	for start := periodStart; !start.After(periodEnd); {
		end := start.AddDate(0, 0, 6-int(start.Weekday()+6)%7)
		if end.After(periodEnd) {
			end = periodEnd
		}
		_, isoWeek := start.ISOWeek()
		weeks = append(weeks, weekRange{
			start:    start,
			end:      end,
			label:    fmt.Sprintf("Week %02d", isoWeek),
			subLabel: fmt.Sprintf("%s - %s", start.Format("02 Jan"), end.Format("02 Jan")),
		})
		start = end.AddDate(0, 0, 1)
	}

	bars := make([]reportBarView, 0)
	for _, set := range sets {
		byTaskWeek := map[string]*reportBarView{}
		for _, occ := range set.Occurrences {
			due := dateOnly(occ.DueDate)
			for weekIndex, week := range weeks {
				if due.Before(week.start) || due.After(week.end) {
					continue
				}
				key := fmt.Sprintf("%d:%d", weekIndex, occ.Task.ID)
				bar, ok := byTaskWeek[key]
				if !ok {
					bar = &reportBarView{
						Label:    week.label,
						SubLabel: week.subLabel,
						UserName: set.User.Name,
						TaskName: occ.Task.Title,
					}
					byTaskWeek[key] = bar
				}
				addOccurrence(bar, occ)
				break
			}
		}

		userBars := make([]reportBarView, 0, len(byTaskWeek))
		for _, bar := range byTaskWeek {
			userBars = append(userBars, *bar)
		}
		sort.SliceStable(userBars, func(i, j int) bool {
			if userBars[i].Label == userBars[j].Label {
				return userBars[i].TaskName < userBars[j].TaskName
			}
			return userBars[i].Label < userBars[j].Label
		})
		bars = append(bars, userBars...)
	}
	return finalizeReportBars(bars)
}

func addOccurrence(bar *reportBarView, occ services.TaskOccurrence) {
	if occ.Status == services.StatusExempt {
		return
	}
	bar.Total++
	if occ.Status == services.StatusCompleted {
		bar.Completed++
	}
}

func finalizeReportBars(bars []reportBarView) ([]reportBarView, int, int) {
	var done, due int
	for i := range bars {
		bars[i].Percent = percent(bars[i].Completed, bars[i].Total)
		done += bars[i].Completed
		due += bars[i].Total
	}
	return bars, done, due
}

func percent(done, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(done)/float64(total)*100 + 0.5)
}
