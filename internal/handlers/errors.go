package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	apmw "github.com/aldoerianda/tracker/internal/middleware"
)

// ErrorPages renders the styled 404/403/500 pages. Falls back to plain text
// for HTMX/AJAX requests so partial-target swaps don't end up replacing a
// dashboard row with an entire layout.
type ErrorPages struct {
	tmpl   *Templates
	logger *slog.Logger
}

func NewErrorPages(tmpl *Templates, logger *slog.Logger) *ErrorPages {
	return &ErrorPages{tmpl: tmpl, logger: logger}
}

type errorView struct {
	BaseView
	Code    int
	Icon    string
	Tone    string
	Heading string
	Message string
}

// NotFound serves the styled 404 page. Wire as r.NotFound(...).
func (e *ErrorPages) NotFound(w http.ResponseWriter, r *http.Request) {
	e.render(w, r, http.StatusNotFound, "search-x", "warning",
		"Page not found",
		"The page you were looking for doesn't exist or has moved.")
}

// MethodNotAllowed serves a styled 405 page. Wire as r.MethodNotAllowed(...).
func (e *ErrorPages) MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	e.render(w, r, http.StatusMethodNotAllowed, "ban", "warning",
		"Method not allowed",
		"That action isn't permitted on this URL.")
}

// Forbidden serves the styled 403 page.
func (e *ErrorPages) Forbidden(w http.ResponseWriter, r *http.Request) {
	e.render(w, r, http.StatusForbidden, "shield-x", "error",
		"Forbidden",
		"You don't have permission to view this page.")
}

// ServerError serves the styled 500 page, logging the underlying error.
func (e *ErrorPages) ServerError(w http.ResponseWriter, r *http.Request, err error) {
	apmw.RequestLogger(e.logger, r).Error("server error", "err", err, "path", r.URL.Path)
	e.render(w, r, http.StatusInternalServerError, "alert-triangle", "error",
		"Something went wrong",
		"We hit an unexpected problem. The error has been logged; please try again.")
}

func (e *ErrorPages) render(w http.ResponseWriter, r *http.Request, code int, icon, tone, heading, message string) {
	// HTMX / AJAX clients get the short plain-text response; rendering the
	// layout would dump the entire page into a swap target.
	if isPartialRequest(r) {
		http.Error(w, heading, code)
		return
	}
	view := errorView{
		BaseView: BaseView{Title: heading + " — Mutaba'ah Tracker"},
		Code:     code,
		Icon:     icon,
		Tone:     tone,
		Heading:  heading,
		Message:  message,
	}
	w.WriteHeader(code)
	if err := e.tmpl.Render(w, "errors/error.html", view); err != nil {
		// Last-resort fallback if template rendering blows up.
		http.Error(w, heading, code)
	}
}

func isPartialRequest(r *http.Request) bool {
	if r.Header.Get("HX-Request") != "" {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest") {
		return true
	}
	return false
}
