package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/remorac/mutabaah/internal/repository"
)

func TestSetFlashCookieAttributes(t *testing.T) {
	rec := httptest.NewRecorder()

	setFlash(rec, "Saved.")

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != flashCookieName {
		t.Fatalf("cookie name = %q, want %q", cookie.Name, flashCookieName)
	}
	if cookie.Value == "" {
		t.Fatalf("cookie value is empty")
	}
	if cookie.Path != "/" {
		t.Fatalf("cookie path = %q, want /", cookie.Path)
	}
	if cookie.MaxAge != 60 {
		t.Fatalf("cookie MaxAge = %d, want 60", cookie.MaxAge)
	}
	if !cookie.HttpOnly {
		t.Fatalf("cookie HttpOnly = false, want true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite = %v, want Lax", cookie.SameSite)
	}
}

func TestPopFlashReturnsMessageAndClearsCookie(t *testing.T) {
	setRec := httptest.NewRecorder()
	setFlash(setRec, "Task completions reset.")
	cookie := setRec.Result().Cookies()[0]

	req := httptest.NewRequest(http.MethodGet, "/settings/users", nil)
	req.AddCookie(cookie)
	popRec := httptest.NewRecorder()

	got := popFlash(popRec, req)

	if got != "Task completions reset." {
		t.Fatalf("popFlash() = %q, want reset message", got)
	}
	cookies := popRec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("clear cookies = %d, want 1", len(cookies))
	}
	clear := cookies[0]
	if clear.Name != flashCookieName || clear.MaxAge != -1 || clear.Value != "" {
		t.Fatalf("clear cookie = %#v, want deleted %s cookie", clear, flashCookieName)
	}
}

func TestPopFlashWithoutCookieReturnsEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/settings/app", nil)

	if got := popFlash(rec, req); got != "" {
		t.Fatalf("popFlash() = %q, want empty", got)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("popFlash without cookie wrote clear cookie")
	}
}

func TestLayoutRendersFlashToast(t *testing.T) {
	tmpl, err := LoadTemplates("../../web/templates")
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}
	view := appSettingsView{
		BaseView: NewBaseView(repository.User{
			ID:   1,
			Name: "Admin",
			Role: repository.UsersRoleAdmin,
		}, "csrf-token", "App Settings — Settings"),
		FormAction:     "/settings/app",
		WeekdayOptions: weekdayOptions(0),
		HistoryOptions: historyOptions(1),
	}
	view.FlashNotice = "Settings updated."
	rec := httptest.NewRecorder()

	if err := tmpl.Render(rec, "settings/app.html", view); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `class="toast toast-end toast-bottom z-50"`) {
		t.Fatalf("toast container missing: %s", body)
	}
	if !strings.Contains(body, "Settings updated.") {
		t.Fatalf("toast message missing: %s", body)
	}
}
