-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, impersonator_user_id, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetSession :one
SELECT id, user_id, impersonator_user_id, expires_at, created_at
FROM sessions
WHERE id = ? AND expires_at > CURRENT_TIMESTAMP;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;

-- name: DeleteUserSessions :exec
DELETE FROM sessions
WHERE user_id = ? OR impersonator_user_id = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP;
