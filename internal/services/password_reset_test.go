package services

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/remorac/mutabaah/internal/repository"
)

type fakeResetStore struct {
	users        map[string]repository.User
	tokens       map[string]repository.PasswordResetToken
	passwordHash string
	deletedFor   int64
	markedHash   string
	deletedOld   bool
}

func newFakeResetStore() *fakeResetStore {
	return &fakeResetStore{
		users:  map[string]repository.User{},
		tokens: map[string]repository.PasswordResetToken{},
	}
}

func (s *fakeResetStore) CreatePasswordResetToken(ctx context.Context, arg repository.CreatePasswordResetTokenParams) error {
	s.tokens[arg.TokenHash] = repository.PasswordResetToken{
		TokenHash: arg.TokenHash,
		UserID:    arg.UserID,
		ExpiresAt: arg.ExpiresAt,
		CreatedAt: time.Now(),
	}
	return nil
}

func (s *fakeResetStore) GetUserByEmail(ctx context.Context, email string) (repository.User, error) {
	u, ok := s.users[email]
	if !ok {
		return repository.User{}, sql.ErrNoRows
	}
	return u, nil
}

func (s *fakeResetStore) GetValidPasswordResetToken(ctx context.Context, tokenHash string) (repository.PasswordResetToken, error) {
	tok, ok := s.tokens[tokenHash]
	if !ok || tok.UsedAt.Valid || !tok.ExpiresAt.After(time.Now().Add(-time.Second)) {
		return repository.PasswordResetToken{}, sql.ErrNoRows
	}
	return tok, nil
}

func (s *fakeResetStore) MarkPasswordResetTokenUsed(ctx context.Context, tokenHash string) (int64, error) {
	tok := s.tokens[tokenHash]
	if tok.UsedAt.Valid {
		return 0, nil
	}
	tok.UsedAt = sql.NullTime{Time: time.Now(), Valid: true}
	s.tokens[tokenHash] = tok
	s.markedHash = tokenHash
	return 1, nil
}

func (s *fakeResetStore) UpdateUserPassword(ctx context.Context, arg repository.UpdateUserPasswordParams) error {
	s.passwordHash = arg.PasswordHash
	return nil
}

func (s *fakeResetStore) DeleteUserSessions(ctx context.Context, arg repository.DeleteUserSessionsParams) error {
	s.deletedFor = arg.UserID
	return nil
}

func (s *fakeResetStore) DeleteExpiredPasswordResetTokens(ctx context.Context) error {
	s.deletedOld = true
	return nil
}

type fakeMailer struct {
	to  string
	url string
	err error
}

func (m *fakeMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	m.to = to
	m.url = resetURL
	return m.err
}

func TestRequestResetCreatesHashedTokenAndEmailsLink(t *testing.T) {
	store := newFakeResetStore()
	store.users["a@example.com"] = repository.User{ID: 42, Email: "a@example.com", IsActive: true}
	mailer := &fakeMailer{}
	svc := NewPasswordResetService(store, mailer, "https://tracker.example.com")

	if err := svc.RequestReset(context.Background(), " A@example.com "); err != nil {
		t.Fatalf("RequestReset() error = %v", err)
	}
	if mailer.to != "a@example.com" {
		t.Fatalf("mailer.to = %q, want a@example.com", mailer.to)
	}
	parsed, err := url.Parse(mailer.url)
	if err != nil {
		t.Fatalf("reset url parse error = %v", err)
	}
	token := parsed.Query().Get("token")
	if token == "" {
		t.Fatalf("reset URL missing token: %s", mailer.url)
	}
	if _, ok := store.tokens[token]; ok {
		t.Fatalf("raw token was stored")
	}
	if _, ok := store.tokens[resetTokenHash(token)]; !ok {
		t.Fatalf("hashed token was not stored")
	}
}

func TestRequestResetUnknownEmailIsGeneric(t *testing.T) {
	store := newFakeResetStore()
	mailer := &fakeMailer{}
	svc := NewPasswordResetService(store, mailer, "https://tracker.example.com")

	if err := svc.RequestReset(context.Background(), "missing@example.com"); err != nil {
		t.Fatalf("RequestReset() error = %v", err)
	}
	if mailer.url != "" {
		t.Fatalf("mailer sent reset for unknown user: %s", mailer.url)
	}
}

