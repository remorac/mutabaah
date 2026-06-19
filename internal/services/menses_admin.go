package services

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/remorac/mutabaah/internal/repository"
)

// ErrMensesPeriodNotFound is returned when a period lookup misses or the
// period does not belong to the caller.
var ErrMensesPeriodNotFound = errors.New("menses period not found")

// MensesPeriodInput is the payload accepted by Create/Update. Strings should
// already be trimmed by the caller; the service validates and applies defaults.
type MensesPeriodInput struct {
	StartDate string // YYYY-MM-DD
	EndDate   string // YYYY-MM-DD or "" (ongoing)
}

// mensesStore is the subset of repository.Queries used by MensesAdminService.
// Defining it as an interface keeps the service unit-testable with a fake.
type mensesStore interface {
	ListMensesPeriodsForUser(ctx context.Context, userID int64) ([]repository.MensesPeriod, error)
	GetMensesPeriod(ctx context.Context, arg repository.GetMensesPeriodParams) (repository.MensesPeriod, error)
	CreateMensesPeriod(ctx context.Context, arg repository.CreateMensesPeriodParams) (int64, error)
	UpdateMensesPeriod(ctx context.Context, arg repository.UpdateMensesPeriodParams) error
	DeleteMensesPeriod(ctx context.Context, arg repository.DeleteMensesPeriodParams) error
}

// MensesAdminService implements per-user menses-period CRUD. Every operation
// is scoped to the supplied userID so users cannot touch others' records.
type MensesAdminService struct {
	q mensesStore
}

func NewMensesAdminService(q mensesStore) *MensesAdminService {
	return &MensesAdminService{q: q}
}

func (s *MensesAdminService) List(ctx context.Context, userID int64) ([]repository.MensesPeriod, error) {
	return s.q.ListMensesPeriodsForUser(ctx, userID)
}

func (s *MensesAdminService) Get(ctx context.Context, userID, id int64) (repository.MensesPeriod, error) {
	p, err := s.q.GetMensesPeriod(ctx, repository.GetMensesPeriodParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.MensesPeriod{}, ErrMensesPeriodNotFound
		}
		return repository.MensesPeriod{}, err
	}
	return p, nil
}

func (s *MensesAdminService) Create(ctx context.Context, userID int64, in MensesPeriodInput) (int64, error) {
	start, end, err := s.validateAndCheckOverlap(ctx, userID, 0, in)
	if err != nil {
		return 0, err
	}
	return s.q.CreateMensesPeriod(ctx, repository.CreateMensesPeriodParams{
		UserID:    userID,
		StartDate: start,
		EndDate:   end,
	})
}

func (s *MensesAdminService) Update(ctx context.Context, userID, id int64, in MensesPeriodInput) error {
	if _, err := s.Get(ctx, userID, id); err != nil {
		return err
	}
	start, end, err := s.validateAndCheckOverlap(ctx, userID, id, in)
	if err != nil {
		return err
	}
	return s.q.UpdateMensesPeriod(ctx, repository.UpdateMensesPeriodParams{
		ID:        id,
		UserID:    userID,
		StartDate: start,
		EndDate:   end,
	})
}

func (s *MensesAdminService) Delete(ctx context.Context, userID, id int64) error {
	if _, err := s.Get(ctx, userID, id); err != nil {
		return err
	}
	return s.q.DeleteMensesPeriod(ctx, repository.DeleteMensesPeriodParams{ID: id, UserID: userID})
}

// validateAndCheckOverlap parses input dates, applies field-level validation,
// and rejects overlap with any other period belonging to the same user
// (excluding the row identified by excludeID, when non-zero, for updates).
func (s *MensesAdminService) validateAndCheckOverlap(ctx context.Context, userID, excludeID int64, in MensesPeriodInput) (time.Time, sql.NullTime, error) {
	verrs := map[string]string{}

	start, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(in.StartDate), time.UTC)
	if err != nil {
		verrs["start_date"] = "Start date is required (YYYY-MM-DD)."
	}

	var end sql.NullTime
	if v := strings.TrimSpace(in.EndDate); v != "" {
		e, eerr := time.ParseInLocation("2006-01-02", v, time.UTC)
		if eerr != nil {
			verrs["end_date"] = "End date must be YYYY-MM-DD."
		} else {
			if err == nil && e.Before(start) {
				verrs["end_date"] = "End date must be on or after start date."
			}
			end = sql.NullTime{Time: e, Valid: true}
		}
	}

	if len(verrs) == 0 {
		others, lerr := s.q.ListMensesPeriodsForUser(ctx, userID)
		if lerr != nil {
			return time.Time{}, sql.NullTime{}, lerr
		}
		farFuture := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		newEnd := farFuture
		if end.Valid {
			newEnd = end.Time
		}
		for _, p := range others {
			if p.ID == excludeID {
				continue
			}
			pEnd := farFuture
			if p.EndDate.Valid {
				pEnd = p.EndDate.Time
			}
			if !start.After(pEnd) && !p.StartDate.After(newEnd) {
				verrs["start_date"] = "This period overlaps an existing one."
				break
			}
		}
	}

	if len(verrs) > 0 {
		return time.Time{}, sql.NullTime{}, &ValidationError{Fields: verrs}
	}
	return start, end, nil
}
