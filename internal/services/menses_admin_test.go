package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/remorac/mutabaah/internal/repository"
)

func mustDate(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		panic(err)
	}
	return t
}

// fakeMensesStore is an in-memory mensesStore for unit tests.
type fakeMensesStore struct {
	periods    map[int64]repository.MensesPeriod
	updated    *repository.UpdateMensesPeriodParams
	nextID     int64
	updateFail error
}

func newFakeMensesStore() *fakeMensesStore {
	return &fakeMensesStore{periods: map[int64]repository.MensesPeriod{}, nextID: 1}
}

func (s *fakeMensesStore) add(id, userID int64, start string, end string) {
	p := repository.MensesPeriod{ID: id, UserID: userID, StartDate: mustDate(start)}
	if end != "" {
		p.EndDate = sql.NullTime{Time: mustDate(end), Valid: true}
	}
	s.periods[id] = p
	if id >= s.nextID {
		s.nextID = id + 1
	}
}

func (s *fakeMensesStore) ListMensesPeriodsForUser(ctx context.Context, userID int64) ([]repository.MensesPeriod, error) {
	var out []repository.MensesPeriod
	for _, p := range s.periods {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *fakeMensesStore) GetMensesPeriod(ctx context.Context, arg repository.GetMensesPeriodParams) (repository.MensesPeriod, error) {
	if p, ok := s.periods[arg.ID]; ok && p.UserID == arg.UserID {
		return p, nil
	}
	return repository.MensesPeriod{}, sql.ErrNoRows
}

func (s *fakeMensesStore) CreateMensesPeriod(ctx context.Context, arg repository.CreateMensesPeriodParams) (int64, error) {
	id := s.nextID
	s.nextID++
	s.periods[id] = repository.MensesPeriod{ID: id, UserID: arg.UserID, StartDate: arg.StartDate, EndDate: arg.EndDate}
	return id, nil
}

func (s *fakeMensesStore) UpdateMensesPeriod(ctx context.Context, arg repository.UpdateMensesPeriodParams) error {
	if s.updateFail != nil {
		return s.updateFail
	}
	s.updated = &arg
	p := s.periods[arg.ID]
	p.StartDate = arg.StartDate
	p.EndDate = arg.EndDate
	s.periods[arg.ID] = p
	return nil
}

func (s *fakeMensesStore) DeleteMensesPeriod(ctx context.Context, arg repository.DeleteMensesPeriodParams) error {
	delete(s.periods, arg.ID)
	return nil
}

func TestMensesUpdate_SetsEndDateOnOngoingPeriod(t *testing.T) {
	store := newFakeMensesStore()
	store.add(1, 7, "2026-01-01", "") // ongoing
	svc := NewMensesAdminService(store)

	err := svc.Update(context.Background(), 7, 1, MensesPeriodInput{StartDate: "2026-01-01", EndDate: "2026-01-05"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if store.updated == nil {
		t.Fatal("UpdateMensesPeriod was not called")
	}
	if !store.updated.EndDate.Valid || store.updated.EndDate.Time.Format("2006-01-02") != "2026-01-05" {
		t.Fatalf("end date not persisted, got %+v", store.updated.EndDate)
	}
}

func TestMensesUpdate_AllowsEditingItselfWithoutOverlap(t *testing.T) {
	store := newFakeMensesStore()
	store.add(1, 7, "2026-01-01", "2026-01-05")
	svc := NewMensesAdminService(store)

	// Editing the same row to overlap its own old range must be allowed (the
	// row is excluded from the overlap check).
	if err := svc.Update(context.Background(), 7, 1, MensesPeriodInput{StartDate: "2026-01-02", EndDate: "2026-01-06"}); err != nil {
		t.Fatalf("expected self-edit to succeed, got %v", err)
	}
}

func TestMensesUpdate_RejectsOverlapWithOtherPeriod(t *testing.T) {
	store := newFakeMensesStore()
	store.add(1, 7, "2026-01-01", "2026-01-05")
	store.add(2, 7, "2026-02-01", "2026-02-05")
	svc := NewMensesAdminService(store)

	err := svc.Update(context.Background(), 7, 2, MensesPeriodInput{StartDate: "2026-01-03", EndDate: "2026-01-10"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if _, ok := ve.Fields["start_date"]; !ok {
		t.Fatalf("expected start_date overlap error, got %v", ve.Fields)
	}
}

func TestMensesUpdate_RejectsEndBeforeStart(t *testing.T) {
	store := newFakeMensesStore()
	store.add(1, 7, "2026-01-01", "2026-01-05")
	svc := NewMensesAdminService(store)

	err := svc.Update(context.Background(), 7, 1, MensesPeriodInput{StartDate: "2026-01-10", EndDate: "2026-01-05"})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if _, ok := ve.Fields["end_date"]; !ok {
		t.Fatalf("expected end_date error, got %v", ve.Fields)
	}
}

func TestMensesUpdate_NotFoundForOtherUsersRow(t *testing.T) {
	store := newFakeMensesStore()
	store.add(1, 7, "2026-01-01", "2026-01-05")
	svc := NewMensesAdminService(store)

	err := svc.Update(context.Background(), 99, 1, MensesPeriodInput{StartDate: "2026-01-01", EndDate: "2026-01-05"})
	if !errors.Is(err, ErrMensesPeriodNotFound) {
		t.Fatalf("expected ErrMensesPeriodNotFound, got %v", err)
	}
}
