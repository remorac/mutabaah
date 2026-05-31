package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/remorac/mutabaah/internal/repository"
)

type fakeUserAdminStore struct {
	users              map[int64]repository.User
	usersByEmail       map[string]repository.User
	activeAdminCount   int64
	updated            repository.UpdateUserParams
	updateWasCalled    bool
	deletedSessionsFor int64
}

func newFakeUserAdminStore() *fakeUserAdminStore {
	return &fakeUserAdminStore{
		users:        map[int64]repository.User{},
		usersByEmail: map[string]repository.User{},
	}
}

func (s *fakeUserAdminStore) CountActiveAdmins(ctx context.Context) (int64, error) {
	return s.activeAdminCount, nil
}

func (s *fakeUserAdminStore) CountUsers(ctx context.Context) (int64, error) {
	return int64(len(s.users)), nil
}

func (s *fakeUserAdminStore) CreateUser(ctx context.Context, arg repository.CreateUserParams) (int64, error) {
	return 1, nil
}

func (s *fakeUserAdminStore) DeleteCompletionsForUserOnDates(ctx context.Context, arg repository.DeleteCompletionsForUserOnDatesParams) error {
	return nil
}

func (s *fakeUserAdminStore) DeleteUser(ctx context.Context, id int64) error {
	return nil
}

func (s *fakeUserAdminStore) DeleteUserSessions(ctx context.Context, arg repository.DeleteUserSessionsParams) error {
	s.deletedSessionsFor = arg.UserID
	return nil
}

func (s *fakeUserAdminStore) GetUserByEmail(ctx context.Context, email string) (repository.User, error) {
	if u, ok := s.usersByEmail[email]; ok {
		return u, nil
	}
	return repository.User{}, sql.ErrNoRows
}

func (s *fakeUserAdminStore) GetUserByID(ctx context.Context, id int64) (repository.User, error) {
	if u, ok := s.users[id]; ok {
		return u, nil
	}
	return repository.User{}, sql.ErrNoRows
}

func (s *fakeUserAdminStore) ListActiveUsers(ctx context.Context) ([]repository.User, error) {
	var users []repository.User
	for _, u := range s.users {
		if u.IsActive {
			users = append(users, u)
		}
	}
	return users, nil
}

func (s *fakeUserAdminStore) ListActiveRegularUsers(ctx context.Context) ([]repository.User, error) {
	var users []repository.User
	for _, u := range s.users {
		if u.IsActive && u.Role == repository.UsersRoleUser {
			users = append(users, u)
		}
	}
	return users, nil
}

func (s *fakeUserAdminStore) ListAllUsers(ctx context.Context) ([]repository.User, error) {
	var users []repository.User
	for _, u := range s.users {
		users = append(users, u)
	}
	return users, nil
}

func (s *fakeUserAdminStore) ListUsers(ctx context.Context, arg repository.ListUsersParams) ([]repository.User, error) {
	return s.ListAllUsers(ctx)
}

func (s *fakeUserAdminStore) UpdateUser(ctx context.Context, arg repository.UpdateUserParams) error {
	s.updated = arg
	s.updateWasCalled = true
	return nil
}

func (s *fakeUserAdminStore) UpdateUserAvatar(ctx context.Context, arg repository.UpdateUserAvatarParams) error {
	return nil
}

func (s *fakeUserAdminStore) UpdateUserPassword(ctx context.Context, arg repository.UpdateUserPasswordParams) error {
	return nil
}

func TestUpdateRejectsSelfDeactivation(t *testing.T) {
	store := newFakeUserAdminStore()
	store.users[1] = repository.User{ID: 1, Email: "admin@example.com", Name: "Admin", Role: repository.UsersRoleAdmin, IsActive: true}
	store.activeAdminCount = 2
	svc := NewUserAdminService(store)

	err := svc.Update(context.Background(), 1, UserInput{
		Email:         "admin@example.com",
		Name:          "Admin",
		Role:          string(repository.UsersRoleAdmin),
		IsActive:      false,
		CurrentUserID: 1,
	})

	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Fields["is_active"] == "" {
		t.Fatalf("Update() error = %v, want is_active validation error", err)
	}
	if store.updateWasCalled {
		t.Fatalf("inactive self update should not be persisted")
	}
}

func TestUpdateRejectsRemovingLastActiveAdmin(t *testing.T) {
	store := newFakeUserAdminStore()
	store.users[1] = repository.User{ID: 1, Email: "admin@example.com", Name: "Admin", Role: repository.UsersRoleAdmin, IsActive: true}
	store.activeAdminCount = 1
	svc := NewUserAdminService(store)

	err := svc.Update(context.Background(), 1, UserInput{
		Email:         "admin@example.com",
		Name:          "Admin",
		Role:          string(repository.UsersRoleUser),
		IsActive:      true,
		CurrentUserID: 2,
	})

	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("Update() error = %v, want ErrLastAdmin", err)
	}
}

func TestUpdateDeactivatingUserRevokesSessions(t *testing.T) {
	store := newFakeUserAdminStore()
	store.users[2] = repository.User{ID: 2, Email: "user@example.com", Name: "User", Role: repository.UsersRoleUser, IsActive: true}
	store.activeAdminCount = 1
	svc := NewUserAdminService(store)

	if err := svc.Update(context.Background(), 2, UserInput{
		Email:         "user@example.com",
		Name:          "User",
		Role:          string(repository.UsersRoleUser),
		IsActive:      false,
		CurrentUserID: 1,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !store.updateWasCalled || store.updated.IsActive {
		t.Fatalf("updated = %+v, want inactive update", store.updated)
	}
	if store.deletedSessionsFor != 2 {
		t.Fatalf("deletedSessionsFor = %d, want 2", store.deletedSessionsFor)
	}
}

func TestListActiveOnlyReturnsActiveUsers(t *testing.T) {
	store := newFakeUserAdminStore()
	store.users[1] = repository.User{ID: 1, Name: "Active", IsActive: true}
	store.users[2] = repository.User{ID: 2, Name: "Inactive", IsActive: false}
	svc := NewUserAdminService(store)

	users, err := svc.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(users) != 1 || users[0].ID != 1 {
		t.Fatalf("ListActive() = %#v, want only active user", users)
	}
}

func TestListActiveRegularExcludesAdminsAndInactiveUsers(t *testing.T) {
	store := newFakeUserAdminStore()
	store.users[1] = repository.User{ID: 1, Name: "Admin", Role: repository.UsersRoleAdmin, IsActive: true}
	store.users[2] = repository.User{ID: 2, Name: "Active User", Role: repository.UsersRoleUser, IsActive: true}
	store.users[3] = repository.User{ID: 3, Name: "Inactive User", Role: repository.UsersRoleUser, IsActive: false}
	svc := NewUserAdminService(store)

	users, err := svc.ListActiveRegular(context.Background())
	if err != nil {
		t.Fatalf("ListActiveRegular() error = %v", err)
	}
	if len(users) != 1 || users[0].ID != 2 {
		t.Fatalf("ListActiveRegular() = %#v, want only active regular user", users)
	}
}

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
