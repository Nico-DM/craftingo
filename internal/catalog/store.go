package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Store is an in-memory read model of the catalog, reloaded after DM edits.
type Store struct {
	db *sql.DB

	mu      sync.RWMutex
	items   []Item
	byID    map[string]*Item
	byName  map[string]*Item
	usedBy  map[string][]*Item // item form id -> items whose recipe consumes it
	loaded  bool
	loadErr error
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, byID: map[string]*Item{}, byName: map[string]*Item{}, usedBy: map[string][]*Item{}}
}

// Load (re)reads the whole catalog. It is safe to call while serving requests;
// readers see either the old or the new snapshot.
func (s *Store) Load(ctx context.Context) error {
	items, err := s.queryItems(ctx)
	if err != nil {
		s.mu.Lock()
		s.loadErr = err
		s.mu.Unlock()
		return err
	}
	recipes, recipeIDs, err := s.queryAllRecipes(ctx)
	if err != nil {
		s.mu.Lock()
		s.loadErr = err
		s.mu.Unlock()
		return err
	}
	if err := s.queryAllGroupsAndOptions(ctx, recipes, recipeIDs); err != nil {
		s.mu.Lock()
		s.loadErr = err
		s.mu.Unlock()
		return err
	}
	for i := range items {
		items[i].Recipe = recipes[items[i].ID]
		if items[i].Recipe != nil && items[i].Recipe.Rarity == "" {
			items[i].Recipe.Rarity = items[i].Rarity
		}
	}
	byID := make(map[string]*Item, len(items))
	byName := make(map[string]*Item, len(items))
	usedBy := map[string][]*Item{}
	for i := range items {
		it := &items[i]
		byID[it.ID] = it
		byName[strings.ToLower(it.Name)] = it
		if it.Recipe == nil {
			continue
		}
		for _, g := range it.Recipe.Groups {
			for _, o := range g.Options {
				for _, f := range o.Forms {
					if f.Kind == "item" && f.ID != "" {
						usedBy[f.ID] = append(usedBy[f.ID], it)
					}
				}
			}
		}
	}
	s.mu.Lock()
	s.items, s.byID, s.byName, s.usedBy, s.loaded, s.loadErr = items, byID, byName, usedBy, true, nil
	s.mu.Unlock()
	return nil
}

func (s *Store) Items() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Item, len(s.items))
	copy(out, s.items)
	return out
}

func (s *Store) Item(id string) (*Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.byID[id]
	return it, ok
}

func (s *Store) ItemByName(name string) (*Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.byName[strings.ToLower(name)]
	return it, ok
}

func (s *Store) UsedBy(id string) []*Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]*Item(nil), s.usedBy[id]...)
}

func (s *Store) Categories() []string {
	return s.distinct(func(it Item) string { return it.Category })
}

func (s *Store) Rarities() []string {
	rs := s.distinct(func(it Item) string { return it.Rarity })
	order := map[string]int{"Common": 1, "Uncommon": 2, "Rare": 3, "Very Rare": 4, "Legendary": 5, "Varies": 6}
	sort.SliceStable(rs, func(i, j int) bool { return order[rs[i]] < order[rs[j]] })
	return rs
}

func (s *Store) Sources() []string {
	return s.distinct(func(it Item) string { return it.Source })
}

