-- +goose Up
ALTER TABLE USERS ADD COLUMN hashed_password TEXT DEFAULT 'unset';

-- +goose Down
ALTER TABLE USERS DROP COLUMN hashed_password;