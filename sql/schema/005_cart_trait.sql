-- +goose Up
-- A cart line can carry several traits with the same total. The crafter picks
-- which one the finished item keeps; an empty value keeps the planner default.
ALTER TABLE craft_cart_lines ADD COLUMN preferred_trait TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE craft_cart_lines DROP COLUMN preferred_trait;
