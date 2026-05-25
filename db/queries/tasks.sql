-- name: CreateTask :execlastid
INSERT INTO tasks (title, description, category, frequency, start_date, end_date, active, sequence)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetTask :one
SELECT id, title, description, category, frequency, start_date, end_date, active, created_at, updated_at, sequence
FROM tasks
WHERE id = ?;

-- name: ListTasks :many
SELECT id, title, description, category, frequency, start_date, end_date, active, created_at, updated_at, sequence
FROM tasks
ORDER BY sequence ASC, created_at DESC;

-- name: ListActiveTasks :many
SELECT id, title, description, category, frequency, start_date, end_date, active, created_at, updated_at, sequence
FROM tasks
WHERE active = TRUE
ORDER BY sequence ASC, created_at DESC;

-- name: UpdateTask :exec
UPDATE tasks
SET title = ?, description = ?, category = ?, frequency = ?, start_date = ?, end_date = ?, active = ?, sequence = ?
WHERE id = ?;

-- name: SetTaskActive :exec
UPDATE tasks SET active = ? WHERE id = ?;

-- name: SetTaskSequence :exec
UPDATE tasks SET sequence = ? WHERE id = ?;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = ?;

-- name: ListTaskCategories :many
SELECT DISTINCT category
FROM tasks
WHERE category IS NOT NULL AND category <> ''
ORDER BY category ASC;
