-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password, is_chirpy_red)
VALUES (
    DEFAULT,
    DEFAULT,
    DEFAULT,
    $1,
    $2,
    DEFAULT
)
RETURNING id, created_at, updated_at, email, is_chirpy_red;

-- name: DeleteAllUsers :exec
DELETE FROM users;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1;

-- name: UpdateUserEmailAndPassword :one
UPDATE users
SET email = $2, hashed_password = $3, updated_at = NOW()
WHERE id = $1
RETURNING id, created_at, updated_at, email, is_chirpy_red;

-- name: UpgradeUserToChirpyRed :one
UPDATE users
SET is_chirpy_red = TRUE, updated_at = NOW()
WHERE id = $1
RETURNING id, created_at, updated_at, email, is_chirpy_red;