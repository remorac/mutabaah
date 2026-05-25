# Remove multi-timezone feature, hardcode `Asia/Jakarta`

## Context

The app currently lets each user pick their own IANA timezone (admin form + `/settings/profile/timezone` self-serve). We don't need per-user timezones — the whole product runs in Indonesia. Remove the feature entirely and use a single hardcoded location, `Asia/Jakarta` (UTC+7), as the app's "today" reference. This deletes a column, a migration, two form fields, a route, a service method, and several validation paths.

## Approach

1. Add a new package-level location in `internal/services`:
   - `var AppLocation = mustLoadLocation("Asia/Jakarta")` (panic at init if zoneinfo is missing — bundled in Go's `time/tzdata` if needed; otherwise relies on system zoneinfo, same as today's `time.LoadLocation` calls).
2. Drop the DB column with a new migration `000008_drop_user_timezone`.
3. Strip `Timezone` from the User model, queries, services, handlers, views, and templates.
4. Replace `services.UserLocation(u)` calls with `services.AppLocation`. `todayFor` loses its `repository.User` argument.

## Files to change

### DB

- **New migration** [db/migrations/000008_drop_user_timezone.up.sql](db/migrations/000008_drop_user_timezone.up.sql)
  - `ALTER TABLE users DROP COLUMN timezone;`
- **New migration** [db/migrations/000008_drop_user_timezone.down.sql](db/migrations/000008_drop_user_timezone.down.sql)
  - `ALTER TABLE users ADD COLUMN timezone VARCHAR(64) NOT NULL DEFAULT 'UTC' AFTER role;`
- [db/schema.sql](db/schema.sql) — remove the `timezone` column from the `users` definition (line 10).
- [db/queries/users.sql](db/queries/users.sql) — remove `timezone` from `CreateUser` (insert columns + `?`), all four `SELECT` projections (lines 6, 11, 16, 46), and `UpdateUser` (line 29). Delete the entire `UpdateUserTimezone` query (lines 37–40).

### Generated sqlc code

Edit in place (project doesn't run sqlc in CI; the generated files are committed):

- [internal/repository/models.go](internal/repository/models.go) — drop `Timezone string` from the `User` struct (line 137).
- [internal/repository/users.sql.go](internal/repository/users.sql.go) — remove the `Timezone` field from `CreateUserParams`, `UpdateUserParams`; remove the column from every `SELECT` and `Scan` call; delete `UpdateUserTimezone`, `UpdateUserTimezoneParams`, and its SQL constant.
- [internal/repository/querier.go](internal/repository/querier.go) — remove the `UpdateUserTimezone(...)` method from the `Querier` interface.

(After the manual edit, optionally re-run sqlc locally to confirm the output matches — not required for the plan, but recommended.)

### Services

- [internal/services/user_admin.go](internal/services/user_admin.go)
  - Remove `Timezone` field from `UserInput` (line 34) and its doc reference (line 28).
  - Drop the `Timezone:` line from `Create` and `Update` calls (lines 93, 129).
  - Delete `SetTimezone(...)` (lines 175–186).
  - Update `validateProfile` (lines 230–273) to no longer compute/return `tz` — adjust signature to return `(email, name, role, verrs)`; remove the `tz, tzErr := NormalizeTimezone(in.Timezone)` block.
  - Delete `NormalizeTimezone` (lines 275–289) and `UserLocation` (lines 291–302).
  - Add the package-level constant and loader:

    ```go
    // AppLocation is the single timezone the app evaluates "today" in.
    var AppLocation = func() *time.Location {
        loc, err := time.LoadLocation("Asia/Jakarta")
        if err != nil {
            panic("services: Asia/Jakarta zoneinfo unavailable: " + err.Error())
        }
        return loc
    }()
    ```

  - Update the comment on `Create` (line 28) — no more "timezone defaults to UTC" note.
- [internal/services/tasks.go](internal/services/tasks.go) — replace "user's timezone" wording in the comments on lines 51, 60, 66, 100 with "app timezone" (no code change; signatures already take an explicit `today`).

### Handlers

- [internal/handlers/dashboard.go](internal/handlers/dashboard.go)
  - Change `todayFor` signature from `todayFor(u repository.User, now func() time.Time) time.Time` to `todayFor(now func() time.Time) time.Time` and use `services.AppLocation` instead of `services.UserLocation(u)` (lines 17–23).
  - Update call sites on lines 102 and 140 to `todayFor(h.now)`.
- [internal/handlers/calendar.go](internal/handlers/calendar.go) — update the two `todayFor(user, h.now)` calls on lines 79 and 115 to `todayFor(h.now)`.
- [internal/handlers/profile.go](internal/handlers/profile.go)
  - Remove `Timezone`, `TzErrors`, `TzSavedFlag` from `profileView` (lines 29, 30, 31).
  - Remove `Timezone:` and `TzSavedFlag:` assignments in `Show` (lines 51–52) and the `?tz_saved=1` query handling.
  - Delete `ChangeTimezone(...)` entirely (lines 56–90).
- [internal/handlers/settings_users.go](internal/handlers/settings_users.go)
  - Remove `Timezone string` from `userFormView` (line 65).
  - Drop the `Timezone:` lines in `NewForm`, `EditForm`, error re-renders, and the `Timezone` assignment on `parseUserInput` (lines 146, 183, 230, 274, 299, 355).
- [cmd/server/main.go](cmd/server/main.go) — delete the `r.Post("/settings/profile/timezone", profileHandler.ChangeTimezone)` route (line 111).
- [cmd/seed/main.go](cmd/seed/main.go) — drop the `Timezone: "UTC"` line (line 118).

### Templates

- [web/templates/settings/profile.html](web/templates/settings/profile.html) — delete the `TzSavedFlag` alert block (lines 19–24) and the entire timezone `<form>` (lines 26–47).
- [web/templates/settings/users/form.html](web/templates/settings/users/form.html) — delete the timezone label/input/help block (lines 63–70).

### Docs

- [PLAN.md](PLAN.md) — replace the bullet at lines 214–217 (Phase 8 timezone handling) with a one-line note that the app runs in a fixed `Asia/Jakarta` timezone; update line 242's "Single server TZ vs. per-user TZ" trade-off note accordingly.

## Verification

1. **Build:** `go build ./...` — confirms the sqlc, services, and handlers changes line up (no stale `User.Timezone` references).
2. **Migrate from scratch:** drop the dev DB, run `make migrate` (or the equivalent `migrate -path db/migrations ...` command), and confirm migrations 000001–000008 apply cleanly. Then run `make migrate down` partway and back up to confirm 000008 reverses.
3. **Seed:** `go run ./cmd/seed` — confirm the seeded admin user is created without a `Timezone` field.
4. **Smoke the app:**
   - `go run ./cmd/server`, log in as the seeded admin.
   - `/settings/profile` should render the password-change card only — no timezone block, no `?tz_saved=1` alert.
   - `POST /settings/profile/timezone` should 404 (route deleted).
   - `/settings/users/new` and the edit form should not show a timezone field; create + update flows still succeed.
   - Dashboard "Today" and the calendar should match the current Asia/Jakarta wall-clock date (test around 17:00 UTC, which is already the next day in Jakarta).
5. **Tests:** `go test ./...` (no existing timezone tests per the exploration, but rerun in case any indirect coverage exists).
