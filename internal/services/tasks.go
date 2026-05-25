package services

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/aldoerianda/tracker/internal/repository"
)

// ErrTaskNotAvailable is returned when the task does not exist or is inactive.
var ErrTaskNotAvailable = errors.New("task not available")

// ErrInvalidOccurrenceDate is returned when the supplied due_date is not a
// real occurrence of the task (e.g. wrong weekday for a weekly task, or a
// date before start_date / after end_date).
var ErrInvalidOccurrenceDate = errors.New("date is not an occurrence of this task")

// OccurrenceStatus is the lifecycle state of a single (task, date) occurrence.
type OccurrenceStatus string

const (
	StatusPending   OccurrenceStatus = "pending"
	StatusMissed    OccurrenceStatus = "missed"
	StatusCompleted OccurrenceStatus = "completed"
)

// TaskOccurrence is one scheduled instance of a task on a specific date,
// annotated with its current completion status for the requesting user.
type TaskOccurrence struct {
	Task        repository.Task
	DueDate     time.Time
	Status      OccurrenceStatus
	CompletedAt time.Time
}

// TaskService computes task occurrences and merges them with completion records.
type TaskService struct {
	q   *repository.Queries
	now func() time.Time
}

func NewTaskService(q *repository.Queries) *TaskService {
	return &TaskService{q: q, now: time.Now}
}

// OccurrencesOn returns occurrences for a single date, using the service's
// own clock to assign pending/missed status. Prefer OccurrencesOnAsOf when
// the caller knows the app's "today" (in the app timezone).
func (s *TaskService) OccurrencesOn(ctx context.Context, userID int64, date time.Time) ([]TaskOccurrence, error) {
	return s.OccurrencesBetween(ctx, userID, date, date)
}

// OccurrencesBetween returns occurrences in [from, to] inclusive, sorted by
// due date then task title. All active tasks are considered; completion state
// is per-user. Status is computed against the service clock; see
// OccurrencesBetweenAsOf for callers that want to anchor "today" to the app
// timezone.
func (s *TaskService) OccurrencesBetween(ctx context.Context, userID int64, from, to time.Time) ([]TaskOccurrence, error) {
	return s.OccurrencesBetweenAsOf(ctx, userID, from, to, dateOnly(s.now()))
}

// OccurrencesOnAsOf is OccurrencesOn but with caller-supplied "today" so the
// pending/missed split honours the app timezone.
func (s *TaskService) OccurrencesOnAsOf(ctx context.Context, userID int64, date, today time.Time) ([]TaskOccurrence, error) {
	return s.OccurrencesBetweenAsOf(ctx, userID, date, date, today)
}

// OccurrencesBetweenAsOf is OccurrencesBetween with caller-supplied "today".
func (s *TaskService) OccurrencesBetweenAsOf(ctx context.Context, userID int64, from, to, today time.Time) ([]TaskOccurrence, error) {
	from = dateOnly(from)
	to = dateOnly(to)
	if to.Before(from) {
		return nil, nil
	}

	tasks, err := s.q.ListActiveTasks(ctx)
	if err != nil {
		return nil, err
	}
	completions, err := s.q.ListCompletionsForUserInRange(ctx, repository.ListCompletionsForUserInRangeParams{
		UserID:      userID,
		FromDueDate: from,
		ToDueDate:   to,
	})
	if err != nil {
		return nil, err
	}

	return buildOccurrences(tasks, completions, dateOnly(today), from, to), nil
}

// Toggle flips the completion state for (taskID, userID, dueDate) and returns
// the resulting occurrence so callers can re-render its row. The task must be
// active, and dueDate must be a real occurrence of the task per its schedule
// — otherwise the call is rejected without touching the DB.
// Uses the service clock for the "today" comparison; ToggleAsOf accepts an
// explicit today so callers can honour the app timezone.
func (s *TaskService) Toggle(ctx context.Context, taskID, userID int64, dueDate time.Time) (TaskOccurrence, error) {
	return s.ToggleAsOf(ctx, taskID, userID, dueDate, dateOnly(s.now()))
}

