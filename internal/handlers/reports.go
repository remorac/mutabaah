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
	auth     *services.AuthService
	tasks    *services.TaskService
	users    *services.UserAdminService
	settings *services.AppSettingsService
	tmpl     *Templates
	errs     *ErrorPages
	logger   *slog.Logger
	now      func() time.Time
}

func NewReportHandler(auth *services.AuthService, tasks *services.TaskService, users *services.UserAdminService, settings *services.AppSettingsService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *ReportHandler {
	return &ReportHandler{auth: auth, tasks: tasks, users: users, settings: settings, tmpl: tmpl, errs: errs, logger: logger, now: time.Now}
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
	Completed int
	Total     int
	Percent   int
}

type reportWeekDayView struct {
	Date      string
	Label     string
	ShortDate string
	sortDate  string
}

type reportMatrixCellView struct {
	Scheduled   bool
	Status      string
	StatusLabel string
	CompletedAt string
}

type reportTaskRowView struct {
	TaskID       int64
	TaskName     string
	Description  string
	Frequency    string
	UserName     string
	Cells        []reportMatrixCellView
	sortSequence int32
	sortTitle    string
}

type reportPageView struct {
	BaseView
	WeekValue       string
	WeekDateValue   string
	WeekDateMin     string
	WeekDateStep    int
	WeekLabel       string
	WeekRangeLabel  string
	MonthLabel      string
	CanFilterUsers  bool
	HasSelectedUser bool
	UserOptions     []reportUserOption
	Bars            []reportBarView
	WeekDays        []reportWeekDayView
	TaskRows        []reportTaskRowView
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
	Data       []int  `json:"data"`
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
	WeekStart        time.Time
	WeekEnd          time.Time
	WeekValue        string
	WeekDateValue    string
	WeekDateMin      string
	WeekDateStep     int
	WeekLabel        string
	WeekRangeLabel   string
	MonthLabel       string
	CanFilterUsers   bool
	HasSelectedUser  bool
	SelectedUserName string
	UserOptions      []reportUserOption
	Bars             []reportBarView
	WeekDays         []reportWeekDayView
	TaskRows         []reportTaskRowView
	TotalDone        int
	TotalDue         int
	TotalPct         int
}

// Show renders the reports page.
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
		BaseView:        NewBaseView(current, token, "Report — Mutaba'ah Yaumiyah"),
		WeekValue:       report.WeekValue,
		WeekDateValue:   report.WeekDateValue,
		WeekDateMin:     report.WeekDateMin,
		WeekDateStep:    report.WeekDateStep,
		WeekLabel:       report.WeekLabel,
		WeekRangeLabel:  report.WeekRangeLabel,
		MonthLabel:      report.MonthLabel,
		CanFilterUsers:  report.CanFilterUsers,
		HasSelectedUser: report.HasSelectedUser,
		UserOptions:     report.UserOptions,
		Bars:            report.Bars,
		WeekDays:        report.WeekDays,
		TaskRows:        report.TaskRows,
		ChartJSON:       reportChartJSON(report.Bars),
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

	filename := fmt.Sprintf("mutabaah-report-%s.pdf", report.WeekValue)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	_, _ = w.Write(pdf)
}