func TestRequestResetInactiveUserIsGeneric(t *testing.T) {
	store := newFakeResetStore()
	store.users["inactive@example.com"] = repository.User{ID: 42, Email: "inactive@example.com", IsActive: false}
	mailer := &fakeMailer{}
	svc := NewPasswordResetService(store, mailer, "https://tracker.example.com")

	if err := svc.RequestReset(context.Background(), "inactive@example.com"); err != nil {
		t.Fatalf("RequestReset() error = %v", err)
	}
	if mailer.url != "" {
		t.Fatalf("mailer sent reset for inactive user: %s", mailer.url)
	}
	if len(store.tokens) != 0 {
		t.Fatalf("reset token created for inactive user: %#v", store.tokens)
	}
}

func TestRequestResetReturnsMailerErrors(t *testing.T) {
	store := newFakeResetStore()
	store.users["a@example.com"] = repository.User{ID: 42, Email: "a@example.com", IsActive: true}
	mailer := &fakeMailer{err: errors.New("smtp down")}
	svc := NewPasswordResetService(store, mailer, "https://tracker.example.com")

	if err := svc.RequestReset(context.Background(), "a@example.com"); err == nil || !strings.Contains(err.Error(), "smtp down") {
		t.Fatalf("RequestReset() error = %v, want smtp down", err)
	}
}

func TestResetPasswordValidTokenUpdatesPasswordAndRevokesSessions(t *testing.T) {
	store := newFakeResetStore()
	token := "reset-token"
	hash := resetTokenHash(token)
	store.tokens[hash] = repository.PasswordResetToken{TokenHash: hash, UserID: 7, ExpiresAt: time.Now().Add(time.Hour)}
	svc := NewPasswordResetService(store, &fakeMailer{}, "https://tracker.example.com")

	if err := svc.ResetPassword(context.Background(), token, "new-password", "new-password"); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(store.passwordHash), []byte("new-password")) != nil {
		t.Fatalf("stored password hash does not match new password")
	}
	if store.markedHash != hash {
		t.Fatalf("markedHash = %q, want %q", store.markedHash, hash)
	}
	if store.deletedFor != 7 {
		t.Fatalf("deleted sessions for user %d, want 7", store.deletedFor)
	}
	if !store.deletedOld {
		t.Fatalf("expired token cleanup was not called")
	}
}

func TestResetPasswordRejectsInvalidAndUsedTokens(t *testing.T) {
	store := newFakeResetStore()
	svc := NewPasswordResetService(store, &fakeMailer{}, "https://tracker.example.com")
	if err := svc.ResetPassword(context.Background(), "missing", "new-password", "new-password"); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("missing token error = %v, want ErrInvalidResetToken", err)
	}

	token := "used-token"
	hash := resetTokenHash(token)
	store.tokens[hash] = repository.PasswordResetToken{
		TokenHash: hash,
		UserID:    7,
		ExpiresAt: time.Now().Add(time.Hour),
		UsedAt:    sql.NullTime{Time: time.Now(), Valid: true},
	}
	if err := svc.ResetPassword(context.Background(), token, "new-password", "new-password"); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("used token error = %v, want ErrInvalidResetToken", err)
	}
}

func TestResetPasswordValidation(t *testing.T) {
	store := newFakeResetStore()
	token := "reset-token"
	hash := resetTokenHash(token)
	store.tokens[hash] = repository.PasswordResetToken{TokenHash: hash, UserID: 7, ExpiresAt: time.Now().Add(time.Hour)}
	svc := NewPasswordResetService(store, &fakeMailer{}, "https://tracker.example.com")

	err := svc.ResetPassword(context.Background(), token, "short", "different")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("ResetPassword() error = %v, want ValidationError", err)
	}
	if ve.Fields["new_password"] == "" || ve.Fields["confirm_password"] == "" {
		t.Fatalf("validation fields = %#v, want password and confirmation errors", ve.Fields)
	}
}
