-- name: GetAppSettings :one
SELECT id, week_start_day, history_weeks, created_at, updated_at
FROM app_settings
WHERE id = 1;

-- name: UpsertAppSettings :exec
INSERT INTO app_settings (id, week_start_day, history_weeks)
VALUES (1, ?, ?)
ON DUPLICATE KEY UPDATE week_start_day = VALUES(week_start_day), history_weeks = VALUES(history_weeks);
