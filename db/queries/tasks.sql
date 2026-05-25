-- name: CreateTask :execlastid
INSERT INTO tasks (title, description, category, frequency, start_date, end_date, active)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetTask :one
SELECT id, title, description, category, frequency, start_date, end_date, active, created_at, updated_at
FROM tasks
WHERE id = ?;

-- name: ListTasks :many
SELECT id, title, description, category, frequency, start_date, end_date, active, created_at, updated_at
FROM tasks
ORDER BY created_at DESC;

-- name: ListActiveTasks :many
SELECT id, title, description, category, frequency, start_date, end_date, active, created_at, updated_at
FROM tasks
WHERE active = TRUE
ORDER BY created_at DESC;

-- name: UpdateTask :exec
UPDATE tasks
SET title = ?, description = ?, category = ?, frequency = ?, start_date = ?, end_date = ?, active = ?
WHERE id = ?;

-- name: SetTaskActive :exec
UPDATE tasks SET active = ? WHERE id = ?;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = ?;

-- name: ListTaskCategories :many
SELECT DISTINCT category
FROM tasks
WHERE category IS NOT NULL AND category <> ''
ORDER BY category ASC;
