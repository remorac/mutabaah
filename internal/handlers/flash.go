package handlers

import (
	"encoding/base64"
	"net/http"
	"time"
)

const flashCookieName = "tracker_flash"

func setFlash(w http.ResponseWriter, message string) {
	if message == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    base64.RawURLEncoding.EncodeToString([]byte(message)),
		Path:     "/",
		MaxAge:   60,
		Expires:  time.Now().Add(time.Minute),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func popFlash(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil || cookie.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	message, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return ""
	}
	return string(message)
}
