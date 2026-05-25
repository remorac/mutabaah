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

// AuthService handles password verification, session lifecycle, and CSRF
// token derivation. CSRF tokens are derived from the session ID via HMAC so
// they need no separate storage and naturally rotate when a session ends.
type AuthService struct {
	q               *repository.Queries
	secret          []byte
	sessionLifetime time.Duration
}

// SessionLifetime is the default session lifetime when no override is given.
func NewAuthService(q *repository.Queries, secret string, lifetimeHours int) *AuthService {
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
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: expires,
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
	if token == "" {
		return repository.User{}, ErrSessionNotFound
	}
	sess, err := s.q.GetSession(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.User{}, ErrSessionNotFound
		}
		return repository.User{}, err
	}
	user, err := s.q.GetUserByID(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.User{}, ErrSessionNotFound
		}
		return repository.User{}, err
	}
	return user, nil
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
