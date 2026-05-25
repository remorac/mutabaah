ALTER TABLE tasks CHANGE COLUMN description category VARCHAR(64) NULL;
ALTER TABLE tasks ADD COLUMN description TEXT NULL AFTER title;
