# Recipe JSON spec

`scripts/build_dataset.py` emits one JSON object per item. `cmd/seed` stores the
item and its recipe tree in SQLite/Turso. The same recipe JSON is what the DM
editor accepts on `/admin/items/{id}`.

## Item

```json
{
  "id": "flask-or-tankard",
  "name": "Flask or Tankard",
  "category": "Gear",
  "subtype": null,
  "type_raw": "Gear",
  "rarity": "Common",
  "magic": false,
  "source": "Crafting",
  "intrinsic_tags": [],
  "archetype": null,
  "template": false,
  "recipe": { "...": "see below" }
}
```

`archetype` marks generated rows:

```json
{"kind": "tier", "base": {"kind": "category", "name": "Weapon"}, "tier": 1}
{"kind": "modifier", "base": {"kind": "category", "name": "Weapon"}, "tier": 2}
```

`template` is true for the generic `Weapon +N` / `Armor +N` rows and for
`Vicious Weapon`; templates are pickers, not ordinary craftable catalog rows.

## Recipe

```json
{
  "kind": "groups",
  "rarity": "Common",
  "extras": [],
  "groups": [
    {
      "quantity": 1,
      "quantity_max": null,
      "min_rarity": null,
      "trait_mode": "fixed",
      "trait_var": null,
      "options": [
        {"forms": [{"kind": "raw", "name": "Metal"}]}
      ]
    },
    {
      "quantity": 2,
      "quantity_max": null,
      "min_rarity": null,
      "trait_mode": "fixed",
      "trait_var": null,
      "options": [
        {"forms": [{"kind": "raw", "name": "Glass"}]},
        {"forms": [{"kind": "raw", "name": "Hide"}]},
        {"forms": [{"kind": "raw", "name": "Bone"}]},
        {"forms": [{"kind": "raw", "name": "Metal"}]}
      ]
    }
  ],
  "notes": []
}
```

`kind` is `groups` or `inherit`. `inherit` recipes have a `base_selector`
instead of groups and mean "same materials as this base category/item".

Every group asks for `quantity` units (or a range via `quantity_max`). Options
are homogeneous alternatives: **every** unit in a group must satisfy the same
option. A unit counts toward **every** group it qualifies for; feasibility is a
per-group count, not an allocation problem.

### Option

```json
{
  "any_raw": false,
  "forms": [{"kind": "raw", "name": "Metal"}],
  "traits": ["Vital"],
  "tags_all": ["Dragon"],
  "tags_any": [],
  "output_tags": ["Metal"],
  "output_tags_any": [],
  "tier": null
}
```

- `any_raw` (`Material`) matches any of the nine raw forms.
- `forms[].kind` is `raw`, `item`, or `generic`.
  - `raw` names one of Bone, Fluid, Food, Glass, Hide, Metal, Mineral,
    Textile, Wood.
  - `item` names a catalog item and keeps its `id`; it is also a sub-craft.
  - `generic` is one of Weapon, Armor, Ammunition, Focus, Instrument. It
    matches a crafted unit whose catalog category is that value.
- `tier` on an option requires a crafted base at least that tier (`+2 Weapon`).
- `traits` are fixed trait requirements. `Trait (Dragon)` is a raw/tag
  requirement, not a form.
- `tags_all` must all be present on the unit; one of `tags_any` must be present
  when the list is non-empty. Tags are lowercase-canonical in the build
  (`Wood`, `Hide`, `Mineral`, `Glass`, `Dragon`, `Bug`, `Sword`, ...).
- `output_tags` / `output_tags_any` are inherited by the crafted item when it
  is used as a material later (Staff, Shield, Flask, dragon vessels, ...).

### Variable trait groups

```json
{
  "quantity": 7,
  "trait_mode": "variable",
  "trait_var": 1,
  "options": [{}]
}
```

This is `7x [Trait] (of the same trait)`. The trait is chosen by the material in
group `trait_var` (1-based). Only that source group picks the trait; the seven
units may be any units carrying the same trait. A dragon material always has a
trait by dragon type, so the group is never unbound.

### Extras

```json
{"kind": "currency", "quantity": 1000, "raw": "1000 GP"}
{"kind": "spell", "raw": "Cast the ... spell"}
{"kind": "narrative", "raw": "..."}
```

Extras are displayed but do not participate in material counting.

## Matching and quantity rules

- Rarity: a unit must be at least the group's `min_rarity` (`C`, `UC`, `R`,
  `VR`, `L`). The crafted output rarity is `max(recipe rarity, lowest rarity
  among consumed units)`.
- Trait magnitude is stored on the inventory unit (`trait_qty`) and is never
  changed by recipes or the calculator. The crafted item carries the most
  prominent non-Basic trait (sum of `trait_qty`); ties are the crafter's choice.
- Overflow units still contribute their full `trait_qty`.
- A raw unit is matched by form + trait + tags + rarity. A crafted unit is
  matched by catalog item id (or generic category), tier, tags and trait.

## Generated combos

Tier and modifier rows are not flattened. The generator composes a page from a
base item:

- `Weapon +1` uses the base recipe unchanged and adds the tier label.
- `Vicious Weapon` replaces its `+2 Weapon` requirement with the chosen base
  (for example `+2 Greataxe`), so the base is itself a sub-craft.

Each generated combo is ephemeral: it is not stored as a new catalog row.
Crafted inventory instances refer to the base catalog item and store their own
tier, trait, trait quantity and tags.

## Admin editing

The DM recipe editor accepts this JSON with `DisallowUnknownFields`. Item forms
may omit `id` when `name` matches a catalog item; the server resolves it.
Saving replaces the recipe in a transaction, so group/option rows from the old
recipe cannot leak into the new one.
