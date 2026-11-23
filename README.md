# Craftingo

A D&D 5e crafting catalog and calculator. It normalizes a homebrew crafting
dataset into one canonical recipe per item, serves a public catalog, and adds
accounts, inventory, a can-craft calculator, a DM editor, and a modular
generator for tiered/modifier items.

Stack: Go, chi, sqlc, SQLite (local) or Turso (libsql).

## Quick start

```bash
# 1. Build the normalized dataset
python3 scripts/build_dataset.py

# 2. Create the database schema
goose -dir sql/schema sqlite3 craftingo.db up

# 3. Load items and recipes
DATABASE_URL=file:craftingo.db go run ./cmd/seed

# 4. Create the first DM account (there is no public signup)
DATABASE_URL=file:craftingo.db go run ./cmd/createuser \
  -username dm -password secret1 -role dm

# 5. Run
PORT=8080 DATABASE_URL=file:craftingo.db go run .
```

Open <http://localhost:8080>. The catalog is public; inventory and the
calculator require a login; `/admin` requires the `dm` role.

### Environment

```bash
PORT=8080
DATABASE_URL=file:craftingo.db
```

For Turso use `DATABASE_URL=libsql://...?authToken=...`. Migrations:

```bash
goose -dir sql/schema turso "$DATABASE_URL" up
```

### Turso + Cloud Run

Cloud Run only receives the Go binary; `data/build/` is generated and gitignored.
The deploy workflow runs migrations, but it does **not** seed the catalog. After
pulling dataset or schema changes, run this from a checkout with a generated
`data/build/`:

```bash
# 1. Rebuild the normalized dataset (and review the report)
python3 scripts/build_dataset.py

# 2. Migrate Turso. Set DATABASE_URL explicitly; an explicit value now wins
#    over .env in scripts/migrateup.sh.
DATABASE_URL='libsql://...?authToken=...' scripts/migrateup.sh

# 3. Seed items and recipes. Safe to rerun: rows are upserted, and DM
#    visibility edits are preserved.
DATABASE_URL='libsql://...?authToken=...' go run ./cmd/seed

# 4. Create the first DM if the database has no accounts
DATABASE_URL='libsql://...?authToken=...' go run ./cmd/createuser \
  -username dm -password '...' -role dm
```

Then deploy by pushing to `main`. The Cloud Run service must use the same
`DATABASE_URL` secret. Migration `007_visible.sql` renames the old `oota` column
to `visible`; if the remote database has never been migrated, the app will fail
at startup with `no such table: items` until steps 2 and 3 are done.

## Commands

```bash
go run ./cmd/seed -dataset data/build/items.jsonl   # rebuild catalog rows
go run ./cmd/createuser -username ... -password ... -role dm|player
go test ./...
sqlc generate                                        # after changing sql/queries
```

`data/build/` is generated and gitignored. `data/items.tsv` is the single source
of truth for items, recipes and stats; do not hand-edit `data/build`.

## Features

- **Public catalog**: search/filter by name, category, rarity, source and magic;
  grouped item pages with recipe groups, variants, stats and reverse
  "used to craft" links. Players only see items marked **visible**; DMs see the
  full catalog.
- **Visibility**: DMs check or uncheck **Visible** directly in the edit-items
  list, or from an item's metadata form. Hidden items disappear from player
  catalog, generator, craft and inventory pickers.
- **Accounts**: DM and player roles, password + session cookie. DMs create
  accounts; there is no signup.
- **Inventory**: raw material stacks (form + rarity + optional trait + tags) and
  individual crafted items (tier, magic, trait, tags). Materials and crafted
  items have separate add forms, filters and sorting.
- **List ordering**: filters and sorting apply immediately. The combined
  **Default** order is mundane before magic, Common to Legendary, category with
  alphabetical subtype, then name. Every sort option has a **Reverse** toggle
  that flips the whole resulting list.
- **Crafting cart**: add craftable items as cart lines. Reserved materials are
  excluded from later lines, a line's output can feed later lines, removing a
  line returns its materials, and only **Finalize** consumes the inventory.
  When several traits tie for the output, the crafter picks the one it keeps.
- **Calculator**: can-craft result, per-group have/missing counts, bound dragon
  traits, and recursive "craft these first" suggestions. It is a feasibility
  check, not a purchase optimizer.
- **DM editor**: edit item metadata, intrinsic tags, archetypes and the recipe
  JSON, with validation and transactional replacement.
- **Generator**: tier templates (`Weapon +1` .. `+4`, `Armor +1` .. `+3`,
  `Ammunition +1` .. `+4`) and named modifiers composed over a chosen base.
  Nothing is flattened. `Vicious Weapon` is a named modifier, not a tier
  template.

## Recipe model

Full JSON shape and matching rules: [docs/recipe-spec.md](docs/recipe-spec.md).
Short version:

- A unit is rarity + optional trait + form (nine raw forms or an item) + tags.
- A recipe is a list of requirement groups with homogeneous options.
- A unit counts toward every group it qualifies for.
- Identical requirement groups collapse to the stricter quantity, not a sum.
- Rarity is a minimum, not a fixed tier.
- Trait quantities are balance values (3–8) and are never rewritten by the
  normalizer; only form choices were size-adjusted.

## Layout

```
cmd/seed            dataset -> database
cmd/createuser      account bootstrap
internal/catalog    item/recipe model, DB loading, recipe persistence
internal/crafting   matching + calculator
internal/auth       PBKDF2 password hashing
internal/database   sqlc output
sql/schema          goose migrations
sql/queries         sqlc queries
scripts/            dataset build script
templates/          server-rendered pages
```
