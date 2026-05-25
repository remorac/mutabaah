ALTER TABLE tasks
    ADD COLUMN sequence INT NOT NULL DEFAULT 0,
    ADD KEY idx_tasks_sequence (sequence);
