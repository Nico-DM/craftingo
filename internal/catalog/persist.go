package catalog

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Nico-DM/craftingo/internal/database"
)

// SaveRecipe replaces an item's recipe with the given tree.
func SaveRecipe(ctx context.Context, q *database.Queries, itemID string, r *Recipe, itemRarity string) error {
	if err := q.DeleteRecipeForItem(ctx, itemID); err != nil {
		return err
	}
	if r == nil {
		return nil
	}
	kind := r.Kind
	if kind == "" {
		kind = "groups"
	}
	var baseKind, baseName sql.NullString
	if r.BaseSelector != nil {
		baseKind = sql.NullString{String: r.BaseSelector.Kind, Valid: true}
		baseName = sql.NullString{String: r.BaseSelector.Name, Valid: true}
	}
	notes, _ := json.Marshal(r.Notes)
	rarity := r.Rarity
	if rarity == "" {
		rarity = itemRarity
	}
	recipeID, err := q.InsertRecipe(ctx, database.InsertRecipeParams{
		ItemID:         itemID,
		Position:       1,
		Kind:           kind,
		Rarity:         rarity,
		OutputQuantity: int64(r.OutputQty()),
		BaseKind:       baseKind,
		BaseName:       baseName,
		Notes:          string(notes),
	})
	if err != nil {
		return err
	}

	for i, g := range r.Groups {
		pos := g.Position
		if pos == 0 {
			pos = i + 1
		}
		var qmax sql.NullInt64
		if g.QuantityMax != nil {
			qmax = sql.NullInt64{Int64: int64(*g.QuantityMax), Valid: true}
		}
		var minRarity sql.NullString
		if g.MinRarity != "" {
			minRarity = sql.NullString{String: g.MinRarity, Valid: true}
		}
		var traitVar sql.NullInt64
		if g.TraitVar != nil {
			traitVar = sql.NullInt64{Int64: int64(*g.TraitVar), Valid: true}
		}
		mode := g.TraitMode
		if mode == "" {
			mode = "fixed"
		}
		groupID, err := q.InsertRecipeGroup(ctx, database.InsertRecipeGroupParams{
			RecipeID:    recipeID,
			Position:    int64(pos),
			Quantity:    int64(g.Quantity),
			QuantityMax: qmax,
			MinRarity:   minRarity,
			TraitMode:   mode,
			TraitVar:    traitVar,
		})
		if err != nil {
			return err
		}
		for j, o := range g.Options {
			forms, _ := json.Marshal(o.Forms)
			traits, _ := json.Marshal(o.Traits)
			tagsAll, _ := json.Marshal(o.TagsAll)
			tagsAny, _ := json.Marshal(o.TagsAny)
			outTags, _ := json.Marshal(o.OutputTags)
			outTagsAny, _ := json.Marshal(o.OutputTagsAny)
			var tier sql.NullInt64
			if o.Tier != nil {
				tier = sql.NullInt64{Int64: int64(*o.Tier), Valid: true}
			}
			err := q.InsertRecipeOption(ctx, database.InsertRecipeOptionParams{
				GroupID:       groupID,
				Position:      int64(j + 1),
				AnyRaw:        boolToInt(o.AnyRaw),
				Tier:          tier,
				Forms:         string(forms),
				Traits:        string(traits),
				TagsAll:       string(tagsAll),
				TagsAny:       string(tagsAny),
				OutputTags:    string(outTags),
				OutputTagsAny: string(outTagsAny),
			})
			if err != nil {
				return err
			}
		}
	}

	for i, e := range r.Extras {
		var qty sql.NullInt64
		if e.Quantity != nil {
			qty = sql.NullInt64{Int64: int64(*e.Quantity), Valid: true}
		}
		if err := q.InsertRecipeExtra(ctx, database.InsertRecipeExtraParams{
			RecipeID: recipeID,
			Position: int64(i + 1),
			Kind:     e.Kind,
			Quantity: qty,
			Raw:      e.Raw,
		}); err != nil {
			return err
		}
	}
	return nil
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
