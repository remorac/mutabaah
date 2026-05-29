package services

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/remorac/mutabaah/internal/repository"
)

const (
	DefaultWeekStartDay = time.Saturday
	DefaultHistoryWeeks = 1
	MinHistoryWeeks     = 1
	MaxHistoryWeeks     = 4
)

type AppSettings struct {
	WeekStartDay time.Weekday
	HistoryWeeks int
}

type AppSettingsInput struct {
	WeekStartDay string
	HistoryWeeks string
}

type AppSettingsService struct {
	q *repository.Queries
}

func NewAppSettingsService(q *repository.Queries) *AppSettingsService {
	return &AppSettingsService{q: q}
}

func DefaultAppSettings() AppSettings {
	return AppSettings{
		WeekStartDay: DefaultWeekStartDay,
		HistoryWeeks: DefaultHistoryWeeks,
	}
}

func (s *AppSettingsService) Get(ctx context.Context) (AppSettings, error) {
	row, err := s.q.GetAppSettings(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultAppSettings(), nil
		}
		return AppSettings{}, err
	}
	settings := AppSettings{
		WeekStartDay: time.Weekday(row.WeekStartDay),
		HistoryWeeks: int(row.HistoryWeeks),
	}
	if settings.WeekStartDay < time.Sunday || settings.WeekStartDay > time.Saturday {
		settings.WeekStartDay = DefaultWeekStartDay
	}
	if settings.HistoryWeeks < MinHistoryWeeks || settings.HistoryWeeks > MaxHistoryWeeks {
		settings.HistoryWeeks = DefaultHistoryWeeks
	}
	return settings, nil
}

func (s *AppSettingsService) Update(ctx context.Context, in AppSettingsInput) error {
	settings, err := ParseAppSettingsInput(in)
	if err != nil {
		return err
	}
	return s.q.UpsertAppSettings(ctx, repository.UpsertAppSettingsParams{
		WeekStartDay: int8(settings.WeekStartDay),
		HistoryWeeks: int8(settings.HistoryWeeks),
	})
}

func ParseAppSettingsInput(in AppSettingsInput) (AppSettings, error) {
	verrs := map[string]string{}

	weekStartRaw, err := strconv.Atoi(in.WeekStartDay)
	if err != nil || weekStartRaw < int(time.Sunday) || weekStartRaw > int(time.Saturday) {
		verrs["week_start_day"] = "Choose a valid week start day."
	}

	historyWeeks, err := strconv.Atoi(in.HistoryWeeks)
	if err != nil || historyWeeks < MinHistoryWeeks || historyWeeks > MaxHistoryWeeks {
		verrs["history_weeks"] = "Choose a duration from 1 to 4 weeks."
	}

	if len(verrs) > 0 {
		return AppSettings{}, &ValidationError{Fields: verrs}
	}
	return AppSettings{
		WeekStartDay: time.Weekday(weekStartRaw),
		HistoryWeeks: historyWeeks,
	}, nil
}
