CREATE TABLE app_settings (
    id             TINYINT   NOT NULL,
    week_start_day TINYINT   NOT NULL DEFAULT 6,
    history_weeks  TINYINT   NOT NULL DEFAULT 1,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT chk_app_settings_singleton CHECK (id = 1),
    CONSTRAINT chk_app_settings_week_start_day CHECK (week_start_day BETWEEN 0 AND 6),
    CONSTRAINT chk_app_settings_history_weeks CHECK (history_weeks BETWEEN 1 AND 4)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO app_settings (id, week_start_day, history_weeks)
VALUES (1, 6, 1);
