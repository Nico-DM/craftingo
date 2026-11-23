-- +goose Up
-- Repurpose the old scraped "oota" flag as DM-controlled visibility. Players
-- only see catalog items where visible = 1; DMs always see everything.
ALTER TABLE items RENAME COLUMN oota TO visible;

-- Generated tier templates (Weapon/Armor/Ammunition +N) stay visible by
-- default so players can find and use them.
UPDATE items SET visible = 1 WHERE is_template = 1;

-- +goose Down
ALTER TABLE items RENAME COLUMN visible TO oota;
