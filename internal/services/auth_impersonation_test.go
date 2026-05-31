package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/remorac/mutabaah/internal/repository"
)

type authImpersonationStore struct {
	users    map[int64]repository.User
	sessions map[string]repository.GetSessionRow
}

func newAuthImpersonationStore() *authImpersonationStore {
	return &authImpersonationStore{
		users:    map[int64]repository.User{},
		sessions: map[string]repository.GetSessionRow{},
	}
}

func (s *authImpersonationStore) CreateSession(ctx context.Context, arg repository.CreateSessionParams) error {
	s.sessions[arg.ID] = repository.GetSessionRow{
		ID:                 arg.ID,
		UserID:             arg.UserID,
		ImpersonatorUserID: arg.ImpersonatorUserID,
		ExpiresAt:          arg.ExpiresAt,
		CreatedAt:          time.Now(),
	}
	return nil
}

func (s *authImpersonationStore) DeleteSession(ctx context.Context, id string) error {
	delete(s.sessions, id)
	return nil
}

func (s *authImpersonationStore) GetSession(ctx context.Context, id string) (repository.GetSessionRow, error) {
	session, ok := s.sessions[id]
	if !ok || !session.ExpiresAt.After(time.Now()) {
		return repository.GetSessionRow{}, sql.ErrNoRows
	}
	return session, nil
}

func (s *authImpersonationStore) GetUserByEmail(ctx context.Context, email string) (repository.User, error) {
	for _, user := range s.users {
		if user.Email == email {
			return user, nil
		}
	}
	return repository.User{}, sql.ErrNoRows
}

func (s *authImpersonationStore) GetUserByID(ctx context.Context, id int64) (repository.User, error) {
	user, ok := s.users[id]
	if !ok {
		return repository.User{}, sql.ErrNoRows
	}
	return user, nil
}

func TestStartImpersonationRotatesSessionToTarget(t *testing.T) {
	store := newAuthImpersonationStore()
	admin := repository.User{ID: 1, Email: "admin@example.com", Name: "Admin", Role: repository.UsersRoleAdmin, IsActive: true}
	target := repository.User{ID: 2, Email: "user@example.com", Name: "User", Role: repository.UsersRoleUser, IsActive: true}
	store.users[admin.ID] = admin
	store.users[target.ID] = target
	store.sessions["old"] = repository.GetSessionRow{ID: "old", UserID: admin.ID, ExpiresAt: time.Now().Add(time.Hour)}

	auth := NewAuthService(store, "test-secret", 1)
	token, gotTarget, err := auth.StartImpersonation(context.Background(), "old", admin, target.ID)
	if err != nil {
		t.Fatalf("StartImpersonation() error = %v", err)
	}
	if token == "" || token == "old" {
		t.Fatalf("token = %q, want rotated token", token)
	}
	if gotTarget.ID != target.ID {
		t.Fatalf("target ID = %d, want %d", gotTarget.ID, target.ID)
	}
	if _, ok := store.sessions["old"]; ok {
		t.Fatalf("old session still exists")
	}
	session := store.sessions[token]
	if session.UserID != target.ID {
		t.Fatalf("session user = %d, want target %d", session.UserID, target.ID)
	}
	if !session.ImpersonatorUserID.Valid || session.ImpersonatorUserID.Int64 != admin.ID {
		t.Fatalf("impersonator = %+v, want admin %d", session.ImpersonatorUserID, admin.ID)
	}
}

func TestStartImpersonationRejectsInvalidActorsAndTargets(t *testing.T) {
	store := newAuthImpersonationStore()
	admin := repository.User{ID: 1, Role: repository.UsersRoleAdmin, IsActive: true}
	user := repository.User{ID: 2, Role: repository.UsersRoleUser, IsActive: true}
	store.users[admin.ID] = admin
	auth := NewAuthService(store, "test-secret", 1)

	if _, _, err := auth.StartImpersonation(context.Background(), "old", user, admin.ID); !errors.Is(err, ErrImpersonationForbidden) {
		t.Fatalf("non-admin error = %v, want ErrImpersonationForbidden", err)
	}
	if _, _, err := auth.StartImpersonation(context.Background(), "old", admin, admin.ID); !errors.Is(err, ErrSelfImpersonation) {
		t.Fatalf("self error = %v, want ErrSelfImpersonation", err)
	}
	if _, _, err := auth.StartImpersonation(context.Background(), "old", admin, 99); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing target error = %v, want ErrUserNotFound", err)
	}
}

