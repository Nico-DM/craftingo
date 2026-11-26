-- +goose Up
-- One craft can produce a batch ("Arrows +3 (20)", "Bead of Force (7)").
-- Existing recipes produce exactly one unit.
ALTER TABLE recipes ADD COLUMN output_quantity INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE recipes DROP COLUMN output_quantity;