func (h *ReportHandler) buildReportData(r *http.Request) (reportData, error) {
	today := todayFor(h.now)
	settings, err := h.settings.Get(r.Context())
	if err != nil {
		return reportData{}, err
	}
	weekStart, weekValue := parseReportWeek(r.URL.Query().Get("week_start"), today, settings.WeekStartDay)
	if r.URL.Query().Get("week_start") == "" {
		weekStart, weekValue = parseReportWeek(r.URL.Query().Get("week"), today, settings.WeekStartDay)
	}
	weekEnd := weekStart.AddDate(0, 0, 6)
	monthStart := time.Date(weekStart.Year(), weekStart.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, -1)
	queryEnd := monthEnd
	if weekEnd.After(queryEnd) {
		queryEnd = weekEnd
	}

	current, _ := apmw.UserFromContext(r.Context())

	var selectedUsers []repository.User
	var options []reportUserOption
	var hasSelectedUser bool
	canFilterUsers := current.Role == repository.UsersRoleAdmin
	if canFilterUsers {
		allUsers, _, err := h.users.List(r.Context(), 0, 0)
		if err != nil {
			return reportData{}, err
		}
		selectedUsers, options, hasSelectedUser = selectedReportUsersForCurrent(current, allUsers, r.URL.Query()["user_id"])
	} else {
		selectedUsers, options, hasSelectedUser = selectedReportUsersForCurrent(current, nil, r.URL.Query()["user_id"])
	}

	var selectedUserName string
	if len(selectedUsers) > 0 {
		selectedUserName = selectedUsers[0].Name
	}

	sets := make([]reportOccurrenceSet, 0, len(selectedUsers))
	for _, u := range selectedUsers {
		occs, err := h.tasks.OccurrencesBetweenAsOf(r.Context(), u.ID, monthStart, queryEnd, today)
		if err != nil {
			return reportData{}, err
		}
		sets = append(sets, reportOccurrenceSet{User: u, Occurrences: occs})
	}

	bars, _, _ := buildReportBars(monthStart, monthEnd, today, settings.WeekStartDay, sets)
	weekDays, taskRows, done, due := buildReportMatrix(weekStart, weekEnd, today, sets)
	weekLabel, weekRangeLabel := selectedReportWeekLabels(monthStart, monthEnd, weekStart, weekEnd, settings.WeekStartDay)
	return reportData{
		WeekStart:        weekStart,
		WeekEnd:          weekEnd,
		WeekValue:        weekValue,
		WeekDateValue:    weekStart.Format("2006-01-02"),
		WeekDateMin:      reportDateInputMin(settings.WeekStartDay),
		WeekDateStep:     7,
		WeekLabel:        weekLabel,
		WeekRangeLabel:   weekRangeLabel,
		MonthLabel:       monthStart.Format("January 2006"),
		CanFilterUsers:   canFilterUsers,
		HasSelectedUser:  hasSelectedUser,
		SelectedUserName: selectedUserName,
		UserOptions:      options,
		Bars:             bars,
		WeekDays:         weekDays,
		TaskRows:         taskRows,
		TotalDone:        done,
		TotalDue:         due,
		TotalPct:         percent(done, due),
	}, nil
}

