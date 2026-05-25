# Mutaba'ah Tracker

A task tracker for Islamic practices (daily/weekly/monthly/yearly tarbiyah tasks). See [specification.txt](specification.txt) and [PLAN.md](PLAN.md).

## Stack
- Go monolith — chi router, `html/template`
- MySQL — `sqlc` for typed queries, `golang-migrate` for migrations
- DaisyUI 5 / Tailwind CSS 4 / Lucide / Alpine.js / HTMX (all via CDN)

## Quick start (Phase 0)

```bash
cp .env.example .env
make run
```

Open <http://localhost:8080> to see the styled "Hello, Mutaba'ah" page. `/healthz` returns `ok`.

## Layout

```
cmd/server/        HTTP entrypoint
internal/
  handlers/        HTTP handlers
  services/        business logic
  repository/      sqlc-generated DB access
  models/          domain types
  middleware/      auth, logging, recovery
  config/          env loading
db/
  migrations/      *.sql migration pairs
  queries/         sqlc input
  schema.sql       composite reference
  sqlc.yaml
web/
  templates/       html/template files
  static/          favicon, minimal css overrides
```

## Make targets
- `make run` — start server
- `make build` — build binary to `bin/tracker`
- `make seed` — create the initial admin user + starter tasks (idempotent)
- `make migrate-up` / `make migrate-down` — apply/revert migrations (requires `migrate` CLI)
- `make sqlc-gen` — regenerate repository code
- `make fmt` / `make vet` / `make tidy` / `make test`

## Password reset email

Forgot-password links are sent by SMTP. Configure `APP_BASE_URL`,
`SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM`, and optionally `SMTP_USERNAME`,
`SMTP_PASSWORD`, and `SMTP_TLS_MODE` (`starttls`, `implicit`, or `none`).
For local development, a Mailpit/MailHog-style SMTP listener on port 1025 works
with `SMTP_TLS_MODE=none`.

## Phase 1 — schema & seed

After installing the `migrate` CLI and running a local MySQL:

```bash
mysql -uroot -e "CREATE DATABASE tracker CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; \
                 CREATE USER 'tracker'@'%' IDENTIFIED BY 'tracker'; \
                 GRANT ALL ON tracker.* TO 'tracker'@'%';"
make migrate-up
make seed   # admin@example.com / changeme-admin-12345 — override with flags
```

Override the seeded admin with flags, e.g. `go run ./cmd/seed -email me@example.com -password 'long-random-secret' -name 'Me'`.

### Task ownership model

Tasks are admin-defined templates (the `tasks` table has no `user_id`) and are
globally visible to every user. Per-occurrence completion is recorded per user
in the `task_completions` table.

## Tooling

Install the CLI tools needed for later phases:

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```