func TestLoginRejectsInactiveUser(t *testing.T) {
	store := newAuthImpersonationStore()
	hash, err := bcrypt.GenerateFromPassword([]byte("password-123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	store.users[1] = repository.User{
		ID:           1,
		Email:        "inactive@example.com",
		PasswordHash: string(hash),
		Role:         repository.UsersRoleUser,
		IsActive:     false,
	}

	auth := NewAuthService(store, "test-secret", 1)
	if _, _, err := auth.Login(context.Background(), "inactive@example.com", "password-123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	if len(store.sessions) != 0 {
		t.Fatalf("inactive login created sessions: %#v", store.sessions)
	}
}

func TestLookupSessionDestroysInactiveUserSession(t *testing.T) {
	store := newAuthImpersonationStore()
	store.users[2] = repository.User{ID: 2, Role: repository.UsersRoleUser, IsActive: false}
	store.sessions["inactive"] = repository.GetSessionRow{ID: "inactive", UserID: 2, ExpiresAt: time.Now().Add(time.Hour)}

	auth := NewAuthService(store, "test-secret", 1)
	if _, err := auth.LookupSessionInfo(context.Background(), "inactive"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("LookupSessionInfo() error = %v, want ErrSessionNotFound", err)
	}
	if _, ok := store.sessions["inactive"]; ok {
		t.Fatalf("inactive user's session was not destroyed")
	}
}

func TestStartImpersonationRejectsInactiveTarget(t *testing.T) {
	store := newAuthImpersonationStore()
	admin := repository.User{ID: 1, Role: repository.UsersRoleAdmin, IsActive: true}
	target := repository.User{ID: 2, Role: repository.UsersRoleUser, IsActive: false}
	store.users[admin.ID] = admin
	store.users[target.ID] = target
	auth := NewAuthService(store, "test-secret", 1)

	if _, _, err := auth.StartImpersonation(context.Background(), "old", admin, target.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("StartImpersonation() error = %v, want ErrUserNotFound", err)
	}
}

func TestStopImpersonationRestoresAdminSession(t *testing.T) {
	store := newAuthImpersonationStore()
	admin := repository.User{ID: 1, Role: repository.UsersRoleAdmin, IsActive: true}
	target := repository.User{ID: 2, Role: repository.UsersRoleUser, IsActive: true}
	store.users[admin.ID] = admin
	store.users[target.ID] = target
	store.sessions["imp"] = repository.GetSessionRow{
		ID:                 "imp",
		UserID:             target.ID,
		ImpersonatorUserID: sql.NullInt64{Int64: admin.ID, Valid: true},
		ExpiresAt:          time.Now().Add(time.Hour),
	}

	auth := NewAuthService(store, "test-secret", 1)
	token, gotAdmin, err := auth.StopImpersonation(context.Background(), "imp")
	if err != nil {
		t.Fatalf("StopImpersonation() error = %v", err)
	}
	if gotAdmin.ID != admin.ID {
		t.Fatalf("admin ID = %d, want %d", gotAdmin.ID, admin.ID)
	}
	if _, ok := store.sessions["imp"]; ok {
		t.Fatalf("impersonated session still exists")
	}
	session := store.sessions[token]
	if session.UserID != admin.ID {
		t.Fatalf("restored session user = %d, want admin %d", session.UserID, admin.ID)
	}
	if session.ImpersonatorUserID.Valid {
		t.Fatalf("restored session still has impersonator: %+v", session.ImpersonatorUserID)
	}
}

func TestStopImpersonationDestroysSessionWhenAdminInvalid(t *testing.T) {
	store := newAuthImpersonationStore()
	target := repository.User{ID: 2, Role: repository.UsersRoleUser, IsActive: true}
	store.users[target.ID] = target
	store.sessions["imp"] = repository.GetSessionRow{
		ID:                 "imp",
		UserID:             target.ID,
		ImpersonatorUserID: sql.NullInt64{Int64: 1, Valid: true},
		ExpiresAt:          time.Now().Add(time.Hour),
	}

	auth := NewAuthService(store, "test-secret", 1)
	if _, _, err := auth.StopImpersonation(context.Background(), "imp"); !errors.Is(err, ErrImpersonatorInvalid) {
		t.Fatalf("StopImpersonation() error = %v, want ErrImpersonatorInvalid", err)
	}
	if _, ok := store.sessions["imp"]; ok {
		t.Fatalf("invalid impersonated session still exists")
	}
}

func TestStopImpersonationRejectsRegularSession(t *testing.T) {
	store := newAuthImpersonationStore()
	admin := repository.User{ID: 1, Role: repository.UsersRoleAdmin, IsActive: true}
	store.users[admin.ID] = admin
	store.sessions["regular"] = repository.GetSessionRow{ID: "regular", UserID: admin.ID, ExpiresAt: time.Now().Add(time.Hour)}

	auth := NewAuthService(store, "test-secret", 1)
	if _, _, err := auth.StopImpersonation(context.Background(), "regular"); !errors.Is(err, ErrNotImpersonating) {
		t.Fatalf("StopImpersonation() error = %v, want ErrNotImpersonating", err)
	}
}
