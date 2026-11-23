-- name: GetItem :one
SELECT *
FROM items
WHERE id = sqlc.arg(id);

-- name: GetItemByName :one
SELECT *
FROM items
WHERE name = sqlc.arg(name);

-- name: ListItems :many
SELECT *
FROM items
WHERE (sqlc.narg(q) IS NULL OR name LIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(category) IS NULL OR category = sqlc.narg(category))
  AND (sqlc.narg(rarity) IS NULL OR rarity = sqlc.narg(rarity))
  AND (sqlc.narg(source) IS NULL OR source = sqlc.narg(source))
  AND (sqlc.narg(magic) IS NULL OR magic = sqlc.narg(magic))
ORDER BY name
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- name: CountItems :one
SELECT COUNT(*)
FROM items
WHERE (sqlc.narg(q) IS NULL OR name LIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(category) IS NULL OR category = sqlc.narg(category))
  AND (sqlc.narg(rarity) IS NULL OR rarity = sqlc.narg(rarity))
  AND (sqlc.narg(source) IS NULL OR source = sqlc.narg(source))
  AND (sqlc.narg(magic) IS NULL OR magic = sqlc.narg(magic));

-- name: ListVariants :many
SELECT *
FROM items
WHERE variant_of = sqlc.arg(variant_of)
ORDER BY name;

-- name: ListBaseCandidates :many
SELECT *
FROM items
WHERE category = sqlc.arg(category)
  AND is_template = 0
ORDER BY name;

-- name: ListCategories :many
SELECT DISTINCT category
FROM items
ORDER BY category;

-- name: ListRarities :many
SELECT DISTINCT rarity
FROM items
ORDER BY rarity;

-- name: ListSources :many
SELECT DISTINCT source
FROM items
ORDER BY source;

-- name: GetRecipeForItem :one
SELECT *
FROM recipes
WHERE item_id = sqlc.arg(item_id)
ORDER BY position
LIMIT 1;

-- name: ListRecipeGroups :many
SELECT *
FROM recipe_groups
WHERE recipe_id = sqlc.arg(recipe_id)
ORDER BY position;

-- name: ListRecipeOptions :many
SELECT *
FROM recipe_options
WHERE group_id = sqlc.arg(group_id)
ORDER BY position;

-- name: ListRecipeExtras :many
SELECT *
FROM recipe_extras
WHERE recipe_id = sqlc.arg(recipe_id)
ORDER BY position;

