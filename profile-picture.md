# Add Feature: User Profile Picture

## Context

The app currently shows a placeholder Lucide `user` icon in both the navbar ([layout.html:275-284](web/templates/layout.html#L275-L284)) and the profile page ([settings/profile.html:15-25](web/templates/settings/profile.html#L15-L25)). Users have no way to personalize their account, so the sidebar feels anonymous in multi-user households. This feature lets a signed-in user upload a profile picture from the profile page; a generated thumbnail replaces the placeholder everywhere the navbar avatar appears.

Decisions already made with the user:
- Storage: **local filesystem** at `web/static/avatars/` (already served by the existing `http.FileServer` at `/static/*` in [cmd/server/main.go:93](cmd/server/main.go#L93))
- Processing: save the **original**, also generate a **256×256 thumbnail** that the UI displays
- No remove feature — uploading a new picture overwrites the previous one

## Scope

- One column on `users`: `avatar_path VARCHAR(255) NULL` (just the filename — thumbnail name is derived as `thumb_<filename>`)
- Accepted types: JPEG, PNG only (sniffed via `http.DetectContentType`, not extension)
- Max upload size: 5 MiB
- Thumbnails always written as JPEG for size

## Implementation Steps

### 1. Migration

Create `db/migrations/000013_add_user_avatar.up.sql`:
```sql
ALTER TABLE users ADD COLUMN avatar_path VARCHAR(255) NULL AFTER name;
```

Create `db/migrations/000013_add_user_avatar.down.sql`:
```sql
ALTER TABLE users DROP COLUMN avatar_path;
```

### 2. sqlc query + regen

In [db/queries/users.sql](db/queries/users.sql):
- Add `avatar_path` to every `SELECT` list (4 spots)
- Append:
  ```sql
  -- name: UpdateUserAvatar :exec
  UPDATE users SET avatar_path = ? WHERE id = ?;
  ```

Run `sqlc generate`. This regenerates [internal/repository/users.sql.go](internal/repository/users.sql.go) and adds `AvatarPath sql.NullString` to `repository.User` in [internal/repository/models.go:139](internal/repository/models.go#L139).

### 3. BaseView plumbing

In [internal/handlers/view.go](internal/handlers/view.go):
- Add `AvatarPath string` to `BaseView`
- Add a constructor:
  ```go
  func NewBaseView(user repository.User, csrfToken, title string) BaseView {
      av := ""
      if user.AvatarPath.Valid && user.AvatarPath.String != "" {
          av = "/static/avatars/thumb_" + user.AvatarPath.String
      }
      return BaseView{Title: title, UserName: user.Name, UserRole: string(user.Role), CSRFToken: csrfToken, AvatarPath: av}
  }
  ```

Update every `BaseView{...}` literal that has a real user to call `NewBaseView(user, token, title)`. Sites identified via grep: [internal/handlers/profile.go](internal/handlers/profile.go) (4 spots), [internal/handlers/settings_users.go](internal/handlers/settings_users.go) (6 spots), [internal/handlers/settings_tasks.go](internal/handlers/settings_tasks.go) (5 spots), plus the dashboard/calendar/reports handlers wherever they construct a BaseView from a user. The login/error pages have no user — leave those literals as-is (their `AvatarPath` stays empty, layout falls back to placeholder).

### 4. Image helper package

Create `internal/imageutil/avatar.go` with:
- `Validate(data []byte) (mime, ext string, err error)` — runs `http.DetectContentType` on first 512 bytes; accepts only `image/jpeg` → `.jpg`, `image/png` → `.png`
- `SaveOriginalAndThumb(dir, basename, ext string, data []byte) error`:
  - Decode via `image/jpeg` or `image/png`
  - Center-crop the largest centered square
  - Resize to 256×256 using `golang.org/x/image/draw` with `draw.CatmullRom.Scale`
  - Encode thumbnail as JPEG quality 85 → write to `dir/thumb_<basename>.jpg`
  - Write original bytes verbatim → `dir/<basename><ext>`
  - File mode `0o644`

Add dep: `go get golang.org/x/image/draw && go mod tidy`.

### 5. Service method

In [internal/services/user_admin.go](internal/services/user_admin.go), add:
```go
func (s *UserAdminService) UpdateAvatar(ctx context.Context, userID int64, filename string) error {
    return s.q.UpdateUserAvatar(ctx, repository.UpdateUserAvatarParams{
        AvatarPath: sql.NullString{String: filename, Valid: filename != ""},
        ID:         userID,
    })
}
```

The service does not touch the filesystem or validate MIME — that lives in the handler.

### 6. Handler

In [internal/handlers/profile.go](internal/handlers/profile.go), add `UploadPicture(w, r)`:
1. `apmw.UserFromContext(r.Context())` → user; `apmw.SessionIDFromContext` → CSRF token
2. `r.Body = http.MaxBytesReader(w, r.Body, 6<<20)` then `r.ParseMultipartForm(6 << 20)`
3. `file, hdr, err := r.FormFile("avatar")` — on miss, re-render profile with `Errors["avatar"] = "Choose a file."`
4. Read full bytes (already capped). Reject if `> 5<<20`.
5. `imageutil.Validate(data)` → reject non-JPEG/PNG with `Errors["avatar"] = "JPEG or PNG only."`
6. Build basename: `fmt.Sprintf("%d_%s", user.ID, hex.EncodeToString(randomBytes(6)))` using `crypto/rand`
7. `imageutil.SaveOriginalAndThumb("web/static/avatars", basename, ext, data)`
8. Best-effort: look up the existing user, delete old original + thumb files via `os.Remove`; log failures but don't fail the request
9. `h.users.UpdateAvatar(ctx, user.ID, basename+ext)`
10. `http.Redirect(w, r, "/settings/profile", http.StatusSeeOther)`

Update `Show` and `ChangePassword` to use `NewBaseView`. Re-render on validation failure follows the existing `Errors` map pattern.

### 7. Route + startup

In [cmd/server/main.go](cmd/server/main.go):
- Inside the auth-required group (after line 113), add:
  ```go
  r.Post("/settings/profile/picture", profileHandler.UploadPicture)
  ```
- After templates are loaded (~line 67), ensure the directory exists:
  ```go
  if err := os.MkdirAll("web/static/avatars", 0o755); err != nil { logger.Error("mkdir avatars", "err", err); os.Exit(1) }
  ```

### 8. Templates

In [web/templates/layout.html:275-284](web/templates/layout.html#L275-L284), conditionally render an `<img>`:
```
{{if .AvatarPath}}
  <div class="avatar">
    <div class="w-10 rounded-full">
      <img src="{{.AvatarPath}}" alt="{{.UserName}}" loading="lazy" />
    </div>
  </div>
{{else}}
  <!-- existing placeholder -->
{{end}}
```

In [web/templates/settings/profile.html:15-25](web/templates/settings/profile.html#L15-L25), same conditional render. Add a new card after the profile card with:
- `<form method="POST" enctype="multipart/form-data" action="/settings/profile/picture">`
- Hidden `_csrf`
- `<input type="file" name="avatar" accept="image/jpeg,image/png" required />`
- Submit button styled like the existing "Update password" button
- `{{with index .Errors "avatar"}}<span class="text-error text-xs">{{.}}</span>{{end}}`

### 9. .gitignore

Append to `.gitignore`:
```
web/static/avatars/*
!web/static/avatars/.gitkeep
```
Create an empty `web/static/avatars/.gitkeep`.

## Files to Create

- `db/migrations/000013_add_user_avatar.up.sql`
- `db/migrations/000013_add_user_avatar.down.sql`
- `internal/imageutil/avatar.go`
- `web/static/avatars/.gitkeep`

## Files to Modify

- `db/queries/users.sql` (add column to SELECTs + new UpdateUserAvatar query)
- `internal/repository/users.sql.go` and `models.go` (regenerated by sqlc)
- `internal/handlers/view.go` (BaseView + NewBaseView)
- `internal/handlers/profile.go` (UploadPicture handler + use NewBaseView)
- `internal/handlers/settings_users.go`, `settings_tasks.go`, plus dashboard/calendar/reports handlers (switch BaseView literals to NewBaseView)
- `internal/services/user_admin.go` (UpdateAvatar method)
- `cmd/server/main.go` (route + MkdirAll)
- `web/templates/layout.html` (conditional avatar)
- `web/templates/settings/profile.html` (conditional avatar + upload form)
- `go.mod`, `go.sum` (`golang.org/x/image` dep)
- `.gitignore`

## Verification

1. `sqlc generate` — confirm `users.sql.go` has `UpdateUserAvatar` and `User.AvatarPath` is `sql.NullString`.
2. Apply migration — `DESCRIBE users` shows the new column.
3. `go vet ./... && go build ./...` — must succeed.
4. Run the server; log in.
5. GET `/settings/profile` — placeholder icon shown, upload card visible.
6. Upload a small JPEG — expect 303 redirect; navbar + profile card now show the thumbnail. Confirm `web/static/avatars/` contains `<id>_<rand>.jpg` and `thumb_<id>_<rand>.jpg`.
7. Reload `/`, `/calendar`, `/reports` — navbar avatar persists across pages.
8. Upload a PNG — original stored as `.png`, thumbnail still `.jpg`.
9. Upload a second image — previous original + thumb removed from disk; DB reflects new filename.
10. Upload a `.txt` renamed to `.jpg` — rejected with inline error, no DB change, no disk write.
11. Upload a 7 MiB file — rejected with size error.
