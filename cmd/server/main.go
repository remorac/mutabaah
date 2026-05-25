package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"github.com/remorac/mutabaah/internal/config"
	"github.com/remorac/mutabaah/internal/handlers"
	apmw "github.com/remorac/mutabaah/internal/middleware"
	"github.com/remorac/mutabaah/internal/repository"
	"github.com/remorac/mutabaah/internal/services"
)

func main() {
	_ = godotenv.Load()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	db, err := sql.Open("mysql", cfg.DatabaseDSN)
	if err != nil {
		logger.Error("open db", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.PingContext(pingCtx); err != nil {
		pingCancel()
		logger.Error("ping db", "err", err)
		os.Exit(1)
	}
	pingCancel()

	queries := repository.New(db)
	auth := services.NewAuthService(queries, cfg.SessionSecret, cfg.SessionLifetime)
	mailer := services.NewSMTPMailer(services.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
		TLSMode:  cfg.SMTPTLSMode,
	})
	passwordResets := services.NewPasswordResetService(queries, mailer, cfg.AppBaseURL)
	tasks := services.NewTaskService(queries)
	taskAdmin := services.NewTaskAdminService(queries)
	userAdmin := services.NewUserAdminService(queries)
	mensesAdmin := services.NewMensesAdminService(queries)

	tmpl, err := handlers.LoadTemplates("web/templates")
	if err != nil {
		logger.Error("load templates", "err", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.AvatarDir, 0o755); err != nil {
		logger.Error("mkdir avatars", "err", err)
		os.Exit(1)
	}

	errorPages := handlers.NewErrorPages(tmpl, logger)
	authHandler := handlers.NewAuthHandler(auth, passwordResets, tmpl, logger, cfg.SecureCookies)
	dashHandler := handlers.NewDashboardHandler(auth, tasks, tmpl, errorPages, logger)
	calHandler := handlers.NewCalendarHandler(auth, tasks, tmpl, errorPages, logger)
	reportHandler := handlers.NewReportHandler(auth, tasks, userAdmin, tmpl, errorPages, logger)
	settingsTasksHandler := handlers.NewSettingsTasksHandler(auth, taskAdmin, tmpl, errorPages, logger)
	settingsUsersHandler := handlers.NewSettingsUsersHandler(auth, userAdmin, tmpl, errorPages, logger)
	profileHandler := handlers.NewProfileHandler(auth, userAdmin, mensesAdmin, tmpl, errorPages, logger, cfg.AvatarDir)

	// Wire styled 403 into RequireAdmin (middleware can't import handlers).
	apmw.ForbiddenHandler = errorPages.Forbidden

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(apmw.SlogRequestLogger(logger))
	r.Use(chimw.Recoverer)
	r.Use(apmw.SecurityHeaders)
	r.Use(apmw.LoadUser(auth))
	r.Use(apmw.CSRF(auth))

	r.NotFound(errorPages.NotFound)
	r.MethodNotAllowed(errorPages.MethodNotAllowed)

	r.Handle("/static/avatars/*", http.StripPrefix("/static/avatars/", http.FileServer(http.Dir(cfg.AvatarDir))))
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})

	loginLimit := apmw.RateLimit(10, 6*time.Second)
	r.Get("/login", authHandler.LoginForm)
	r.With(loginLimit).Post("/login", authHandler.Login)
	r.Get("/forgot-password", authHandler.ForgotPasswordForm)
	r.With(loginLimit).Post("/forgot-password", authHandler.ForgotPassword)
	r.Get("/reset-password", authHandler.ResetPasswordForm)
	r.With(loginLimit).Post("/reset-password", authHandler.ResetPassword)
	r.Post("/logout", authHandler.Logout)

	r.Group(func(r chi.Router) {
		r.Use(apmw.RequireAuth)
		r.Get("/", dashHandler.Home)
		r.Post("/tasks/{id}/complete", dashHandler.ToggleComplete)
		r.Get("/calendar", calHandler.Month)
		r.Get("/calendar/day", calHandler.Day)
		r.Get("/settings/profile", profileHandler.Show)
		r.Post("/settings/profile", profileHandler.ChangePassword)
		r.Post("/settings/profile/picture", profileHandler.UploadPicture)
		r.Post("/settings/periods", profileHandler.CreatePeriod)
		r.Post("/settings/periods/{id}/delete", profileHandler.DeletePeriod)
	})

	r.Group(func(r chi.Router) {
		r.Use(apmw.RequireAuth)
		r.Use(apmw.RequireAdmin)
		r.Get("/reports", reportHandler.Show)
		r.Get("/reports/export.pdf", reportHandler.ExportPDF)
		r.Get("/settings/tasks", settingsTasksHandler.List)
		r.Get("/settings/tasks/new", settingsTasksHandler.NewForm)
		r.Post("/settings/tasks", settingsTasksHandler.Create)
		r.Get("/settings/tasks/{id}/edit", settingsTasksHandler.EditForm)
		r.Post("/settings/tasks/{id}", settingsTasksHandler.Update)
		r.Post("/settings/tasks/{id}/active", settingsTasksHandler.SetActive)
		r.Post("/settings/tasks/{id}/delete", settingsTasksHandler.Delete)
		r.Post("/settings/tasks/{id}/move-up", settingsTasksHandler.MoveUp)
		r.Post("/settings/tasks/{id}/move-down", settingsTasksHandler.MoveDown)
		r.Get("/settings/users", settingsUsersHandler.List)
		r.Get("/settings/users/new", settingsUsersHandler.NewForm)
		r.Post("/settings/users", settingsUsersHandler.Create)
		r.Get("/settings/users/{id}/edit", settingsUsersHandler.EditForm)
		r.Post("/settings/users/{id}", settingsUsersHandler.Update)
		r.Post("/settings/users/{id}/delete", settingsUsersHandler.Delete)
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "err", err)
	}
}
