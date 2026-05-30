package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

type SettingsAppHandler struct {
	auth     *services.AuthService
	settings *services.AppSettingsService
	tmpl     *Templates
	errs     *ErrorPages
	logger   *slog.Logger
}

func NewSettingsAppHandler(auth *services.AuthService, settings *services.AppSettingsService, tmpl *Templates, errs *ErrorPages, logger *slog.Logger) *SettingsAppHandler {
	return &SettingsAppHandler{auth: auth, settings: settings, tmpl: tmpl, errs: errs, logger: logger}
}

type appSettingsWeekdayOption struct {
	Value    string
	Label    string
	Selected bool
}

type appSettingsHistoryOption struct {
	Value    string
	Label    string
	Selected bool
}

type appSettingsView struct {
	BaseView
	FormAction     string
	Errors         map[string]string
	WeekStartDay   string
	HistoryWeeks   string
	WeekdayOptions []appSettingsWeekdayOption
	HistoryOptions []appSettingsHistoryOption
}

func (h *SettingsAppHandler) Show(w http.ResponseWriter, r *http.Request) {
	current, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	settings, err := h.settings.Get(r.Context())
	if err != nil {
		h.errs.ServerError(w, r, err)
		return
	}
	view := h.view(r, current, token, settings, nil)
	view.FlashNotice = popFlash(w, r)
	if err := h.tmpl.Render(w, "settings/app.html", view); err != nil {
		h.errs.ServerError(w, r, err)
	}
}

func (h *SettingsAppHandler) Update(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	current, _ := apmw.UserFromContext(r.Context())
	sid := apmw.SessionIDFromContext(r.Context())
	token := h.auth.CSRFToken(sid)

	input := services.AppSettingsInput{
		WeekStartDay: r.PostFormValue("week_start_day"),
		HistoryWeeks: r.PostFormValue("history_weeks"),
	}
	settings, parseErr := services.ParseAppSettingsInput(input)
	if parseErr == nil {
		parseErr = h.settings.Update(r.Context(), input)
	}
	if parseErr != nil {
		var ve *services.ValidationError
		if errors.As(parseErr, &ve) {
			fallback := services.DefaultAppSettings()
			fallback.WeekStartDay = parseWeekdayValue(input.WeekStartDay, fallback.WeekStartDay)
			fallback.HistoryWeeks = parseIntValue(input.HistoryWeeks, fallback.HistoryWeeks)
			w.WriteHeader(http.StatusUnprocessableEntity)
			if err := h.tmpl.Render(w, "settings/app.html", h.view(r, current, token, fallback, ve.Fields)); err != nil {
				h.errs.ServerError(w, r, err)
			}
			return
		}
		h.errs.ServerError(w, r, parseErr)
		return
	}

	h.logger.Info("app settings updated", "by", current.ID, "week_start_day", settings.WeekStartDay, "history_weeks", settings.HistoryWeeks)
	setFlash(w, "Settings updated.")
	http.Redirect(w, r, "/settings/app", http.StatusSeeOther)
}

func (h *SettingsAppHandler) view(r *http.Request, current repository.User, token string, settings services.AppSettings, errs map[string]string) appSettingsView {
	return appSettingsView{
		BaseView:       NewBaseViewForRequest(r, current, token, "App Settings — Settings"),
		FormAction:     "/settings/app",
		Errors:         errs,
		WeekStartDay:   strconv.Itoa(int(settings.WeekStartDay)),
		HistoryWeeks:   strconv.Itoa(settings.HistoryWeeks),
		WeekdayOptions: weekdayOptions(settings.WeekStartDay),
		HistoryOptions: historyOptions(settings.HistoryWeeks),
	}
}

func weekdayOptions(selected time.Weekday) []appSettingsWeekdayOption {
	options := make([]appSettingsWeekdayOption, 0, 7)
	base := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		day := time.Weekday(i)
		options = append(options, appSettingsWeekdayOption{
			Value:    strconv.Itoa(i),
			Label:    base.AddDate(0, 0, i).Format("Monday"),
			Selected: day == selected,
		})
	}
	return options
}

func historyOptions(selected int) []appSettingsHistoryOption {
	options := make([]appSettingsHistoryOption, 0, services.MaxHistoryWeeks)
	for i := services.MinHistoryWeeks; i <= services.MaxHistoryWeeks; i++ {
		label := strconv.Itoa(i) + " week"
		if i != 1 {
			label += "s"
		}
		options = append(options, appSettingsHistoryOption{
			Value:    strconv.Itoa(i),
			Label:    label,
			Selected: i == selected,
		})
	}
	return options
}

func parseWeekdayValue(raw string, fallback time.Weekday) time.Weekday {
	n, err := strconv.Atoi(raw)
	if err != nil || n < int(time.Sunday) || n > int(time.Saturday) {
		return fallback
	}
	return time.Weekday(n)
}

func parseIntValue(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
