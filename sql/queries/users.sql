-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email)
VALUES (
    DEFAULT,
    DEFAULT,
    DEFAULT,
    $1
)
RETURNING *;

-- name: DeleteAllUsers :exec
DELETE FROM users;