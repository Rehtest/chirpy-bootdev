-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
    DEFAULT,
    DEFAULT,
    DEFAULT,
    $1,
    $2
)
RETURNING *;

-- name: GetChirpsByAscendingCreatedAt :many
SELECT * FROM chirps
ORDER BY created_at ASC;

-- name: GetChirpByID :one
SELECT * FROM chirps
WHERE id = $1;

-- name: DeleteChirp :exec
DELETE FROM chirps
WHERE id = $1;