// ToggleAsOf is Toggle with caller-supplied "today" for status assignment.
func (s *TaskService) ToggleAsOf(ctx context.Context, taskID, userID int64, dueDate, today time.Time) (TaskOccurrence, error) {
	dueDate = dateOnly(dueDate)
	today = dateOnly(today)

	tasks, err := s.q.ListActiveTasks(ctx)
	if err != nil {
		return TaskOccurrence{}, err
	}
	var task repository.Task
	found := false
	for _, t := range tasks {
		if t.ID == taskID {
			task = t
			found = true
			break
		}
	}
	if !found {
		return TaskOccurrence{}, ErrTaskNotAvailable
	}

	validDates := occurrenceDates(task, dueDate, dueDate)
	if len(validDates) == 0 || !validDates[0].Equal(dueDate) {
		return TaskOccurrence{}, ErrInvalidOccurrenceDate
	}

	_, lookupErr := s.q.GetCompletion(ctx, repository.GetCompletionParams{
		TaskID:  taskID,
		UserID:  userID,
		DueDate: dueDate,
	})
	alreadyCompleted := lookupErr == nil
	if !alreadyCompleted && !errors.Is(lookupErr, sql.ErrNoRows) {
		return TaskOccurrence{}, lookupErr
	}

	occ := TaskOccurrence{Task: task, DueDate: dueDate}

	if alreadyCompleted {
		if err := s.q.MarkTaskIncomplete(ctx, repository.MarkTaskIncompleteParams{
			TaskID:  taskID,
			UserID:  userID,
			DueDate: dueDate,
		}); err != nil {
			return TaskOccurrence{}, err
		}
		if dueDate.Before(today) {
			occ.Status = StatusMissed
		} else {
			occ.Status = StatusPending
		}
		return occ, nil
	}

	if err := s.q.MarkTaskComplete(ctx, repository.MarkTaskCompleteParams{
		TaskID:  taskID,
		UserID:  userID,
		DueDate: dueDate,
	}); err != nil {
		return TaskOccurrence{}, err
	}
	c, err := s.q.GetCompletion(ctx, repository.GetCompletionParams{
		TaskID:  taskID,
		UserID:  userID,
		DueDate: dueDate,
	})
	if err != nil {
		return TaskOccurrence{}, err
	}
	occ.Status = StatusCompleted
	occ.CompletedAt = c.CompletedAt
	return occ, nil
}

// buildOccurrences is the pure core of the resolver: given task definitions,
// any existing completion rows in the range, and "today", it produces the
// final list of occurrences with their status assigned.
func buildOccurrences(tasks []repository.Task, completions []repository.TaskCompletion, today, from, to time.Time) []TaskOccurrence {
	type key struct {
		taskID int64
		day    string
	}
	completionByKey := make(map[key]repository.TaskCompletion, len(completions))
	for _, c := range completions {
		completionByKey[key{c.TaskID, dateOnly(c.DueDate).Format("2006-01-02")}] = c
	}

	var out []TaskOccurrence
	for _, t := range tasks {
		for _, d := range occurrenceDates(t, from, to) {
			occ := TaskOccurrence{Task: t, DueDate: d}
			if c, ok := completionByKey[key{t.ID, d.Format("2006-01-02")}]; ok {
				occ.Status = StatusCompleted
				occ.CompletedAt = c.CompletedAt
			} else if d.Before(today) {
				occ.Status = StatusMissed
			} else {
				occ.Status = StatusPending
			}
			out = append(out, occ)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].DueDate.Equal(out[j].DueDate) {
			return out[i].DueDate.Before(out[j].DueDate)
		}
		if out[i].Task.Sequence != out[j].Task.Sequence {
			return out[i].Task.Sequence < out[j].Task.Sequence
		}
		return out[i].Task.Title < out[j].Task.Title
	})
	return out
}

// occurrenceDates enumerates the dates a task is due within [from, to],
// honouring its frequency, start_date, and optional end_date.
//
// Month-end semantics: monthly/yearly tasks only generate on months/years
// where the start day-of-month actually exists. A task starting Jan 31 skips
// Feb/Apr/Jun/Sep/Nov; a yearly task on Feb 29 only fires on leap years.
func occurrenceDates(t repository.Task, from, to time.Time) []time.Time {
	start := dateOnly(t.StartDate)
	rangeEnd := to
	if t.EndDate.Valid {
		taskEnd := dateOnly(t.EndDate.Time)
		if taskEnd.Before(rangeEnd) {
			rangeEnd = taskEnd
		}
	}
	rangeStart := from
	if rangeStart.Before(start) {
		rangeStart = start
	}
	if rangeStart.After(rangeEnd) {
		return nil
	}

	var out []time.Time
	switch t.Frequency {
	case repository.TasksFrequencyDaily:
		for d := rangeStart; !d.After(rangeEnd); d = d.AddDate(0, 0, 1) {
			out = append(out, d)
		}

	case repository.TasksFrequencyWeekly:
		daysSinceStart := int(rangeStart.Sub(start).Hours() / 24)
		mod := daysSinceStart % 7
		first := rangeStart
		if mod != 0 {
			first = rangeStart.AddDate(0, 0, 7-mod)
		}
		for d := first; !d.After(rangeEnd); d = d.AddDate(0, 0, 7) {
			out = append(out, d)
		}

	case repository.TasksFrequencyMonthly:
		day := start.Day()
		y, m, _ := rangeStart.Date()
		for {
			if day <= daysInMonth(y, m) {
				cand := time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
				if cand.After(rangeEnd) {
					return out
				}
				if !cand.Before(start) && !cand.Before(rangeStart) {
					out = append(out, cand)
				}
			}
			m++
			if m > 12 {
				m = 1
				y++
			}
			if time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).After(rangeEnd) {
				break
			}
		}

	case repository.TasksFrequencyYearly:
		mo := start.Month()
		day := start.Day()
		for y := rangeStart.Year(); ; y++ {
			if day <= daysInMonth(y, mo) {
				cand := time.Date(y, mo, day, 0, 0, 0, 0, time.UTC)
				if cand.After(rangeEnd) {
					return out
				}
				if !cand.Before(start) && !cand.Before(rangeStart) {
					out = append(out, cand)
				}
			}
			if time.Date(y+1, mo, 1, 0, 0, 0, 0, time.UTC).After(rangeEnd) {
				break
			}
		}
	}
	return out
}

func daysInMonth(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
