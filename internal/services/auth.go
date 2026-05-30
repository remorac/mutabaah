package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/remorac/mutabaah/internal/repository"
)

// ErrInvalidCredentials is returned when login fails for either an unknown
// email or a wrong password. The caller MUST NOT distinguish between these
// to avoid user enumeration.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrSessionNotFound is returned when a session token is missing or expired.
var ErrSessionNotFound = errors.New("session not found")

// ErrImpersonationForbidden is returned when a non-admin attempts to
// impersonate another user.
var ErrImpersonationForbidden = errors.New("impersonation forbidden")

// ErrSelfImpersonation is returned when an admin tries to impersonate themself.
var ErrSelfImpersonation = errors.New("cannot impersonate yourself")

// ErrNotImpersonating is returned when a stop request is made from a regular
// session.
var ErrNotImpersonating = errors.New("not impersonating")

// ErrImpersonatorInvalid is returned when the original admin no longer exists
// or no longer has the admin role. The impersonated session has already been
// destroyed before this error is returned.
var ErrImpersonatorInvalid = errors.New("impersonator invalid")

// AuthService handles password verification, session lifecycle, and CSRF
// token derivation. CSRF tokens are derived from the session ID via HMAC so
// they need no separate storage and naturally rotate when a session ends.
type AuthService struct {
	q               authStore
	secret          []byte
	sessionLifetime time.Duration
}

type authStore interface {
	CreateSession(ctx context.Context, arg repository.CreateSessionParams) error
	DeleteSession(ctx context.Context, id string) error
	GetSession(ctx context.Context, id string) (repository.GetSessionRow, error)
	GetUserByEmail(ctx context.Context, email string) (repository.User, error)
	GetUserByID(ctx context.Context, id int64) (repository.User, error)
}

// SessionInfo is the resolved state for a session token.
type SessionInfo struct {
	User          repository.User
	Impersonator  repository.User
	Impersonating bool
}

// SessionLifetime is the default session lifetime when no override is given.
func NewAuthService(q authStore, secret string, lifetimeHours int) *AuthService {
	if lifetimeHours <= 0 {
		lifetimeHours = 24 * 14
	}
	return &AuthService{
		q:               q,
		secret:          []byte(secret),
		sessionLifetime: time.Duration(lifetimeHours) * time.Hour,
	}
}

// Login verifies email+password and creates a new session row, returning the
// opaque session token to set as a cookie.
func (s *AuthService) Login(ctx context.Context, email, password string) (token string, user repository.User, err error) {
	user, err = s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", repository.User{}, ErrInvalidCredentials
		}
		return "", repository.User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return "", repository.User{}, ErrInvalidCredentials
	}

	token, err = randomToken(32)
	if err != nil {
		return "", repository.User{}, err
	}
	expires := time.Now().Add(s.sessionLifetime)
	if err := s.q.CreateSession(ctx, repository.CreateSessionParams{
		ID:                 token,
		UserID:             user.ID,
		ImpersonatorUserID: sql.NullInt64{},
		ExpiresAt:          expires,
	}); err != nil {
		return "", repository.User{}, err
	}
	return token, user, nil
}

// Logout destroys the supplied session, ignoring not-found errors.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, token)
}

// LookupSession resolves a session token to its user. Returns ErrSessionNotFound
// if the token is missing, expired, or the user no longer exists.
func (s *AuthService) LookupSession(ctx context.Context, token string) (repository.User, error) {
	info, err := s.LookupSessionInfo(ctx, token)
	return info.User, err
}

// LookupSessionInfo resolves a session token to its effective user plus the
// original admin when the session is impersonated.
func (s *AuthService) LookupSessionInfo(ctx context.Context, token string) (SessionInfo, error) {
	if token == "" {
		return SessionInfo{}, ErrSessionNotFound
	}
	sess, err := s.q.GetSession(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionInfo{}, ErrSessionNotFound
		}
		return SessionInfo{}, err
	}
	user, err := s.q.GetUserByID(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionInfo{}, ErrSessionNotFound
		}
		return SessionInfo{}, err
	}
	info := SessionInfo{User: user}
	if sess.ImpersonatorUserID.Valid {
		impersonator, err := s.q.GetUserByID(ctx, sess.ImpersonatorUserID.Int64)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return SessionInfo{}, ErrSessionNotFound
			}
			return SessionInfo{}, err
		}
		info.Impersonator = impersonator
		info.Impersonating = true
	}
	return info, nil
}