func reportChartJSON(bars []reportBarView) template.JS {
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
				data.Users[i].Data = append(data.Users[i].Data, 0)
			}
		}

		seriesName := bar.UserName
		userIndex, ok := userIndexes[seriesName]
		if !ok {
			userIndex = len(data.Users)
			userIndexes[seriesName] = userIndex
			data.Users = append(data.Users, reportChartUserSeries{
				Name:       seriesName,
				Data:       make([]int, len(data.Labels)),
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
		data.Users[userIndex].Data[labelIndex] = data.Users[userIndex].Percentage[labelIndex]
	}
	b, err := json.Marshal(data)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(b)
}

func reportDateInputMin(weekStartDay time.Weekday) string {
	baseSunday := time.Date(1970, 1, 4, 0, 0, 0, 0, time.UTC)
	return baseSunday.AddDate(0, 0, int(weekStartDay)).Format("2006-01-02")
}

func parseReportWeek(s string, fallback time.Time, weekStartDay time.Weekday) (time.Time, string) {
	if selected, err := time.ParseInLocation("2006-01-02", s, time.UTC); err == nil {
		start := services.WeekStartFor(selected, weekStartDay)
		return start, reportWeekValue(start)
	}
	if len(s) == len("2006-W02") && s[4] == '-' && s[5] == 'W' {
		year, yearErr := strconv.Atoi(s[:4])
		week, weekErr := strconv.Atoi(s[6:])
		if yearErr == nil && weekErr == nil {
			if isoStart, ok := isoWeekStart(year, week); ok {
				start := services.WeekStartFor(isoStart, weekStartDay)
				return start, reportWeekValue(start)
			}
		}
	}
	fallback = fallback.In(services.AppLocation)
	start := services.WeekStartFor(dateOnly(fallback), weekStartDay)
	return start, reportWeekValue(start)
}

func isoWeekStart(year, week int) (time.Time, bool) {
	if week < 1 || week > 53 {
		return time.Time{}, false
	}
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	start := isoMondayWeekStartFor(jan4).AddDate(0, 0, (week-1)*7)
	gotYear, gotWeek := start.ISOWeek()
	if gotYear != year || gotWeek != week {
		return time.Time{}, false
	}
	return start, true
}

func reportWeekValue(start time.Time) string {
	year, week := start.AddDate(0, 0, 2).ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func isoMondayWeekStartFor(day time.Time) time.Time {
	day = dateOnly(day)
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
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

func selectedReportUsersForCurrent(current repository.User, all []repository.User, raw []string) ([]repository.User, []reportUserOption, bool) {
	if current.Role != repository.UsersRoleAdmin {
		return []repository.User{current}, nil, true
	}
	return selectedReportUsers(all, raw, current.ID)
}

func buildReportBars(periodStart, periodEnd, today time.Time, weekStartDay time.Weekday, sets []reportOccurrenceSet) ([]reportBarView, int, int) {
	return buildWeeklyReportBars(periodStart, periodEnd, today, weekStartDay, sets)
}

func buildWeeklyReportBars(periodStart, periodEnd, today time.Time, weekStartDay time.Weekday, sets []reportOccurrenceSet) ([]reportBarView, int, int) {
	type weekBar struct {
		start time.Time
		end   time.Time
		bar   reportBarView
	}

	var weeks []weekBar
	for weekNumber, start := 1, periodStart; !start.After(periodEnd); weekNumber++ {
		end := services.WeekEndFor(start, weekStartDay)
		if end.After(periodEnd) {
			end = periodEnd
		}
		weeks = append(weeks, weekBar{
			start: start,
			end:   end,
			bar: reportBarView{
				Label:    fmt.Sprintf("Week %d", weekNumber),
				SubLabel: reportChartRangeLabel(start, end, today),
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
					addOccurrence(&userWeeks[i].bar, occ, today)
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

func buildReportMatrix(weekStart, weekEnd, today time.Time, sets []reportOccurrenceSet) ([]reportWeekDayView, []reportTaskRowView, int, int) {
	days := make([]reportWeekDayView, 0, 7)
	dayIndexes := map[string]int{}
	for d := dateOnly(weekStart); !d.After(weekEnd); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		dayIndexes[key] = len(days)
		days = append(days, reportWeekDayView{
			Date:      d.Format("02 Jan 2006"),
			Label:     d.Format("Mon"),
			ShortDate: d.Format("02 Jan"),
			sortDate:  key,
		})
	}

	type rowKey struct {
		userID int64
		taskID int64
	}
	rowIndexes := map[rowKey]int{}
	rows := make([]reportTaskRowView, 0)
	done := 0
	due := 0

	for _, set := range sets {
		for _, occ := range set.Occurrences {
			dueDate := dateOnly(occ.DueDate)
			if dueDate.Before(weekStart) || dueDate.After(weekEnd) {
				continue
			}
			dayIndex, ok := dayIndexes[dueDate.Format("2006-01-02")]
			if !ok {
				continue
			}

			countsTowardDue := occurrenceCountsTowardDue(occ, today)
			if countsTowardDue {
				due++
			}
			if occ.Status == services.StatusCompleted {
				done++
			}

			key := rowKey{userID: set.User.ID, taskID: occ.Task.ID}
			rowIndex, ok := rowIndexes[key]
			if !ok {
				rowIndex = len(rows)
				rowIndexes[key] = rowIndex
				rows = append(rows, reportTaskRowView{
					TaskID:       occ.Task.ID,
					TaskName:     occ.Task.Title,
					Description:  occ.Task.Description.String,
					Frequency:    string(occ.Task.Frequency),
					UserName:     set.User.Name,
					Cells:        make([]reportMatrixCellView, len(days)),
					sortSequence: occ.Task.Sequence,
					sortTitle:    occ.Task.Title,
				})
			}
			rows[rowIndex].Cells[dayIndex] = reportMatrixCellView{
				Scheduled:   true,
				Status:      string(occ.Status),
				StatusLabel: reportStatusLabel(occ.Status),
				CompletedAt: reportCompletedAtLabel(occ.CompletedAt),
			}
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].UserName != rows[j].UserName {
			return rows[i].UserName < rows[j].UserName
		}
		if rows[i].sortSequence != rows[j].sortSequence {
			return rows[i].sortSequence < rows[j].sortSequence
		}
		if rows[i].sortTitle != rows[j].sortTitle {
			return rows[i].sortTitle < rows[j].sortTitle
		}
		return rows[i].TaskID < rows[j].TaskID
	})
	return days, rows, done, due
}

func selectedReportWeekLabels(monthStart, monthEnd, weekStart, weekEnd time.Time, weekStartDay time.Weekday) (string, string) {
	weeks, label := reportWeekRanges(monthStart, monthEnd, weekStartDay)
	for _, week := range weeks {
		if !weekStart.After(week.end) && !weekEnd.Before(week.start) {
			return week.label, fmt.Sprintf("%s - %s", weekStart.Format("02 Jan"), weekEnd.Format("02 Jan"))
		}
	}
	return label, fmt.Sprintf("%s - %s", weekStart.Format("02 Jan"), weekEnd.Format("02 Jan"))
}

type reportWeekRange struct {
	start    time.Time
	end      time.Time
	label    string
	subLabel string
}

func reportWeekRanges(periodStart, periodEnd time.Time, weekStartDay time.Weekday) ([]reportWeekRange, string) {
	weeks := make([]reportWeekRange, 0)
	lastLabel := "Week 1"
	for weekNumber, start := 1, periodStart; !start.After(periodEnd); weekNumber++ {
		end := services.WeekEndFor(start, weekStartDay)
		if end.After(periodEnd) {
			end = periodEnd
		}
		label := fmt.Sprintf("Week %d", weekNumber)
		lastLabel = label
		weeks = append(weeks, reportWeekRange{
			start:    start,
			end:      end,
			label:    label,
			subLabel: fmt.Sprintf("%s - %s", start.Format("02 Jan"), end.Format("02 Jan")),
		})
		start = end.AddDate(0, 0, 1)
	}
	return weeks, lastLabel
}

func reportChartRangeLabel(start, end, today time.Time) string {
	start = dateOnly(start)
	end = dateOnly(end)
	today = dateOnly(today)
	if start.After(today) {
		return ""
	}
	if end.After(today) {
		end = today
	}
	return fmt.Sprintf("%s - %s", start.Format("02 Jan"), end.Format("02 Jan"))
}

func reportStatusLabel(status services.OccurrenceStatus) string {
	switch status {
	case services.StatusCompleted:
		return "completed"
	case services.StatusMissed:
		return "missed"
	case services.StatusExempt:
		return "exempt"
	default:
		return "pending"
	}
}

func reportCompletedAtLabel(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(services.AppLocation).Format("02 Jan 2006 15:04")
}

func addOccurrence(bar *reportBarView, occ services.TaskOccurrence, today time.Time) {
	if !occurrenceCountsTowardDue(occ, today) {
		return
	}
	bar.Total++
	if occ.Status == services.StatusCompleted {
		bar.Completed++
	}
}

func occurrenceCountsTowardDue(occ services.TaskOccurrence, today time.Time) bool {
	if occ.Status == services.StatusExempt {
		return false
	}
	return !dateOnly(occ.DueDate).After(dateOnly(today))
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
