-- Task templates. Tasks are admin-defined and globally visible to every user.
CREATE TABLE tasks (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    title       VARCHAR(255) NOT NULL,
    description TEXT         NULL,
    category    VARCHAR(64)  NULL,
    frequency   ENUM('daily','weekly','monthly','yearly') NOT NULL,
    start_date  DATE         NOT NULL,
    end_date    DATE         NULL,
    active      BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_tasks_active (active),
    KEY idx_tasks_frequency (frequency)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
