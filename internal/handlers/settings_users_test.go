package handlers

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

type fakeSettingsUsers struct {
	user           repository.User
	getErr         error
	resetUserID    int64
	resetDates     []time.Time
	resetAllowed   []time.Time
	resetWasCalled bool
}

func (s *fakeSettingsUsers) List(ctx context.Context, limit, offset int32) ([]repository.User, int64, error) {
	return nil, 0, nil
}

func (s *fakeSettingsUsers) Get(ctx context.Context, id int64) (repository.User, error) {
	if s.getErr != nil {
		return repository.User{}, s.getErr
	}
	return s.user, nil
}

func (s *fakeSettingsUsers) Create(ctx context.Context, in services.UserInput) (int64, error) {
	return 0, nil
}

func (s *fakeSettingsUsers) Update(ctx context.Context, id int64, in services.UserInput) error {
	return nil
}

func (s *fakeSettingsUsers) Delete(ctx context.Context, id int64) error {
	return nil
}

func (s *fakeSettingsUsers) ResetCompletions(ctx context.Context, userID int64, dates, allowedDates []time.Time) error {
	s.resetWasCalled = true
	s.resetUserID = userID
	s.resetDates = append([]time.Time(nil), dates...)
	s.resetAllowed = append([]time.Time(nil), allowedDates...)
	return nil
}

type fakeSettingsMenses struct {
	periods []repository.MensesPeriod
}

func (s fakeSettingsMenses) List(ctx context.Context, userID int64) ([]repository.MensesPeriod, error) {
	return s.periods, nil
}

type fakeSettingsGetter struct {
	settings services.AppSettings
}

func (s fakeSettingsGetter) Get(ctx context.Context) (services.AppSettings, error) {
	return s.settings, nil
}

func TestAvatarURLUsesThumbnailPath(t *testing.T) {
	user := repository.User{
		AvatarPath: sql.NullString{String: "42_profile.png", Valid: true},
	}

	got := avatarURL(user)

	if got != "/static/avatars/thumb_42_profile.jpg" {
		t.Fatalf("avatarURL() = %q, want thumbnail URL", got)
	}
}

func TestUsersIndexRendersAvatarAndDetailLinksWithoutRowActions(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	view := userListView{
		BaseView: BaseView{Title: "Users", CSRFToken: "csrf-token"},
		Rows: []userListRow{{
			ID:         2,
			Email:      "fatimah@example.com",
			Name:       "Fatimah",
			Role:       string(repository.UsersRoleUser),
			CreatedAt:  "2026-05-30",
			DetailURL:  "/settings/users/2",
			AvatarPath: "/static/avatars/thumb_2_avatar.jpg",
		}},
		Page:       1,
		TotalPages: 1,
		Total:      1,
	}
	rec := httptest.NewRecorder()

	if err := tmpl.Render(rec, "settings/users/index.html", view); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`href="/settings/users/2"`,
		`src="/static/avatars/thumb_2_avatar.jpg"`,
		"fatimah@example.com",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("users index missing %q: %s", want, body)
		}
	}
	for _, unwanted := range []string{
		`/settings/users/2/edit`,
		`/settings/users/2/impersonate`,
		`/settings/users/2/reset-data`,
		`user-reset-modal-2`,
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("users index still contains row action %q: %s", unwanted, body)
		}
	}
}

func TestUserDetailRendersDataActionsAndReadOnlyPeriods(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	view := userDetailView{
		BaseView:       BaseView{Title: "Fatimah", CSRFToken: "csrf-token"},
		ID:             2,
		Email:          "fatimah@example.com",
		Name:           "Fatimah",
		Role:           string(repository.UsersRoleUser),
		CreatedAt:      "2026-05-30",
		AvatarPath:     "/static/avatars/thumb_2_avatar.jpg",
		EditURL:        "/settings/users/2/edit",
		ResetURL:       "/settings/users/2/reset-data",
		ImpersonateURL: "/settings/users/2/impersonate",
		CanImpersonate: true,
		ResetDates: []resetDateOption{{
			Value: "2026-05-30",
			Label: "Sat, May 30",
		}},
		Periods: []periodRow{{
			ID:        7,
			StartDate: "2026-05-01",
			EndDate:   "2026-05-06",
		}, {
			ID:        8,
			StartDate: "2026-05-28",
			Ongoing:   true,
		}},
	}
	rec := httptest.NewRecorder()

	if err := tmpl.Render(rec, "settings/users/show.html", view); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"Fatimah",
		"fatimah@example.com",
		`src="/static/avatars/thumb_2_avatar.jpg"`,
		`href="/settings/users/2/edit"`,
		`action="/settings/users/2/reset-data"`,
		`action="/settings/users/2/impersonate"`,
		"2026-05-01",
		"2026-05-06",
		"2026-05-28",
		"ongoing",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("user detail missing %q: %s", want, body)
		}
	}
	for _, unwanted := range []string{
		`action="/settings/periods"`,
		`/settings/periods/7/delete`,
		`hx-post="/settings/periods`,
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("user detail has editable period UI %q: %s", unwanted, body)
		}
	}
}

func TestUserDetailHandlerReturnsNotFoundForMissingUser(t *testing.T) {
	handler := newSettingsUsersTestHandler(t, &fakeSettingsUsers{getErr: services.ErrUserNotFound}, fakeSettingsMenses{}, fakeSettingsGetter{
		settings: services.DefaultAppSettings(),
	})
	rec := httptest.NewRecorder()
	req := newSettingsUserRequest(http.MethodGet, "/settings/users/99", "99", nil)

	handler.Show(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Page not found") {
		t.Fatalf("not found body missing heading: %s", rec.Body.String())
	}
}

func TestResetDataRedirectsBackToUserDetail(t *testing.T) {
	users := &fakeSettingsUsers{}
	handler := newSettingsUsersTestHandler(t, users, fakeSettingsMenses{}, fakeSettingsGetter{
		settings: services.DefaultAppSettings(),
	})
	form := url.Values{"dates": {"2026-05-30"}}
	rec := httptest.NewRecorder()
	req := newSettingsUserRequest(http.MethodPost, "/settings/users/2/reset-data", "2", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	handler.ResetData(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/settings/users/2" {
		t.Fatalf("Location = %q, want detail page", got)
	}
	if !users.resetWasCalled || users.resetUserID != 2 || len(users.resetDates) != 1 || len(users.resetAllowed) == 0 {
		t.Fatalf("reset call = called:%t user:%d dates:%d allowed:%d", users.resetWasCalled, users.resetUserID, len(users.resetDates), len(users.resetAllowed))
	}
}

func newSettingsUsersTestHandler(t *testing.T, users settingsUserService, menses settingsMensesService, settings settingsGetter) *SettingsUsersHandler {
	t.Helper()
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	return &SettingsUsersHandler{
		auth:     services.NewAuthService(nil, "test-secret-long-enough", 1),
		users:    users,
		menses:   menses,
		settings: settings,
		tmpl:     tmpl,
		errs:     NewErrorPages(tmpl, nilLogger()),
		logger:   nilLogger(),
	}
}

func newSettingsUserRequest(method, target, id string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}
