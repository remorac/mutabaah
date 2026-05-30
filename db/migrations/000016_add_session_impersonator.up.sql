ALTER TABLE sessions
    ADD COLUMN impersonator_user_id BIGINT NULL AFTER user_id,
    ADD KEY idx_sessions_impersonator (impersonator_user_id),
    ADD CONSTRAINT fk_sessions_impersonator_user FOREIGN KEY (impersonator_user_id) REFERENCES users(id) ON DELETE CASCADE;
