-- +goose Up
-- A craft in progress. Lines are ordered; each line's output is available to
-- later lines, and nothing is removed from the inventory until the whole cart
-- is finalized.
CREATE TABLE craft_cart_lines (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    item_id    TEXT NOT NULL,
    position   INTEGER NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_cart_owner ON craft_cart_lines (owner_id, position);

-- +goose Down
DROP TABLE craft_cart_lines;