func (s *Store) distinct(f func(Item) string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	var out []string
	for _, it := range s.items {
		v := f(it)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// Templates returns generated tier rows (for example Weapon +1 and Ammunition
// +4). Named modifiers are listed separately by Modifiers.
func (s *Store) Templates() []Item {
	var out []Item
	for _, it := range s.Items() {
		if it.Archetype != nil && it.Archetype.Kind == "tier" {
			out = append(out, it)
		}
	}
	return out
}

// Modifiers returns named modifier recipes that consume a tiered generic base.
func (s *Store) Modifiers() []Item {
	var out []Item
	for _, it := range s.Items() {
		if it.Archetype != nil && it.Archetype.Kind == "modifier" {
			out = append(out, it)
		}
	}
	return out
}

// BaseCandidates lists items that can be used as the base for an archetype.
func (s *Store) BaseCandidates(kind, name string) []Item {
	var out []Item
	for _, it := range s.Items() {
		if it.Template || it.Archetype != nil && it.Archetype.Kind == "tier" && it.Template {
			continue
		}
		switch kind {
		case "category":
			if strings.EqualFold(it.Category, name) && !it.Magic {
				out = append(out, it)
			}
		case "item":
			if it.ID == name {
				out = append(out, it)
			}
		}
	}
	return out
}

func (s *Store) queryItems(ctx context.Context) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, category, subtype, type_raw, rarity, magic, oota, source,
		       short_description, variant_of, variant_label, attunement, damage,
		       properties, weight, value, description, scraped_name, stat_source_code,
		       stat_source, match_status, match_method, match_detail, match_confidence,
		       intrinsic_tags, archetype_kind, archetype_base_kind, archetype_base_name,
		       archetype_tier, is_template
		FROM items
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var (
			it                                                Item
			subtype, variantOf, variantLabel, attunement      sql.NullString
			damage, properties, weight, value, description    sql.NullString
			scrapedName, statSourceCode, statSource           sql.NullString
			matchMethod, matchDetail                          sql.NullString
			matchConfidence                                   sql.NullFloat64
			intrinsicTags                                     string
			archetypeKind, archetypeBaseKind, archetypeBaseNm sql.NullString
			archetypeTier                                     sql.NullInt64
			magic, oota, isTemplate                           int64
		)
		err := rows.Scan(
			&it.ID, &it.Name, &it.Category, &subtype, &it.TypeRaw, &it.Rarity,
			&magic, &oota, &it.Source, &it.ShortDescription, &variantOf, &variantLabel,
			&attunement, &damage, &properties, &weight, &value, &description,
			&scrapedName, &statSourceCode, &statSource, &it.MatchStatus,
			&matchMethod, &matchDetail, &matchConfidence, &intrinsicTags,
			&archetypeKind, &archetypeBaseKind, &archetypeBaseNm, &archetypeTier,
			&isTemplate,
		)
		if err != nil {
			return nil, err
		}
		it.Subtype = strPtr(subtype)
		it.VariantOf = strPtr(variantOf)
		it.VariantLabel = strPtr(variantLabel)
		it.Attunement = strPtr(attunement)
		it.Damage = strPtr(damage)
		it.Properties = strPtr(properties)
		it.Weight = strPtr(weight)
		it.Value = strPtr(value)
		it.Description = strPtr(description)
		it.ScrapedName = strPtr(scrapedName)
		it.StatSourceCode = strPtr(statSourceCode)
		it.StatSource = strPtr(statSource)
		it.MatchMethod = strPtr(matchMethod)
		it.MatchDetail = strPtr(matchDetail)
		if matchConfidence.Valid {
			v := matchConfidence.Float64
			it.MatchConfidence = &v
		}
		it.Magic = magic == 1
		it.Oota = oota == 1
		it.Template = isTemplate == 1
		_ = json.Unmarshal([]byte(intrinsicTags), &it.IntrinsicTags)
		if archetypeKind.Valid {
			a := &Archetype{Kind: archetypeKind.String}
			if archetypeBaseKind.Valid {
				a.Base.Kind = archetypeBaseKind.String
			}
			if archetypeBaseNm.Valid {
				a.Base.Name = archetypeBaseNm.String
			}
			if archetypeTier.Valid {
				v := int(archetypeTier.Int64)
				a.Tier = &v
			}
			it.Archetype = a
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (s *Store) queryAllRecipes(ctx context.Context) (map[string]*Recipe, map[int64]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, item_id, kind, rarity, base_kind, base_name, notes
		FROM recipes ORDER BY item_id, position`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := map[string]*Recipe{}
	ids := map[int64]string{}
	for rows.Next() {
		var (
			id       int64
			itemID   string
			kind     string
			rarity   string
			baseKind sql.NullString
			baseName sql.NullString
			notesRaw string
		)
		if err := rows.Scan(&id, &itemID, &kind, &rarity, &baseKind, &baseName, &notesRaw); err != nil {
			return nil, nil, err
		}
		r := &Recipe{Kind: kind, Rarity: rarity}
		_ = json.Unmarshal([]byte(notesRaw), &r.Notes)
		if baseKind.Valid {
			r.BaseSelector = &BaseSelector{Kind: baseKind.String, Name: baseName.String}
		}
		ids[id] = itemID
		out[itemID] = r
	}
	return out, ids, rows.Err()
}

// queryAllGroupsAndOptions attaches groups, options and extras in bulk, so a
// remote Turso database sees five queries instead of thousands.
func (s *Store) queryAllGroupsAndOptions(ctx context.Context, recipes map[string]*Recipe, recipeIDs map[int64]string) error {
	grows, err := s.db.QueryContext(ctx, `
		SELECT g.id, g.recipe_id, g.position, g.quantity, g.quantity_max,
		       g.min_rarity, g.trait_mode, g.trait_var
		FROM recipe_groups g ORDER BY g.recipe_id, g.position`)
	if err != nil {
		return err
	}
	defer grows.Close()

	groups := map[int64]*Group{}
	groupsByRecipe := map[int64][]*Group{}
	for grows.Next() {
		var (
			gid       int64
			recipeID  int64
			pos       int
			quantity  int
			quantityM sql.NullInt64
			minRarity sql.NullString
			traitMode string
			traitVar  sql.NullInt64
		)
		if err := grows.Scan(&gid, &recipeID, &pos, &quantity, &quantityM, &minRarity, &traitMode, &traitVar); err != nil {
			return err
		}
		g := &Group{Quantity: quantity, MinRarity: minRarity.String, TraitMode: traitMode, Position: pos}
		if quantityM.Valid {
			v := int(quantityM.Int64)
			g.QuantityMax = &v
		}
		if traitVar.Valid {
			v := int(traitVar.Int64)
			g.TraitVar = &v
		}
		groups[gid] = g
		groupsByRecipe[recipeID] = append(groupsByRecipe[recipeID], g)
	}
	if err := grows.Err(); err != nil {
		return err
	}

	orows, err := s.db.QueryContext(ctx, `
		SELECT group_id, position, any_raw, tier, forms, traits, tags_all, tags_any,
		       output_tags, output_tags_any
		FROM recipe_options ORDER BY group_id, position`)
	if err != nil {
		return err
	}
	defer orows.Close()
	for orows.Next() {
		var (
			groupID          int64
			o                Option
			anyRaw           int64
			tier             sql.NullInt64
			forms, traits    string
			tagsAll, tagsAny string
			outTags, outAny  string
		)
		if err := orows.Scan(&groupID, &o.Position, &anyRaw, &tier, &forms, &traits,
			&tagsAll, &tagsAny, &outTags, &outAny); err != nil {
			return err
		}
		o.AnyRaw = anyRaw == 1
		if tier.Valid {
			v := int(tier.Int64)
			o.Tier = &v
		}
		_ = json.Unmarshal([]byte(forms), &o.Forms)
		_ = json.Unmarshal([]byte(traits), &o.Traits)
		_ = json.Unmarshal([]byte(tagsAll), &o.TagsAll)
		_ = json.Unmarshal([]byte(tagsAny), &o.TagsAny)
		_ = json.Unmarshal([]byte(outTags), &o.OutputTags)
		_ = json.Unmarshal([]byte(outAny), &o.OutputTagsAny)
		if g, ok := groups[groupID]; ok {
			g.Options = append(g.Options, o)
		}
	}
	if err := orows.Err(); err != nil {
		return err
	}

	for recipeID, list := range groupsByRecipe {
		itemID := recipeIDs[recipeID]
		r := recipes[itemID]
		if r == nil {
			continue
		}
		for _, g := range list {
			r.Groups = append(r.Groups, *g)
		}
	}

	erows, err := s.db.QueryContext(ctx, `
		SELECT e.recipe_id, e.kind, e.quantity, e.raw
		FROM recipe_extras e ORDER BY e.recipe_id, e.position`)
	if err != nil {
		return err
	}
	defer erows.Close()
	for erows.Next() {
		var (
			recipeID int64
			e        Extra
			qty      sql.NullInt64
		)
		if err := erows.Scan(&recipeID, &e.Kind, &qty, &e.Raw); err != nil {
			return err
		}
		if qty.Valid {
			v := int(qty.Int64)
			e.Quantity = &v
		}
		if r := recipes[recipeIDs[recipeID]]; r != nil {
			r.Extras = append(r.Extras, e)
		}
	}
	return erows.Err()
}

func strPtr(v sql.NullString) *string {
	if !v.Valid || v.String == "" {
		return nil
	}
	s := v.String
	return &s
}

func (r *Recipe) AddNotes(n ...string) {
	r.Notes = append(r.Notes, n...)
}

func (s *Store) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("catalog store (%d items, loaded=%v)", len(s.items), s.loaded)
}
