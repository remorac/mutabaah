-- One row per (task, user, due_date) when a scheduled occurrence is completed.
CREATE TABLE task_completions (
    id           BIGINT    NOT NULL AUTO_INCREMENT,
    task_id      BIGINT    NOT NULL,
    user_id      BIGINT    NOT NULL,
    due_date     DATE      NOT NULL,
    completed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_task_completions_task_user_date (task_id, user_id, due_date),
    KEY idx_task_completions_user_date (user_id, due_date),
    CONSTRAINT fk_task_completions_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT fk_task_completions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
