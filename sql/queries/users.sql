-- name: CreateUser :one
INSERT INTO users (id, username, password_hash, role, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(username), sqlc.arg(password_hash), sqlc.arg(role),
        sqlc.arg(created_at), sqlc.arg(updated_at))
RETURNING *;

-- name: GetUserByUsername :one
SELECT *
FROM users
WHERE username = sqlc.arg(username);

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = sqlc.arg(id);

-- name: ListUsers :many
SELECT *
FROM users
ORDER BY username;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = sqlc.arg(password_hash), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = sqlc.arg(id);

-- name: CreateSession :exec
INSERT INTO sessions (token, user_id, expires_at, created_at)
VALUES (sqlc.arg(token), sqlc.arg(user_id), sqlc.arg(expires_at), sqlc.arg(created_at));

-- name: GetSessionUser :one
SELECT u.*
FROM users u
JOIN sessions s ON s.user_id = u.id
WHERE s.token = sqlc.arg(token)
  AND s.expires_at > sqlc.arg(now);

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token = sqlc.arg(token);

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= sqlc.arg(now);
