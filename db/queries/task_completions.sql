-- name: MarkTaskComplete :exec
INSERT INTO task_completions (task_id, user_id, due_date, completed_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE completed_at = CURRENT_TIMESTAMP;

-- name: MarkTaskIncomplete :exec
DELETE FROM task_completions
WHERE task_id = ? AND user_id = ? AND due_date = ?;

-- name: GetCompletion :one
SELECT id, task_id, user_id, due_date, completed_at, created_at
FROM task_completions
WHERE task_id = ? AND user_id = ? AND due_date = ?;

-- name: ListCompletionsForUserOnDate :many
SELECT id, task_id, user_id, due_date, completed_at, created_at
FROM task_completions
WHERE user_id = ? AND due_date = ?;

-- name: ListCompletionsForUserInRange :many
SELECT id, task_id, user_id, due_date, completed_at, created_at
FROM task_completions
WHERE user_id = ? AND due_date BETWEEN ? AND ?
ORDER BY due_date ASC;
