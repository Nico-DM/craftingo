// Command seed loads data/build/items.jsonl into a migrated database.
//
// Usage:
//
//	DATABASE_URL=file:craftingo.db go run ./cmd/seed
//	DATABASE_URL=file:craftingo.db go run ./cmd/seed -dataset data/build/items.jsonl
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/Nico-DM/craftingo/internal/catalog"
	"github.com/Nico-DM/craftingo/internal/database"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"
)

func main() {
	dataset := flag.String("dataset", "data/build/items.jsonl", "items JSONL produced by scripts/build_dataset.py")
	flag.Parse()

	_ = godotenv.Load(".env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	db, err := catalog.OpenDB(dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	items, err := readDataset(*dataset)
	if err != nil {
		log.Fatal(err)
	}

	if err := seed(db, items); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("seeded %d items\n", len(items))
}

func readDataset(path string) ([]catalog.Item, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-supplied build input
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var items []catalog.Item
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for line := 1; sc.Scan(); line++ {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var item catalog.Item
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		items = append(items, item)
	}
	return items, sc.Err()
}

func seed(db *sql.DB, items []catalog.Item) error {
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	// Turso/libsql over HTTP chokes on one giant transaction; commit in batches.
	const batchSize = 25

	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		if err := seedBatch(ctx, db, items[start:end], now); err != nil {
			return err
		}
		fmt.Printf("seeded %d/%d\n", end, len(items))
	}
	return nil
}

func seedBatch(ctx context.Context, db *sql.DB, items []catalog.Item, now string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q := database.For(tx)
	for _, it := range items {
		if err := q.UpsertItem(ctx, toUpsertParams(it, now)); err != nil {
			return fmt.Errorf("item %s: %w", it.Name, err)
		}
		if err := q.DeleteRecipeForItem(ctx, it.ID); err != nil {
			return err
		}
		if it.Recipe == nil {
			continue
		}
		if err := catalog.SaveRecipe(ctx, q, it.ID, it.Recipe, it.Rarity); err != nil {
			return fmt.Errorf("recipe %s: %w", it.Name, err)
		}
	}
	return tx.Commit()
}

func toUpsertParams(it catalog.Item, now string) database.UpsertItemParams {
	narg := func(s *string) sql.NullString {
		if s == nil || *s == "" {
			return sql.NullString{}
		}
		return sql.NullString{String: *s, Valid: true}
	}
	confidence := sql.NullFloat64{}
	if it.MatchConfidence != nil {
		confidence = sql.NullFloat64{Float64: *it.MatchConfidence, Valid: true}
	}
	tags, _ := json.Marshal(it.IntrinsicTags)
	status := it.MatchStatus
	if status == "" {
		status = "needs_review"
	}
	params := database.UpsertItemParams{
		ID:               it.ID,
		Name:             it.Name,
		Category:         it.Category,
		Subtype:          narg(it.Subtype),
		TypeRaw:          it.TypeRaw,
		Rarity:           it.Rarity,
		Magic:            boolInt(it.Magic),
		Visible:          boolInt(it.Visible),
		Source:           it.Source,
		ShortDescription: narg(it.ShortDescription),
		VariantOf:        narg(it.VariantOf),
		VariantLabel:     narg(it.VariantLabel),
		Attunement:       narg(it.Attunement),
		Damage:           narg(it.Damage),
		Properties:       narg(it.Properties),
		Weight:           narg(it.Weight),
		Value:            narg(it.Value),
		Description:      narg(it.Description),
		ScrapedName:      narg(it.ScrapedName),
		StatSourceCode:   narg(it.StatSourceCode),
		StatSource:       narg(it.StatSource),
		MatchStatus:      status,
		MatchMethod:      narg(it.MatchMethod),
		MatchDetail:      narg(it.MatchDetail),
		MatchConfidence:  confidence,
		IntrinsicTags:    string(tags),
		IsTemplate:       boolInt(it.Template),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if it.Archetype != nil {
		params.ArchetypeKind = sql.NullString{String: it.Archetype.Kind, Valid: true}
		params.ArchetypeBaseKind = sql.NullString{String: it.Archetype.Base.Kind, Valid: true}
		params.ArchetypeBaseName = sql.NullString{String: it.Archetype.Base.Name, Valid: true}
		if it.Archetype.Tier != nil {
			params.ArchetypeTier = sql.NullInt64{Int64: int64(*it.Archetype.Tier), Valid: true}
		}
	}
	return params
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
