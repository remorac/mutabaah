# Mutaba'ah Tracker — Development Plan

A phased plan for building the Mutaba'ah Tracker per [specification.txt](specification.txt). Each phase is independently executable; later phases assume earlier ones are complete but each phase ends in a runnable state.

---

## Phase 0 — Project Scaffolding

**Goal:** Bootable Go HTTP server that serves a "Hello" page using the chosen stack.

- [ ] Initialize Go module: `go mod init github.com/<user>/tracker`
- [ ] Create directory layout:
  ```
  /cmd/server/main.go
  /internal/
    /handlers/      (HTTP handlers)
    /services/      (business logic)
    /repository/    (DB access, sqlc generated)
    /models/        (domain types)
    /middleware/    (auth, logging, recovery)
    /config/        (env loading)
  /db/
    /migrations/    (.sql files)
    /queries/       (sqlc input)
    /schema.sql
    sqlc.yaml
  /web/
    /templates/     (Go html/template)
    /static/        (favicon, minimal CSS overrides)
  .env.example
  Makefile
  README.md
  ```
- [ ] Add dependencies: `chi`, `chi/middleware`, `go-sql-driver/mysql`, `joho/godotenv`, `golang-migrate` (CLI), `sqlc` (CLI)
- [ ] Configure [sqlc.yaml](db/sqlc.yaml) targeting MySQL
- [ ] Set up base layout template ([web/templates/layout.html](web/templates/layout.html)) loading CDN assets:
  - Tailwind CSS 4
  - DaisyUI 5
  - Lucide Icons
  - Alpine JS
  - HTMX
- [ ] Implement minimal `/healthz` endpoint and a render-only `/` page to validate the template + CDN pipeline
- [ ] Add Makefile targets: `run`, `build`, `migrate-up`, `migrate-down`, `sqlc-gen`, `fmt`, `vet`
- [ ] Document env vars in `.env.example` (DB DSN, session secret, port)

**Deliverable:** `make run` boots, browser shows a styled "Hello, Mutaba'ah" page with DaisyUI theme.

---

## Phase 1 — Database Schema & Migrations

**Goal:** All tables created via migrations; sqlc generates typed query code.

### Tables

- [ ] `users`
  - `id` BIGINT PK AUTO_INCREMENT
  - `email` VARCHAR(255) UNIQUE NOT NULL
  - `password_hash` VARCHAR(255) NOT NULL
  - `name` VARCHAR(255) NOT NULL
  - `role` ENUM('admin','user') NOT NULL DEFAULT 'user'
  - `created_at`, `updated_at` TIMESTAMP
- [ ] `tasks` — task definitions (templates, not occurrences)
  - `id` BIGINT PK
  - `user_id` BIGINT FK → users.id (NULL = applies to all users, or per-user if non-null — decide in design step)
  - `title` VARCHAR(255) NOT NULL
  - `description` TEXT
  - `category` VARCHAR(64) (e.g., fard, sunnah, quran, dhikr)
  - `frequency` ENUM('daily','weekly','monthly','yearly') NOT NULL
  - `start_date` DATE NOT NULL
  - `end_date` DATE NULL
  - `active` BOOL DEFAULT TRUE
  - `created_at`, `updated_at`
- [ ] `task_completions` — one row per (task, user, date) when completed
  - `id` BIGINT PK
  - `task_id` BIGINT FK → tasks.id
  - `user_id` BIGINT FK → users.id
  - `due_date` DATE NOT NULL (the scheduled occurrence date)
  - `completed_at` TIMESTAMP NULL
  - UNIQUE (`task_id`, `user_id`, `due_date`)
- [ ] `sessions` (if using DB-backed sessions)
  - `id` CHAR(64) PK (random token)
  - `user_id` BIGINT FK
  - `expires_at` TIMESTAMP
  - `created_at`

### Tasks

