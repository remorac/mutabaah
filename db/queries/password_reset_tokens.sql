-- name: CreatePasswordResetToken :exec
INSERT INTO password_reset_tokens (token_hash, user_id, expires_at)
VALUES (?, ?, ?);

-- name: GetValidPasswordResetToken :one
SELECT token_hash, user_id, expires_at, used_at, created_at
FROM password_reset_tokens
WHERE token_hash = ? AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP;

-- name: MarkPasswordResetTokenUsed :execrows
UPDATE password_reset_tokens
SET used_at = CURRENT_TIMESTAMP
WHERE token_hash = ? AND used_at IS NULL;

-- name: DeleteExpiredPasswordResetTokens :exec
DELETE FROM password_reset_tokens
WHERE expires_at <= CURRENT_TIMESTAMP OR used_at IS NOT NULL;
