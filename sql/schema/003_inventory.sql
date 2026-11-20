-- +goose Up
-- One row per stack of identical raw material, or per individual crafted item.
-- Crafted items are always quantity 1 and keep their own trait/tag snapshot.
CREATE TABLE inventory_units (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('material', 'crafted')),
    form_kind  TEXT NOT NULL CHECK (form_kind IN ('raw', 'item')),
    form_name  TEXT NOT NULL,
    item_id    TEXT REFERENCES items (id) ON DELETE SET NULL,
    name       TEXT NOT NULL,
    rarity     TEXT NOT NULL,
    trait      TEXT,
    trait_qty  INTEGER NOT NULL DEFAULT 0 CHECK (trait_qty >= 0),
    tier       INTEGER NOT NULL DEFAULT 0 CHECK (tier >= 0),
    tags       TEXT NOT NULL DEFAULT '[]',
    quantity   INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0),
    note       TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_inventory_owner ON inventory_units (owner_id);
CREATE INDEX idx_inventory_item ON inventory_units (item_id);
CREATE INDEX idx_inventory_form ON inventory_units (form_kind, form_name);

-- +goose Down
DROP TABLE inventory_units;
