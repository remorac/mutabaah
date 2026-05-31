package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log/slog"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"

	"github.com/remorac/mutabaah/internal/repository"
)

// Seeds an admin user and a starter set of tasks (fard salah ×5, dhikr, weekly
// Surah Al-Kahf, monthly Quran khatam). Re-running is safe: existing users
// (by email) are skipped, and starter tasks already present (by title) are
// not duplicated.
func main() {
	_ = godotenv.Load()

	adminEmail := flag.String("email", "admin@example.com", "admin email")
	adminPassword := flag.String("password", "changeme-admin-12345", "admin password (>= 8 chars recommended)")
	adminName := flag.String("name", "Admin", "admin display name")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dsn := os.Getenv("APP_DATABASE_DSN")
	if dsn == "" {
		logger.Error("APP_DATABASE_DSN is not set")
		os.Exit(1)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		logger.Error("open db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		logger.Error("ping db", "err", err)
		os.Exit(1)
	}

	q := repository.New(db)

	adminID, err := ensureAdmin(ctx, q, *adminEmail, *adminPassword, *adminName)
	if err != nil {
		logger.Error("seed admin", "err", err)
		os.Exit(1)
	}
	logger.Info("admin ready", "id", adminID, "email", *adminEmail)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	starter := []repository.CreateTaskParams{
		{Title: "Subuh", Description: nullString("fard"), Frequency: repository.TasksFrequencyDaily, StartDate: today, Active: true},
		{Title: "Dzuhur", Description: nullString("fard"), Frequency: repository.TasksFrequencyDaily, StartDate: today, Active: true},
		{Title: "Ashar", Description: nullString("fard"), Frequency: repository.TasksFrequencyDaily, StartDate: today, Active: true},
		{Title: "Maghrib", Description: nullString("fard"), Frequency: repository.TasksFrequencyDaily, StartDate: today, Active: true},
		{Title: "Isya", Description: nullString("fard"), Frequency: repository.TasksFrequencyDaily, StartDate: today, Active: true},
		{Title: "Dhikr pagi & petang", Description: nullString("dhikr"), Frequency: repository.TasksFrequencyDaily, StartDate: today, Active: true},
		{Title: "Surah Al-Kahf", Description: nullString("quran"), Frequency: repository.TasksFrequencyWeekly, StartDate: today, Active: true},
		{Title: "Khatam Al-Qur'an", Description: nullString("quran"), Frequency: repository.TasksFrequencyMonthly, StartDate: today, Active: true},
	}

	existing, err := q.ListTasks(ctx)
	if err != nil {
		logger.Error("list tasks", "err", err)
		os.Exit(1)
	}
	have := map[string]bool{}
	for _, t := range existing {
		have[t.Title] = true
	}

	for _, p := range starter {
		if have[p.Title] {
			logger.Info("task already seeded", "title", p.Title)
			continue
		}
		id, err := q.CreateTask(ctx, p)
		if err != nil {
			logger.Error("create task", "title", p.Title, "err", err)
			os.Exit(1)
		}
		logger.Info("task seeded", "id", id, "title", p.Title)
	}

	logger.Info("seed complete")
}

func ensureAdmin(ctx context.Context, q *repository.Queries, email, password, name string) (int64, error) {
	existing, err := q.GetUserByEmail(ctx, email)
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	return q.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: string(hash),
		Name:         name,
		Role:         repository.UsersRoleAdmin,
		IsActive:     true,
	})
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