-- name: UpsertItem :exec
INSERT INTO items (
    id, name, category, subtype, type_raw, rarity, magic, visible, source,
    short_description, variant_of, variant_label, attunement, damage, properties,
    weight, value, description, scraped_name, stat_source_code, stat_source,
    match_status, match_method, match_detail, match_confidence, intrinsic_tags,
    archetype_kind, archetype_base_kind, archetype_base_name, archetype_tier,
    is_template, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(name), sqlc.arg(category), sqlc.narg(subtype),
    sqlc.arg(type_raw), sqlc.arg(rarity), sqlc.arg(magic), sqlc.arg(visible),
    sqlc.arg(source), sqlc.narg(short_description), sqlc.narg(variant_of),
    sqlc.narg(variant_label), sqlc.narg(attunement), sqlc.narg(damage),
    sqlc.narg(properties), sqlc.narg(weight), sqlc.narg(value),
    sqlc.narg(description), sqlc.narg(scraped_name), sqlc.narg(stat_source_code),
    sqlc.narg(stat_source), sqlc.arg(match_status), sqlc.narg(match_method),
    sqlc.narg(match_detail), sqlc.narg(match_confidence), sqlc.arg(intrinsic_tags),
    sqlc.narg(archetype_kind), sqlc.narg(archetype_base_kind),
    sqlc.narg(archetype_base_name), sqlc.narg(archetype_tier), sqlc.arg(is_template),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE SET
    name                 = excluded.name,
    category             = excluded.category,
    subtype              = excluded.subtype,
    type_raw             = excluded.type_raw,
    rarity               = excluded.rarity,
    magic                = excluded.magic,
    -- visible is intentionally not updated: DM visibility edits survive a
    -- reseed. New rows still take the dataset value above.
    source               = excluded.source,
    short_description    = excluded.short_description,
    variant_of           = excluded.variant_of,
    variant_label        = excluded.variant_label,
    attunement           = excluded.attunement,
    damage               = excluded.damage,
    properties           = excluded.properties,
    weight               = excluded.weight,
    value                = excluded.value,
    description          = excluded.description,
    scraped_name         = excluded.scraped_name,
    stat_source_code     = excluded.stat_source_code,
    stat_source          = excluded.stat_source,
    match_status         = excluded.match_status,
    match_method         = excluded.match_method,
    match_detail         = excluded.match_detail,
    match_confidence     = excluded.match_confidence,
    intrinsic_tags       = excluded.intrinsic_tags,
    archetype_kind       = excluded.archetype_kind,
    archetype_base_kind  = excluded.archetype_base_kind,
    archetype_base_name  = excluded.archetype_base_name,
    archetype_tier       = excluded.archetype_tier,
    is_template          = excluded.is_template,
    updated_at           = excluded.updated_at;

-- name: UpdateItemMeta :exec
UPDATE items
SET name = sqlc.arg(name),
    category = sqlc.arg(category),
    subtype = sqlc.narg(subtype),
    type_raw = sqlc.arg(type_raw),
    rarity = sqlc.arg(rarity),
    magic = sqlc.arg(magic),
    visible = sqlc.arg(visible),
    source = sqlc.arg(source),
    short_description = sqlc.narg(short_description),
    variant_of = sqlc.narg(variant_of),
    variant_label = sqlc.narg(variant_label),
    attunement = sqlc.narg(attunement),
    damage = sqlc.narg(damage),
    properties = sqlc.narg(properties),
    weight = sqlc.narg(weight),
    value = sqlc.narg(value),
    description = sqlc.narg(description),
    intrinsic_tags = sqlc.arg(intrinsic_tags),
    archetype_kind = sqlc.narg(archetype_kind),
    archetype_base_kind = sqlc.narg(archetype_base_kind),
    archetype_base_name = sqlc.narg(archetype_base_name),
    archetype_tier = sqlc.narg(archetype_tier),
    is_template = sqlc.arg(is_template),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteRecipeForItem :exec
DELETE FROM recipes
WHERE item_id = sqlc.arg(item_id);

-- name: InsertRecipe :one
INSERT INTO recipes (
    item_id, position, kind, rarity, base_kind, base_name, notes
) VALUES (
    sqlc.arg(item_id), sqlc.arg(position), sqlc.arg(kind), sqlc.arg(rarity),
    sqlc.narg(base_kind), sqlc.narg(base_name), sqlc.arg(notes)
)
RETURNING id;

-- name: InsertRecipeGroup :one
INSERT INTO recipe_groups (
    recipe_id, position, quantity, quantity_max, min_rarity, trait_mode, trait_var
) VALUES (
    sqlc.arg(recipe_id), sqlc.arg(position), sqlc.arg(quantity),
    sqlc.narg(quantity_max), sqlc.narg(min_rarity), sqlc.arg(trait_mode),
    sqlc.narg(trait_var)
)
RETURNING id;

-- name: InsertRecipeOption :exec
INSERT INTO recipe_options (
    group_id, position, any_raw, tier, forms, traits, tags_all, tags_any,
    output_tags, output_tags_any
) VALUES (
    sqlc.arg(group_id), sqlc.arg(position), sqlc.arg(any_raw), sqlc.narg(tier),
    sqlc.arg(forms), sqlc.arg(traits), sqlc.arg(tags_all), sqlc.arg(tags_any),
    sqlc.arg(output_tags), sqlc.arg(output_tags_any)
);

-- name: InsertRecipeExtra :exec
INSERT INTO recipe_extras (recipe_id, position, kind, quantity, raw)
VALUES (sqlc.arg(recipe_id), sqlc.arg(position), sqlc.arg(kind), sqlc.narg(quantity), sqlc.arg(raw));

-- name: CountCatalog :one
SELECT COUNT(*) FROM items;

-- name: CountRecipes :one
SELECT COUNT(*) FROM recipes;

-- name: UpdateItemVisible :exec
UPDATE items
SET visible = sqlc.arg(visible),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);
