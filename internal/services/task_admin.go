package services

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aldoerianda/tracker/internal/repository"
)

// ErrTaskNotFound is returned when a task lookup misses.
var ErrTaskNotFound = errors.New("task not found")

// ValidationError carries field-level errors back to the form renderer.
// Keyed by form field name so templates can show inline messages.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	sort.Strings(parts)
	return "validation failed: " + strings.Join(parts, "; ")
}

// TaskInput is the payload accepted by Create/Update. Strings should already
// be trimmed by the caller; the service validates and applies defaults.
type TaskInput struct {
	Title       string
	Description string
	Frequency   string
	StartDate   string // YYYY-MM-DD
	EndDate     string // YYYY-MM-DD or ""
	Active      bool
	Sequence    string // integer as string; defaults to 0
}

// TaskListFilter narrows the settings task listing.
type TaskListFilter struct {
	Search string // case-insensitive substring on title or description
}

// TaskAdminService implements admin-side task CRUD with validation.
type TaskAdminService struct {
	q *repository.Queries
}

func NewTaskAdminService(q *repository.Queries) *TaskAdminService {
	return &TaskAdminService{q: q}
}

// List returns all tasks (active + inactive) matching the filter, sorted by
// creation time descending — matches ListTasks ordering.
func (s *TaskAdminService) List(ctx context.Context, f TaskListFilter) ([]repository.Task, error) {
	tasks, err := s.q.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	search := strings.ToLower(strings.TrimSpace(f.Search))

	out := tasks[:0]
	for _, t := range tasks {
		if search != "" {
			title := strings.ToLower(t.Title)
			desc := ""
			if t.Description.Valid {
				desc = strings.ToLower(t.Description.String)
			}
			if !strings.Contains(title, search) && !strings.Contains(desc, search) {
				continue
			}
		}
		out = append(out, t)
	}
	return out, nil
}

// Get fetches a task by id.
func (s *TaskAdminService) Get(ctx context.Context, id int64) (repository.Task, error) {
	t, err := s.q.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.Task{}, ErrTaskNotFound
		}
		return repository.Task{}, err
	}
	return t, nil
}

// Create validates the input and inserts a task.
func (s *TaskAdminService) Create(ctx context.Context, in TaskInput) (int64, error) {
	params, err := s.validateAndBuild(in)
	if err != nil {
		return 0, err
	}
	return s.q.CreateTask(ctx, params)
}

// Update validates and updates an existing task.
func (s *TaskAdminService) Update(ctx context.Context, id int64, in TaskInput) error {
	if _, err := s.q.GetTask(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskNotFound
		}
		return err
	}
	params, err := s.validateAndBuild(in)
	if err != nil {
		return err
	}
	return s.q.UpdateTask(ctx, repository.UpdateTaskParams{
		Title:       params.Title,
		Description: params.Description,
		Frequency:   params.Frequency,
		StartDate:   params.StartDate,
		EndDate:     params.EndDate,
		Active:      params.Active,
		Sequence:    params.Sequence,
		ID:          id,
	})
}

// Move shifts a task one position up (delta=-1) or down (delta=+1) in the
// ordered task list. Sequences are normalised across all tasks on every move
// so ties created by legacy zero-defaults don't leave the list unchanged.
func (s *TaskAdminService) Move(ctx context.Context, id int64, delta int) error {
	if delta != -1 && delta != 1 {
		return errors.New("delta must be -1 or 1")
	}
	tasks, err := s.q.ListTasks(ctx)
	if err != nil {
		return err
	}
	idx := -1
	for i, t := range tasks {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrTaskNotFound
	}
	target := idx + delta
	if target < 0 || target >= len(tasks) {
		return nil // already at the edge; no-op
	}
	tasks[idx], tasks[target] = tasks[target], tasks[idx]
	for i, t := range tasks {
		newSeq := int32((i + 1) * 10)
		if t.Sequence == newSeq {
			continue
		}
		if err := s.q.SetTaskSequence(ctx, repository.SetTaskSequenceParams{
			ID:       t.ID,
			Sequence: newSeq,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Delete permanently removes a task and (via FK ON DELETE CASCADE) all of its
// completion history.
func (s *TaskAdminService) Delete(ctx context.Context, id int64) error {
	if _, err := s.q.GetTask(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskNotFound
		}
		return err
	}
	return s.q.DeleteTask(ctx, id)
}

func (s *TaskAdminService) validateAndBuild(in TaskInput) (repository.CreateTaskParams, error) {
	verrs := map[string]string{}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		verrs["title"] = "Title is required."
	} else if len(title) > 255 {
		verrs["title"] = "Title must be 255 characters or fewer."
	}

	freq := repository.TasksFrequency(strings.TrimSpace(in.Frequency))
	switch freq {
	case repository.TasksFrequencyDaily, repository.TasksFrequencyWeekly,
		repository.TasksFrequencyMonthly, repository.TasksFrequencyYearly:
	default:
		verrs["frequency"] = "Choose a frequency."
	}

	start, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(in.StartDate), time.UTC)
	if err != nil {
		verrs["start_date"] = "Start date is required (YYYY-MM-DD)."
	}

	var end sql.NullTime
	if s := strings.TrimSpace(in.EndDate); s != "" {
		e, eerr := time.ParseInLocation("2006-01-02", s, time.UTC)
		if eerr != nil {
			verrs["end_date"] = "End date must be YYYY-MM-DD."
		} else {
			if err == nil && e.Before(start) {
				verrs["end_date"] = "End date must be on or after start date."
			}
			end = sql.NullTime{Time: e, Valid: true}
		}
	}

	desc := strings.TrimSpace(in.Description)
	if len(desc) > 64 {
		verrs["description"] = "Description must be 64 characters or fewer."
	}

	var seq int32
	if s := strings.TrimSpace(in.Sequence); s != "" {
		n, perr := strconv.ParseInt(s, 10, 32)
		if perr != nil {
			verrs["sequence"] = "Sequence must be a whole number."
		} else {
			seq = int32(n)
		}
	}

	if len(verrs) > 0 {
		return repository.CreateTaskParams{}, &ValidationError{Fields: verrs}
	}

	var nullDesc sql.NullString
	if desc != "" {
		nullDesc = sql.NullString{String: desc, Valid: true}
	}

	return repository.CreateTaskParams{
		Title:       title,
		Description: nullDesc,
		Frequency:   freq,
		StartDate:   start,
		EndDate:     end,
		Active:      in.Active,
		Sequence:    seq,
	}, nil
}
