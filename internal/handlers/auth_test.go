package handlers

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

type authTestStore struct {
	user  repository.User
	token repository.PasswordResetToken
}

func (s *authTestStore) CreatePasswordResetToken(ctx context.Context, arg repository.CreatePasswordResetTokenParams) error {
	s.token = repository.PasswordResetToken{TokenHash: arg.TokenHash, UserID: arg.UserID, ExpiresAt: arg.ExpiresAt}
	return nil
}

func (s *authTestStore) GetUserByEmail(ctx context.Context, email string) (repository.User, error) {
	if s.user.Email == email {
		return s.user, nil
	}
	return repository.User{}, sql.ErrNoRows
}

func (s *authTestStore) GetValidPasswordResetToken(ctx context.Context, tokenHash string) (repository.PasswordResetToken, error) {
	if s.token.TokenHash == tokenHash && s.token.ExpiresAt.After(time.Now()) && !s.token.UsedAt.Valid {
		return s.token, nil
	}
	return repository.PasswordResetToken{}, sql.ErrNoRows
}

func (s *authTestStore) MarkPasswordResetTokenUsed(ctx context.Context, tokenHash string) (int64, error) {
	if s.token.UsedAt.Valid {
		return 0, nil
	}
	s.token.UsedAt = sql.NullTime{Time: time.Now(), Valid: true}
	return 1, nil
}

func (s *authTestStore) UpdateUserPassword(ctx context.Context, arg repository.UpdateUserPasswordParams) error {
	return nil
}

func (s *authTestStore) DeleteUserSessions(ctx context.Context, arg repository.DeleteUserSessionsParams) error {
	return nil
}

func (s *authTestStore) DeleteExpiredPasswordResetTokens(ctx context.Context) error {
	return nil
}

type authTestMailer struct {
	url string
}

func (m *authTestMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	m.url = resetURL
	return nil
}

func newAuthTestHandler(t *testing.T) (*AuthHandler, *authTestStore, *authTestMailer) {
	t.Helper()
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	store := &authTestStore{user: repository.User{ID: 1, Email: "a@example.com"}}
	mailer := &authTestMailer{}
	resets := services.NewPasswordResetService(store, mailer, "https://tracker.example.com")
	return NewAuthHandler(services.NewAuthService(nil, "test-secret-long-enough", 1), resets, tmpl, nilLogger(), false), store, mailer
}

func nilLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestForgotPasswordFormRenders(t *testing.T) {
	handler, _, _ := newAuthTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/forgot-password", nil)

	handler.ForgotPasswordForm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Send reset link") {
		t.Fatalf("forgot password form missing submit text: %s", rec.Body.String())
	}
}

func TestForgotPasswordPostRendersGenericSuccess(t *testing.T) {
	handler, _, mailer := newAuthTestHandler(t)
	form := url.Values{"email": {"a@example.com"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	handler.ForgotPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "If an account exists") {
		t.Fatalf("generic success missing: %s", rec.Body.String())
	}
	if mailer.url == "" {
		t.Fatalf("reset email was not sent")
	}
}

func TestResetPasswordFormValidAndInvalidToken(t *testing.T) {
	handler, store, mailer := newAuthTestHandler(t)
	if err := handler.resets.RequestReset(context.Background(), "a@example.com"); err != nil {
		t.Fatalf("RequestReset() error = %v", err)
	}
	if store.token.TokenHash == "" || mailer.url == "" {
		t.Fatalf("test reset token was not created")
	}
	parsed, _ := url.Parse(mailer.url)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reset-password?token="+url.QueryEscape(parsed.Query().Get("token")), nil)
	handler.ResetPasswordForm(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Update password") {
		t.Fatalf("valid token status/body = %d/%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/reset-password?token=bad", nil)
	handler.ResetPasswordForm(rec, req)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "invalid or has expired") {
		t.Fatalf("invalid token status/body = %d/%s", rec.Code, rec.Body.String())
	}
}

func TestResetPasswordPostValidationAndSuccess(t *testing.T) {
	handler, _, mailer := newAuthTestHandler(t)
	if err := handler.resets.RequestReset(context.Background(), "a@example.com"); err != nil {
		t.Fatalf("RequestReset() error = %v", err)
	}
	parsed, _ := url.Parse(mailer.url)
	token := parsed.Query().Get("token")

	form := url.Values{"token": {token}, "new_password": {"short"}, "confirm_password": {"different"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ResetPassword(rec, req)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Passwords do not match") {
		t.Fatalf("validation status/body = %d/%s", rec.Code, rec.Body.String())
	}

	form = url.Values{"token": {token}, "new_password": {"new-password"}, "confirm_password": {"new-password"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ResetPassword(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("success status/location = %d/%q, want 303 /login", rec.Code, rec.Header().Get("Location"))
	}
}
