-- name: CreateUser :execlastid
INSERT INTO users (email, password_hash, name, role)
VALUES (?, ?, ?, ?);

-- name: GetUserByID :one
SELECT id, email, password_hash, name, role, created_at, updated_at, avatar_path
FROM users
WHERE id = ?;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, name, role, created_at, updated_at, avatar_path
FROM users
WHERE email = ?;

-- name: ListUsers :many
SELECT id, email, password_hash, name, role, created_at, updated_at, avatar_path
FROM users
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CountAdmins :one
SELECT COUNT(*) FROM users WHERE role = 'admin';

-- name: UpdateUser :exec
UPDATE users
SET email = ?, name = ?, role = ?
WHERE id = ?;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = ?
WHERE id = ?;

-- name: UpdateUserAvatar :exec
UPDATE users
SET avatar_path = ?
WHERE id = ?;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: ListAllUsers :many
SELECT id, email, password_hash, name, role, created_at, updated_at, avatar_path
FROM users
ORDER BY name ASC;
