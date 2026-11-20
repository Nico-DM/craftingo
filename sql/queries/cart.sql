-- name: ListCartLines :many
SELECT *
FROM craft_cart_lines
WHERE owner_id = sqlc.arg(owner_id)
ORDER BY position, created_at;

-- name: CreateCartLine :one
INSERT INTO craft_cart_lines (id, owner_id, item_id, position, preferred_trait, created_at)
VALUES (sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(item_id), sqlc.arg(position), sqlc.arg(preferred_trait), sqlc.arg(created_at))
RETURNING *;

-- name: SetCartLineTrait :exec
UPDATE craft_cart_lines
SET preferred_trait = sqlc.arg(preferred_trait)
WHERE id = sqlc.arg(id) AND owner_id = sqlc.arg(owner_id);

-- name: DeleteCartLine :exec
DELETE FROM craft_cart_lines
WHERE id = sqlc.arg(id) AND owner_id = sqlc.arg(owner_id);

-- name: ClearCart :exec
DELETE FROM craft_cart_lines
WHERE owner_id = sqlc.arg(owner_id);
