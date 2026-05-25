package services

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/remorac/mutabaah/internal/repository"
)

// ErrUserNotFound is returned when a user lookup misses.
var ErrUserNotFound = errors.New("user not found")

// ErrLastAdmin is returned when an operation would leave the system with no
// admin user (deleting the last admin, or demoting them to a regular user).
var ErrLastAdmin = errors.New("cannot remove the last admin")

// MinPasswordLength enforces the minimum accepted password length.
const MinPasswordLength = 8

// UserInput is the payload accepted by user create/update. Password is required
// on Create and optional on Update — leave blank to keep the existing hash.
type UserInput struct {
	Email    string
	Name     string
	Role     string
	Password string
}

// AppLocation is the single timezone the app evaluates "today" in.
var AppLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		panic("services: Asia/Jakarta zoneinfo unavailable: " + err.Error())
	}
	return loc
}()

// UserAdminService implements admin-side user CRUD plus the self-service
// password change. Validation lives here so handlers can stay thin.
type UserAdminService struct {
	q *repository.Queries
}

func NewUserAdminService(q *repository.Queries) *UserAdminService {
	return &UserAdminService{q: q}
}

// List returns users in name order. Pagination is layered on top via the
// optional Limit/Offset; pass 0 for Limit to return every row (admin pages
// have a small population so this is safe for v1).
func (s *UserAdminService) List(ctx context.Context, limit, offset int32) ([]repository.User, int64, error) {
	total, err := s.q.CountUsers(ctx)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		users, err := s.q.ListAllUsers(ctx)
		return users, total, err
	}
	users, err := s.q.ListUsers(ctx, repository.ListUsersParams{Limit: limit, Offset: offset})
	return users, total, err
}

// Get fetches a single user by ID.
func (s *UserAdminService) Get(ctx context.Context, id int64) (repository.User, error) {
	u, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.User{}, ErrUserNotFound
		}
		return repository.User{}, err
	}
	return u, nil
}

// Create validates input and inserts a new user, hashing the password.
func (s *UserAdminService) Create(ctx context.Context, in UserInput) (int64, error) {
	email, name, role, verrs := s.validateProfile(ctx, in, 0)
	if pwErr := validatePassword(in.Password); pwErr != "" {
		verrs["password"] = pwErr
	}
	if len(verrs) > 0 {
		return 0, &ValidationError{Fields: verrs}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	return s.q.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: string(hash),
		Name:         name,
		Role:         role,
	})
}

// Update validates and updates an existing user. If Password is empty the
// existing hash is preserved. Demoting the last admin returns ErrLastAdmin.
func (s *UserAdminService) Update(ctx context.Context, id int64, in UserInput) error {
	existing, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	email, name, role, verrs := s.validateProfile(ctx, in, id)
	if in.Password != "" {
		if pwErr := validatePassword(in.Password); pwErr != "" {
			verrs["password"] = pwErr
		}
	}
	if len(verrs) > 0 {
		return &ValidationError{Fields: verrs}
	}
	if existing.Role == repository.UsersRoleAdmin && role != repository.UsersRoleAdmin {
		admins, err := s.q.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}
	if err := s.q.UpdateUser(ctx, repository.UpdateUserParams{
		Email: email,
		Name:  name,
		Role:  role,
		ID:    id,
	}); err != nil {
		return err
	}
	if in.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := s.q.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
			PasswordHash: string(hash),
			ID:           id,
		}); err != nil {
			return err
		}
		// Force re-login on password change so any leaked session is revoked.
		if err := s.q.DeleteUserSessions(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes a user. FK cascades drop their sessions, assignments, and
// completion history. Refuses to delete the last admin.
func (s *UserAdminService) Delete(ctx context.Context, id int64) error {
	u, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	if u.Role == repository.UsersRoleAdmin {
		admins, err := s.q.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}
	return s.q.DeleteUser(ctx, id)
}

// ChangePassword is the self-service flow. The caller must supply a new
// password meeting MinPasswordLength. All of the user's sessions are revoked on
// success.
func (s *UserAdminService) ChangePassword(ctx context.Context, userID int64, next, confirm string) error {
	verrs := map[string]string{}
	if msg := validatePassword(next); msg != "" {
		verrs["new_password"] = msg
	}
	if next != confirm {
		verrs["confirm_password"] = "Passwords do not match."
	}
	if len(verrs) > 0 {
		return &ValidationError{Fields: verrs}
	}
	_, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.q.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
		PasswordHash: string(hash),
		ID:           userID,
	}); err != nil {
		return err
	}
	return s.q.DeleteUserSessions(ctx, userID)
}

// validateProfile checks email/name/role and enforces email uniqueness against
// any user other than `selfID` (pass 0 on Create).
func (s *UserAdminService) validateProfile(ctx context.Context, in UserInput, selfID int64) (string, string, repository.UsersRole, map[string]string) {
	verrs := map[string]string{}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	name := strings.TrimSpace(in.Name)
	roleStr := strings.TrimSpace(in.Role)

	if email == "" {
		verrs["email"] = "Email is required."
	} else if _, err := mail.ParseAddress(email); err != nil {
		verrs["email"] = "Enter a valid email address."
	} else if len(email) > 255 {
		verrs["email"] = "Email must be 255 characters or fewer."
	} else {
		existing, err := s.q.GetUserByEmail(ctx, email)
		if err == nil && existing.ID != selfID {
			verrs["email"] = "Email already in use."
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			verrs["email"] = "Could not check email uniqueness."
		}
	}

	if name == "" {
		verrs["name"] = "Name is required."
	} else if len(name) > 255 {
		verrs["name"] = "Name must be 255 characters or fewer."
	}

	role := repository.UsersRole(roleStr)
	switch role {
	case repository.UsersRoleAdmin, repository.UsersRoleUser:
	default:
		verrs["role"] = "Choose a role."
	}

	return email, name, role, verrs
}

func validatePassword(pw string) string {
	if pw == "" {
		return "Password is required."
	}
	if len(pw) < MinPasswordLength {
		return "Password must be at least 8 characters."
	}
	return ""
}
