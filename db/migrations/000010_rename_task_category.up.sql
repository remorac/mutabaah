ALTER TABLE tasks DROP COLUMN description;
ALTER TABLE tasks CHANGE COLUMN category description VARCHAR(64) NULL AFTER title;
