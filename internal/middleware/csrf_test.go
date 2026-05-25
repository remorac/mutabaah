package middleware

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/remorac/mutabaah/internal/services"
)

func TestCSRFAllowsMultipartFormToken(t *testing.T) {
	auth := services.NewAuthService(nil, "test-secret", 1)
	sessionID := "session-1"
	token := auth.CSRFToken(sessionID)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField(CSRFFieldName, token); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("avatar", "avatar.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("fake image bytes")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	called := false
	handler := CSRF(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if _, _, err := r.FormFile("avatar"); err != nil {
			t.Fatalf("multipart form was not available to handler: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/settings/profile/picture", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), ctxKeySessionID, sessionID))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestCSRFAllowsURLEncodedFormToken(t *testing.T) {
	auth := services.NewAuthService(nil, "test-secret", 1)
	sessionID := "session-1"

	form := url.Values{}
	form.Set(CSRFFieldName, auth.CSRFToken(sessionID))

	handler := CSRF(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/settings/profile", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(context.WithValue(req.Context(), ctxKeySessionID, sessionID))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}
