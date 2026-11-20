-- +goose Up
-- Remember whether a crafted unit is magical. Names alone cannot tell: a
-- Vicious Weapon Greataxe shares its base item id with a mundane Greataxe.
ALTER TABLE inventory_units ADD COLUMN magic INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE inventory_units DROP COLUMN magic;
