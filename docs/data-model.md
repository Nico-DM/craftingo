# Craftingo data model

The catalog is built from `data/items.tsv`. `scripts/build_dataset.py` normalizes it
into `data/build/items.jsonl`; `cmd/seed` loads that JSON into SQLite (local file or
Turso). The database is a derived artifact and can be rebuilt at any time.

## Units

A **unit** is one ingredient:

- a rarity: Common, Uncommon, Rare, Very Rare, Legendary
- an optional trait, or none (= Basic)
- a form: one of the nine raw forms or one item
- zero or more freeform tags
- a trait magnitude (`trait_qty`) for trait-bearing units

Raw forms are `Bone`, `Fluid`, `Food`, `Glass`, `Hide`, `Metal`, `Mineral`,
`Textile`, `Wood`. Legacy names are rewritten on import:

| Legacy      | Current |
| ----------- | ------- |
| Hardwood    | Wood    |
| Gem         | Mineral |
| Leather     | Hide    |
| Flesh       | Hide    |
| Foodstuff   | Food    |
| Plant       | Wood    |
| Spice       | Food    |
| Dye         | Fluid   |
| Earthenware | Glass   |
| Wax         | Fluid (tag `Bug`) |
| Feather     | Textile |

Tags are freeform but canonicalized (for example `Hardwood` -> `Wood`,
`Leather` -> `Hide`, `10-Foot` -> `10-foot`). A tag in `tags_all` is required;
one of `tags_any` is required when that list is non-empty.

## Recipes

An item has one recipe. A recipe is a list of **requirement groups**. Each group:

- asks for `quantity` units (optionally a minimum..maximum range)
- has one or more **options**; every unit in the group must satisfy the same option
  (homogeneous options)
- can require a minimum input rarity (`C`, `UC`, `R`, `VR`, `L`)
- is either a fixed group or a **variable-trait** group

An option can specify:

- `any_raw` (`Material` = any raw form)
- a tier (`+1`, `+2`, ...)
- one or more forms: raw form, item, or a generic category
  (`Weapon`, `Armor`, `Ammunition`, `Focus`, `Instrument`)
- required traits
- required/optional tags
- output tags inherited by the crafted item (only for material-like crafted items
  such as `Staff`, `Shield`, `Flask or Tankard`, dragon vessels, ...)

A unit counts toward **every** group it qualifies for. Feasibility is therefore a
per-group count, not an allocation problem. Identical groups collapse to the
stricter quantity instead of summing.

Alternatives written with `;` in the source are merged into one recipe: ingredients
shared by every alternative stay as groups, and the differing ingredient becomes
one group with one option per alternative. If alternatives differ in more than one
ingredient, the build reports a conflict instead of guessing.

`As X` inherit recipes are expanded from `X`; material qualifiers filter the
inherited options. Fully generic material sets such as `(Metal/Bone/Wood) Staff`
collapse to `Staff`.

## Rarity

- The recipe rarity defaults to the item rarity.
- A `C`/`UC`/`R`/`VR`/`L` prefix on an ingredient sets that group's minimum input
  rarity.
- The crafted item's rarity is `max(recipe rarity, lowest rarity among the consumed
  units)`.

## Traits

- A fixed trait group such as `3x Vital` requires units carrying `Vital`.
- `Nx [Trait] (of the same trait)` is a variable group bound to the group that
  contains a `(Dragon)` material. All N units must carry the same trait as the
  chosen dragon material.
- The crafted item carries the most prominent non-Basic trait among all consumed
  units. Prominence is the sum of `trait_qty`; ties are chosen by the crafter.
- Units consumed beyond the group requirement still contribute their full
  `trait_qty`.

## Archetypes

Two kinds are generated rather than stored as flat recipes:

- **tier**: `Weapon +1` .. `Weapon +4`, `Armor +1` .. `Armor +3` are templates over
  any base of that category.
- **modifier**: a named item whose recipe consumes a generic base, e.g.
  `Vicious Weapon` consumes a `+2 Weapon`, or `Flame Tongue (Sword)` consumes a
  `+2` sword. These are not flattened: a Vicious Greataxe is crafted from a
  `+2 Greataxe`.

Generated combos get a composed page; each crafted inventory instance gets its own
page.

## Non-material requirements

Spell requirements (`Cast the ...`), currency (`1000 GP`) and narrative requirements
are stored as recipe extras and displayed. They do not participate in material
counting.

## Database shape

- `items`: denormalized item + 5e.tools stat block, intrinsic tags, archetype.
- `recipes`, `recipe_groups`, `recipe_options`, `recipe_extras`: the canonical
  recipe tree. Forms/traits/tags are JSON arrays because they are read and written
  as a unit.
- `users`, `sessions`: DM/player accounts and cookie sessions.
- `inventory_units`: stacks of raw materials and individual crafted items.

## Build pipeline

```sh
python3 scripts/build_dataset.py
goose -dir sql/schema sqlite3 craftingo.db up
DATABASE_URL=file:craftingo.db go run ./cmd/seed
```

The build emits `data/build/items.jsonl`, `data/build/recipes.txt` (review aid) and
`data/build/report.md` (conflicts/warnings).
