-- name: CreateMensesPeriod :execlastid
INSERT INTO menses_periods (user_id, start_date, end_date) VALUES (?, ?, ?);

-- name: UpdateMensesPeriod :exec
UPDATE menses_periods SET start_date = ?, end_date = ? WHERE id = ? AND user_id = ?;

-- name: DeleteMensesPeriod :exec
DELETE FROM menses_periods WHERE id = ? AND user_id = ?;

-- name: GetMensesPeriod :one
SELECT id, user_id, start_date, end_date, created_at, updated_at
FROM menses_periods WHERE id = ? AND user_id = ?;

-- name: ListMensesPeriodsForUser :many
SELECT id, user_id, start_date, end_date, created_at, updated_at
FROM menses_periods WHERE user_id = ? ORDER BY start_date DESC;

-- name: ListMensesPeriodsForUserInRange :many
SELECT id, user_id, start_date, end_date, created_at, updated_at
FROM menses_periods
WHERE user_id = ?
  AND start_date <= sqlc.arg('to_date')
  AND (end_date IS NULL OR end_date >= sqlc.arg('from_date'))
ORDER BY start_date ASC;