// StartImpersonation rotates the current admin session into a session for the
// target user. The returned token must replace the browser's session cookie.
func (s *AuthService) StartImpersonation(ctx context.Context, currentSessionID string, admin repository.User, targetUserID int64) (string, repository.User, error) {
	if admin.Role != repository.UsersRoleAdmin {
		return "", repository.User{}, ErrImpersonationForbidden
	}
	if admin.ID == targetUserID {
		return "", repository.User{}, ErrSelfImpersonation
	}
	target, err := s.q.GetUserByID(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", repository.User{}, ErrUserNotFound
		}
		return "", repository.User{}, err
	}
	token, err := randomToken(32)
	if err != nil {
		return "", repository.User{}, err
	}
	if err := s.q.CreateSession(ctx, repository.CreateSessionParams{
		ID:                 token,
		UserID:             target.ID,
		ImpersonatorUserID: sql.NullInt64{Int64: admin.ID, Valid: true},
		ExpiresAt:          time.Now().Add(s.sessionLifetime),
	}); err != nil {
		return "", repository.User{}, err
	}
	if err := s.Logout(ctx, currentSessionID); err != nil {
		return "", repository.User{}, err
	}
	return token, target, nil
}

// StopImpersonation rotates an impersonated session back to the original admin.
// If the original admin is gone or no longer admin, the current session is
// destroyed and ErrImpersonatorInvalid is returned.
func (s *AuthService) StopImpersonation(ctx context.Context, currentSessionID string) (string, repository.User, error) {
	if currentSessionID == "" {
		return "", repository.User{}, ErrSessionNotFound
	}
	sess, err := s.q.GetSession(ctx, currentSessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", repository.User{}, ErrSessionNotFound
		}
		return "", repository.User{}, err
	}
	if !sess.ImpersonatorUserID.Valid {
		return "", repository.User{}, ErrNotImpersonating
	}
	admin, err := s.q.GetUserByID(ctx, sess.ImpersonatorUserID.Int64)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = s.Logout(ctx, currentSessionID)
			return "", repository.User{}, ErrImpersonatorInvalid
		}
		return "", repository.User{}, err
	}
	if admin.Role != repository.UsersRoleAdmin {
		_ = s.Logout(ctx, currentSessionID)
		return "", repository.User{}, ErrImpersonatorInvalid
	}
	token, err := randomToken(32)
	if err != nil {
		return "", repository.User{}, err
	}
	if err := s.q.CreateSession(ctx, repository.CreateSessionParams{
		ID:                 token,
		UserID:             admin.ID,
		ImpersonatorUserID: sql.NullInt64{},
		ExpiresAt:          time.Now().Add(s.sessionLifetime),
	}); err != nil {
		return "", repository.User{}, err
	}
	if err := s.Logout(ctx, currentSessionID); err != nil {
		return "", repository.User{}, err
	}
	return token, admin, nil
}

// CSRFToken returns a token bound to the given session ID. Deterministic so
// it can be regenerated on each request without storage; invalidates when
// the session rotates.
func (s *AuthService) CSRFToken(sessionID string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte("csrf:"))
	mac.Write([]byte(sessionID))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyCSRF performs a constant-time compare between the expected token for
// the session and the supplied token from the request.
func (s *AuthService) VerifyCSRF(sessionID, supplied string) bool {
	if sessionID == "" || supplied == "" {
		return false
	}
	expected := s.CSRFToken(sessionID)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) == 1
}

// SessionLifetime exposes the configured cookie lifetime for handlers.
func (s *AuthService) SessionLifetime() time.Duration {
	return s.sessionLifetime
}

func randomToken(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
