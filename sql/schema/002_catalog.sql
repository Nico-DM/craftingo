-- +goose Up
-- Catalog: items, their canonical recipe tree, and 5e.tools stats denormalized
-- onto each item. Scraped data is a build input, never runtime data.

CREATE TABLE items (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    category          TEXT NOT NULL,       -- Weapon, Armor, Wondrous Item, Potion, ...
    subtype           TEXT,                -- Martial Melee, Tattoo, Instrument, ...
    type_raw          TEXT NOT NULL,       -- original "Weapon, Martial Melee"
    rarity            TEXT NOT NULL,       -- Common, Uncommon, Rare, Very Rare, Legendary
    magic             INTEGER NOT NULL DEFAULT 0 CHECK (magic IN (0, 1)),
    oota              INTEGER NOT NULL DEFAULT 0 CHECK (oota IN (0, 1)),
    source            TEXT NOT NULL,       -- book title, or Crafting / Combat Module
    short_description TEXT,

    -- Variants (e.g. "Dark Shard Amulet, Archfey") are first-class items with
    -- their own recipe, grouped under the scraped base name.
    variant_of        TEXT,
    variant_label     TEXT,

    attunement        TEXT,
    damage            TEXT,
    properties        TEXT,
    weight            TEXT,
    value             TEXT,
    description       TEXT,
    scraped_name      TEXT,
    stat_source_code  TEXT,
    stat_source       TEXT,

    match_status      TEXT NOT NULL DEFAULT 'unmatched'
                      CHECK (match_status IN ('matched', 'unmatched', 'needs_review')),
    match_method      TEXT,
    match_detail      TEXT,
    match_confidence  REAL,

    -- JSON arrays. Intrinsic tags come from the item name/type; archetype
    -- describes generated combos (tier templates and named modifier bases).
    intrinsic_tags       TEXT NOT NULL DEFAULT '[]',
    archetype_kind       TEXT CHECK (archetype_kind IN ('tier', 'modifier')),
    archetype_base_kind  TEXT CHECK (archetype_base_kind IN ('category', 'item')),
    archetype_base_name  TEXT,
    archetype_tier       INTEGER,
    is_template          INTEGER NOT NULL DEFAULT 0 CHECK (is_template IN (0, 1)),

    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);

CREATE INDEX idx_items_category ON items (category);
CREATE INDEX idx_items_rarity   ON items (rarity);
CREATE INDEX idx_items_source   ON items (source);
CREATE INDEX idx_items_variant  ON items (variant_of);
CREATE INDEX idx_items_match    ON items (match_status);
CREATE INDEX idx_items_archetype ON items (archetype_kind);

-- One canonical recipe per item. `inherit` recipes (the generic Weapon/Armor
-- +N templates) have no groups; they select a base category instead.
CREATE TABLE recipes (
    id        INTEGER PRIMARY KEY,
    item_id   TEXT NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    position  INTEGER NOT NULL DEFAULT 1,
    kind      TEXT NOT NULL CHECK (kind IN ('groups', 'inherit')),
    rarity    TEXT NOT NULL,
    base_kind TEXT CHECK (base_kind IN ('category', 'item')),
    base_name TEXT,
    notes     TEXT NOT NULL DEFAULT '[]',
    UNIQUE (item_id, position)
);

CREATE INDEX idx_recipes_item ON recipes (item_id);

CREATE TABLE recipe_groups (
    id           INTEGER PRIMARY KEY,
    recipe_id    INTEGER NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    position     INTEGER NOT NULL,
    quantity     INTEGER NOT NULL CHECK (quantity > 0),
    quantity_max INTEGER,
    min_rarity   TEXT,                    -- C/UC/R/VR/L
    trait_mode   TEXT NOT NULL DEFAULT 'fixed' CHECK (trait_mode IN ('fixed', 'variable')),
    trait_var    INTEGER,                 -- source group for a variable trait
    UNIQUE (recipe_id, position)
);

CREATE INDEX idx_groups_recipe ON recipe_groups (recipe_id);

CREATE TABLE recipe_options (
    id              INTEGER PRIMARY KEY,
    group_id        INTEGER NOT NULL REFERENCES recipe_groups (id) ON DELETE CASCADE,
    position        INTEGER NOT NULL,
    any_raw         INTEGER NOT NULL DEFAULT 0 CHECK (any_raw IN (0, 1)),
    tier            INTEGER,
    forms           TEXT NOT NULL DEFAULT '[]',  -- JSON [{kind,name,id}]
    traits          TEXT NOT NULL DEFAULT '[]',  -- JSON ["Vital", ...]
    tags_all        TEXT NOT NULL DEFAULT '[]',  -- JSON
    tags_any        TEXT NOT NULL DEFAULT '[]',  -- JSON
    output_tags     TEXT NOT NULL DEFAULT '[]',  -- JSON
    output_tags_any TEXT NOT NULL DEFAULT '[]',  -- JSON
    UNIQUE (group_id, position)
);

CREATE INDEX idx_options_group ON recipe_options (group_id);

CREATE TABLE recipe_extras (
    id         INTEGER PRIMARY KEY,
    recipe_id  INTEGER NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('currency', 'spell', 'narrative')),
    quantity   INTEGER,
    raw        TEXT NOT NULL
);

CREATE INDEX idx_extras_recipe ON recipe_extras (recipe_id);

-- +goose Down
DROP TABLE recipe_extras;
DROP TABLE recipe_options;
DROP TABLE recipe_groups;
DROP TABLE recipes;
DROP TABLE items;
