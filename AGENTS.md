# AGENTS.md

Craftingo is a Go 1.27.1 single-binary D&D 5e crafting catalog. The root package
is the HTTP server; `cmd/` contains one-shot CLIs. There is no frontend build
step: pages are server-rendered `html/template` files with no `package.json`.

## Local setup

```bash
python3 scripts/build_dataset.py
goose -dir sql/schema sqlite3 craftingo.db up
DATABASE_URL=file:craftingo.db go run ./cmd/seed
DATABASE_URL=file:craftingo.db go run ./cmd/createuser -username dm -password secret1 -role dm
PORT=8080 DATABASE_URL=file:craftingo.db go run .
```

- `.env` is auto-loaded by the server and both CLIs; `scripts/migrateup.sh` sources it.
- The server does not run migrations. It fails at startup if the schema is
  missing; an empty migrated database simply serves an empty catalog.
- `cmd/seed` upserts items and replaces their recipes; it does not delete items
  removed from the dataset. Recreate the database for a clean rebuild.
- SQLite vs Turso: `catalog.OpenDB` chooses the driver from the URL (`file:` or a
  plain path uses SQLite; `libsql://` uses Turso). Migrations differ:
  `goose -dir sql/schema sqlite3 craftingo.db up` vs
  `goose -dir sql/schema turso "$DATABASE_URL" up`.

## Data pipeline

- `data/items.tsv` is the tracked source of truth (optional untracked
  `data/items_extra.tsv` is also read). Never hand-edit generated
  `data/build/*`; it is gitignored.
- `data/descriptions.md` holds only the Markdown descriptions the catalog uses.
  The full upstream dump (`data/descriptions.full.md`) is local and gitignored;
  `scripts/filter_descriptions.py` reduces it and reports items without a
  description. The build drops `***Base items.***` sections.
- `python3 scripts/build_dataset.py` emits `items.jsonl`, `recipes.txt`, and
  `report.md`; check `report.md` for merge conflicts, warnings, and missing
  descriptions.
- `data/ingredients.draft.tsv` is local scratch, not build input.
- Read `docs/data-model.md` and `docs/recipe-spec.md` before changing recipe
  matching or persistence. Key rule: a unit counts toward every group it
  qualifies for, so feasibility is per-group counting, not allocation.

## Architecture

- Root `handlers_*.go` files are HTTP handlers grouped by domain. Routes and the
  public / `requireLogin` / `requireDM` groups live in `main.go`.
- `internal/catalog` owns the item/recipe model, the in-memory `Store` loaded at
  startup and reloaded by `/admin/reload`, and DB persistence.
- `internal/crafting` contains matching plus calculator/cart planning. Cart plans
  are recomputed from inventory on each request; inventory is consumed only on
  finalize.
- `internal/auth` is PBKDF2 password hashing. There is no signup; bootstrap
  accounts with `cmd/createuser`. `/admin` requires role `dm`.
- Templates are embedded with `//go:embed templates/*.html`. A new file in
  `templates/` is picked up automatically, but requires a rebuild/restart.
- Open the DB with `catalog.OpenDB` and obtain queries via `database.For(db)`;
  avoid `database.New`/raw `sql.Open`. sqlc emits `?1` placeholders, while
  libsql only accepts `?`; `internal/database/rewrite.go` performs the
  translation.

## Code generation and dependencies

- After changing `sql/schema` or `sql/queries`, run `sqlc generate` (config in
  `sqlc.yaml`, output `internal/database`). Generated files are checked in.
- Add a new goose migration under `sql/schema/` for schema changes; do not edit
  migrations that have already been applied.
- Dependencies are vendored and committed. After changing `go.mod`/`go.sum`, run
  `go mod vendor`.

## Verification / CI

```bash
go test ./...                                      # CI: go test -cover ./...
go test ./internal/crafting -run TestMatchOption   # single test
go fmt ./...
staticcheck ./...
gosec -exclude-dir=internal/database ./...
```

CI runs on PRs to `main`. Pushes to `main` deploy through `scripts/buildprod.sh`
(`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`) into the Docker image and Cloud Run.
Do not commit `.env`, local `*.db` files, the root `craftingo` binary, or
`data/build/`.
