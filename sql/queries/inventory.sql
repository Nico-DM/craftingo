-- name: ListInventory :many
SELECT *
FROM inventory_units
WHERE owner_id = sqlc.arg(owner_id)
ORDER BY kind, name, created_at;

-- name: GetInventoryUnit :one
SELECT *
FROM inventory_units
WHERE id = sqlc.arg(id) AND owner_id = sqlc.arg(owner_id);

-- name: CreateInventoryUnit :one
INSERT INTO inventory_units (
    id, owner_id, kind, form_kind, form_name, item_id, name, rarity, trait,
    trait_qty, tier, magic, tags, quantity, note, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(owner_id), sqlc.arg(kind), sqlc.arg(form_kind),
    sqlc.arg(form_name), sqlc.narg(item_id), sqlc.arg(name), sqlc.arg(rarity),
    sqlc.narg(trait), sqlc.arg(trait_qty), sqlc.arg(tier), sqlc.arg(magic), sqlc.arg(tags),
    sqlc.arg(quantity), sqlc.narg(note), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: UpdateInventoryQuantity :exec
UPDATE inventory_units
SET quantity = sqlc.arg(quantity), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND owner_id = sqlc.arg(owner_id);

-- name: UpdateInventoryNote :exec
UPDATE inventory_units
SET note = sqlc.narg(note), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND owner_id = sqlc.arg(owner_id);

-- name: DeleteInventoryUnit :exec
DELETE FROM inventory_units
WHERE id = sqlc.arg(id) AND owner_id = sqlc.arg(owner_id);

-- name: UpdateInventoryUnit :exec
UPDATE inventory_units
SET name = sqlc.arg(name),
    rarity = sqlc.arg(rarity),
    trait = sqlc.narg(trait),
    trait_qty = sqlc.arg(trait_qty),
    tier = sqlc.arg(tier),
    magic = sqlc.arg(magic),
    tags = sqlc.arg(tags),
    quantity = sqlc.arg(quantity),
    note = sqlc.narg(note),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND owner_id = sqlc.arg(owner_id);
