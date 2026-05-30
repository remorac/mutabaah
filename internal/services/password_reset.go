package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/remorac/mutabaah/internal/repository"
)

var ErrInvalidResetToken = errors.New("invalid password reset token")

const PasswordResetLifetime = time.Hour

type passwordResetStore interface {
	CreatePasswordResetToken(ctx context.Context, arg repository.CreatePasswordResetTokenParams) error
	GetUserByEmail(ctx context.Context, email string) (repository.User, error)
	GetValidPasswordResetToken(ctx context.Context, tokenHash string) (repository.PasswordResetToken, error)
	MarkPasswordResetTokenUsed(ctx context.Context, tokenHash string) (int64, error)
	UpdateUserPassword(ctx context.Context, arg repository.UpdateUserPasswordParams) error
	DeleteUserSessions(ctx context.Context, arg repository.DeleteUserSessionsParams) error
	DeleteExpiredPasswordResetTokens(ctx context.Context) error
}

type PasswordResetService struct {
	q       passwordResetStore
	mailer  Mailer
	baseURL string
	now     func() time.Time
}

func NewPasswordResetService(q passwordResetStore, mailer Mailer, baseURL string) *PasswordResetService {
	return &PasswordResetService{
		q:       q,
		mailer:  mailer,
		baseURL: strings.TrimRight(baseURL, "/"),
		now:     time.Now,
	}
}

func (s *PasswordResetService) RequestReset(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	hash := resetTokenHash(token)
	if err := s.q.CreatePasswordResetToken(ctx, repository.CreatePasswordResetTokenParams{
		TokenHash: hash,
		UserID:    user.ID,
		ExpiresAt: s.now().Add(PasswordResetLifetime),
	}); err != nil {
		return err
	}
	return s.mailer.SendPasswordReset(ctx, user.Email, s.resetURL(token))
}

func (s *PasswordResetService) ValidateToken(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrInvalidResetToken
	}
	_, err := s.q.GetValidPasswordResetToken(ctx, resetTokenHash(token))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidResetToken
		}
		return err
	}
	return nil
}

func (s *PasswordResetService) ResetPassword(ctx context.Context, token, next, confirm string) error {
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

	hash := resetTokenHash(token)
	reset, err := s.q.GetValidPasswordResetToken(ctx, hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidResetToken
		}
		return err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	affected, err := s.q.MarkPasswordResetTokenUsed(ctx, hash)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrInvalidResetToken
	}
	if err := s.q.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
		PasswordHash: string(passwordHash),
		ID:           reset.UserID,
	}); err != nil {
		return err
	}
	if err := s.q.DeleteUserSessions(ctx, repository.DeleteUserSessionsParams{
		UserID:             reset.UserID,
		ImpersonatorUserID: sql.NullInt64{Int64: reset.UserID, Valid: true},
	}); err != nil {
		return err
	}
	return s.q.DeleteExpiredPasswordResetTokens(ctx)
}

func (s *PasswordResetService) resetURL(token string) string {
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return s.baseURL + "/reset-password?token=" + url.QueryEscape(token)
	}
	u.Path = "/reset-password"
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

func resetTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