- [ ] Write up migrations under [db/migrations/](db/migrations/) (one up/down pair per table)
- [x] **Task ownership model**: tasks are admin-defined templates globally visible to every user; per-user completion state lives in `task_completions(task_id, user_id, due_date)`.
- [ ] Write sqlc queries: CRUD for users/tasks, occurrence-fetching queries (today's tasks, missed tasks, calendar range)
- [ ] Generate sqlc code into [internal/repository/](internal/repository/)
- [ ] Seed script: one admin user + a handful of starter tasks (fard salah ×5, dhikr, weekly Surah Al-Kahf, monthly Quran khatam)

**Deliverable:** `make migrate-up && make sqlc-gen` succeeds; seed data visible in MySQL.

---

## Phase 2 — Authentication

**Goal:** Email/password login with secure session cookies; role-aware middleware.

- [x] Password hashing: `golang.org/x/crypto/bcrypt`
- [x] Session strategy: signed cookie OR DB-backed sessions table (recommend DB-backed for revocability) — chose **DB-backed** ([internal/services/auth.go](internal/services/auth.go))
- [x] Handlers ([internal/handlers/auth.go](internal/handlers/auth.go)):
  - `GET /login` — render form
  - `POST /login` — verify credentials, create session, set HttpOnly + Secure + SameSite=Lax cookie
  - `POST /logout` — destroy session
- [x] Middleware ([internal/middleware/](internal/middleware/)):
  - `RequireAuth` — redirect to /login when unauthenticated
  - `RequireAdmin` — 403 for non-admins
  - CSRF protection — hand-rolled HMAC token bound to session id (no extra storage; rotates with the session)
- [x] Rate limiting on `/login` (token bucket per IP) — [internal/middleware/ratelimit.go](internal/middleware/ratelimit.go)
- [x] Login form view ([web/templates/auth/login.html](web/templates/auth/login.html)) using DaisyUI `card` + `input` components

**Deliverable:** Admin and regular user can log in/out; protected routes enforce auth; CSRF tokens present in all forms.

---

## Phase 3 — Task Definitions & Occurrence Engine

**Goal:** Given task definitions, compute which occurrences are due for a given user on a given date.

- [x] Implement occurrence resolver in [internal/services/tasks.go](internal/services/tasks.go):
  - `OccurrencesOn(userID, date) []TaskOccurrence`
  - `OccurrencesBetween(userID, from, to) []TaskOccurrence`
  - Daily → every day from `start_date`
  - Weekly → every 7 days from `start_date` (anchored to start_date weekday)
  - Monthly → same day-of-month from `start_date`; months without that day are skipped (e.g. Jan 31 → skip Feb/Apr/Jun/Sep/Nov)
  - Yearly → same month/day from `start_date`; Feb 29 only fires on leap years
- [x] Merge with `task_completions` to mark completed/missed/pending
  - Pending: due_date >= today, not completed
  - Missed: due_date < today, not completed
  - Completed: has matching row
- [x] Unit tests for resolver edge cases (Feb 29 yearly, Jan 31 monthly, end_date, range clamping, status assignment, sort order)
- [x] Hijri calendar consideration: out of scope for v1 (Gregorian only); note for future.

**Deliverable:** `go test ./internal/services/...` passes; service layer can answer "what's due today" without UI.

---

## Phase 4 — Dashboard

**Goal:** Logged-in user lands on a dashboard showing today's tasks and missed tasks.

- [x] `GET /` — dashboard (auth required)
- [x] Sections:
  - **Today's Tasks** — split into Completed (checkmark icon) and Uncompleted (cross icon)
  - **Missed Tasks** — list with cross icons, limited to last N days (e.g., 30)
- [x] Each task row has a checkbox/button to mark complete; uses HTMX `POST /tasks/:id/complete?date=YYYY-MM-DD` with partial swap (entire `#dashboard-inner` re-renders so rows can move between columns)
- [x] Allow un-completing (toggle behavior) — `TaskService.Toggle` validates (task, date) before any DB write
- [x] Template partials: [web/templates/dashboard/index.html](web/templates/dashboard/index.html), [web/templates/dashboard/_dashboard_inner.html](web/templates/dashboard/_dashboard_inner.html), [web/templates/dashboard/_task_row.html](web/templates/dashboard/_task_row.html)
- [x] Icons via Lucide (`check`, `x`, `circle`, `undo-2`)

**Deliverable:** User sees their day, can tick tasks off; no full page reloads.

---

## Phase 5 — Calendar View

**Goal:** Monthly calendar grid showing past completions and upcoming due tasks.

- [x] `GET /calendar?month=YYYY-MM` — renders month grid ([internal/handlers/calendar.go](internal/handlers/calendar.go))
- [x] Each cell shows: date number + small indicators (count of completed / total due)
- [x] Click a cell → HTMX-load a side panel listing that day's tasks with status (`GET /calendar/day?date=YYYY-MM-DD`)
- [x] Navigation: prev / next month, jump to today
- [x] Performance: single occurrences-in-range fetch covering the padded grid; resolver merges completions
- [x] Color intensity per day based on completion ratio (heatmap-style); future days render flat to avoid looking missed

**Deliverable:** User can browse any month, see history, click days for detail.

---

## Phase 6 — Settings: Task Management (Admin)

**Goal:** Admin CRUD over task definitions.

- [x] `GET /settings/tasks` — list with search, filter by frequency/category ([internal/handlers/settings_tasks.go](internal/handlers/settings_tasks.go))
- [x] `GET /settings/tasks/new` and `POST /settings/tasks`
- [x] `GET /settings/tasks/:id/edit` and `POST /settings/tasks/:id`
- [x] `POST /settings/tasks/:id/delete` — soft delete via `active=false` to preserve completion history
- [x] Form fields: title, description, category, frequency, start_date, end_date, assignment scope (all users or specific users)
- [x] DaisyUI components: `table`, Alpine-driven `modal` for confirm delete, `input`, `select`, `textarea`, `toggle`, `radio`
- [x] Validation in service layer with friendly error rendering ([internal/services/task_admin.go](internal/services/task_admin.go) — `ValidationError` carries per-field messages; form re-renders with 422 + inline errors)
- [x] Admin gate on the route group; non-admins get 403, Settings link hidden in sidebar for non-admins

**Deliverable:** Admin can fully manage tasks; non-admins get 403.

---

## Phase 7 — Settings: User Management (Admin)

**Goal:** Admin CRUD over users.

- [x] `GET /settings/users` — list (paginated if >50) ([internal/handlers/settings_users.go](internal/handlers/settings_users.go))
- [x] `GET /settings/users/new` and `POST /settings/users` (admin sets initial password; user can change later)
- [x] `GET /settings/users/:id/edit` and `POST /settings/users/:id`
- [x] `POST /settings/users/:id/delete` — guard against deleting last admin (also blocks self-delete and demote-of-last-admin)
- [x] User "profile" page for self-service password change (`GET/POST /settings/profile`) ([internal/handlers/profile.go](internal/handlers/profile.go)) — revokes all sessions on success
- [x] Validation: email format, password strength (min 12 chars), unique email ([internal/services/user_admin.go](internal/services/user_admin.go))

**Deliverable:** Admin manages users; users can change their own password.

---

## Phase 8 — Polish & Hardening

- [x] Consistent error pages (404, 403, 500) with DaisyUI styling ([internal/handlers/errors.go](internal/handlers/errors.go), [web/templates/errors/error.html](web/templates/errors/error.html)) — HTMX/XHR requests get plain text so partial swaps don't replace a row with a full page
- [x] Structured logging (`slog`) with request IDs ([internal/middleware/logger.go](internal/middleware/logger.go)) — chi `RequestID` feeds an access-log slog entry; `RequestLogger(r)` returns a request-scoped logger for handlers
- [x] Graceful shutdown (`http.Server.Shutdown` on SIGTERM) — already wired in [cmd/server/main.go](cmd/server/main.go)
- [x] Security headers middleware (CSP, X-Frame-Options, Referrer-Policy) — [internal/middleware/security.go](internal/middleware/security.go); CSP allow-lists the CDN origins layout.html uses (`cdn.jsdelivr.net`, `unpkg.com`)
- [x] CSRF token coverage audit on every POST — every POST form carries `_csrf` (or `X-CSRF-Token` via `hx-headers` for HTMX); only `/login` is exempt by design (anonymous)
- [ ] Audit log table for admin actions (optional) — deferred
- [x] Timezone handling: the app uses fixed `Asia/Jakarta` time for its "today" reference.
- [x] Backup strategy doc for MySQL — [docs/backup.md](docs/backup.md)
- [x] Dockerfile + docker-compose (app + MySQL) for local dev — [Dockerfile](Dockerfile), [docker-compose.yml](docker-compose.yml), [.dockerignore](.dockerignore)

**Deliverable:** Production-ready single-binary deployment story.

---

## Phase 9 — Stretch / Future

- [ ] Hijri calendar overlay in calendar view
- [ ] Streaks and stats (current streak, longest streak per task)
- [ ] Email reminders for missed tasks (cron + SMTP)
- [ ] Export completion history (CSV)
- [ ] PWA / offline mode
- [ ] Multi-language UI (English / Bahasa Indonesia / Arabic)

---

## Open Design Questions

These should be resolved before or during the phase they affect:

1. **Task ownership** — Are tasks global (admin defines, all users see) or per-user? Recommended: admin-defined templates with explicit user assignment (Phase 1).
2. **Edit-history of completions** — Can users back-fill completions for past dates? (Affects Phase 4/5 UX.)
3. **Timezone** — Resolved: fixed `Asia/Jakarta` app timezone (Phase 8).
4. **Hijri vs Gregorian** — V1 Gregorian only; Hijri is Phase 9.
5. **Session storage** — Cookie-only signed vs DB-backed. Recommended: DB-backed for revocability (Phase 2).

---

## Suggested Execution Order

Phases are linear (0 → 8). Phase 9 is optional. Each phase should end with a working app: don't merge a phase that leaves the build broken. Recommended cadence: one phase = one PR.
