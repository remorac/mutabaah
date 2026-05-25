ALTER TABLE tasks
    DROP KEY idx_tasks_sequence,
    DROP COLUMN sequence;
