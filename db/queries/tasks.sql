-- name: CreateTask :execlastid
INSERT INTO tasks (title, frequency, start_date, end_date, active, sequence, description)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetTask :one
SELECT id, title, frequency, start_date, end_date, active, created_at, updated_at, sequence, description
FROM tasks
WHERE id = ?;

-- name: ListTasks :many
SELECT id, title, frequency, start_date, end_date, active, created_at, updated_at, sequence, description
FROM tasks
ORDER BY sequence ASC, created_at DESC;

-- name: ListActiveTasks :many
SELECT id, title, frequency, start_date, end_date, active, created_at, updated_at, sequence, description
FROM tasks
WHERE active = TRUE
ORDER BY sequence ASC, created_at DESC;

-- name: UpdateTask :exec
UPDATE tasks
SET title = ?, frequency = ?, start_date = ?, end_date = ?, active = ?, sequence = ?, description = ?
WHERE id = ?;

-- name: SetTaskActive :exec
UPDATE tasks SET active = ? WHERE id = ?;

-- name: SetTaskSequence :exec
UPDATE tasks SET sequence = ? WHERE id = ?;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = ?;
