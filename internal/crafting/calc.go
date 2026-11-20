// Package crafting implements recipe matching and the can-craft calculator.
package crafting

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Nico-DM/craftingo/internal/catalog"
)

// Unit is one stack of raw material or one crafted item from an inventory.
type Unit struct {
	ID       string
	Kind     string // "material" or "crafted"
	FormKind string // "raw" or "item"
	FormName string
	ItemID   string
	Category string
	Subtype  string
	Name     string
	Rarity   string
	Trait    string
	TraitQty int
	Tier     int
	Tags     []string
	Quantity int
	Magic    bool
}

// GroupProgress is the per-group result of a feasibility check.
type GroupProgress struct {
	Position      int
	Group         catalog.Group
	Required      int
	Have          int
	Missing       int
	VariableTrait string
	OptionHave    []int // matches per option, for display
}

// Subcraft is a missing item that can itself be crafted from the inventory.
type Subcraft struct {
	Item   *catalog.Item
	Recipe *catalog.Item
	Depth  int
}

// Result is the full calculator output for one item.
type Result struct {
	Item       *catalog.Item
	CanCraft   bool
	Groups     []GroupProgress
	Subcrafts  []Subcraft
	MissingAll int
}

// Calculator evaluates recipes against a snapshot of inventory units.
type Calculator struct {
	Store *catalog.Store
}

// MatchOption reports whether one unit satisfies one recipe option, ignoring the
// group quantity and the variable-trait rule (handled by the caller).
func MatchOption(unit Unit, opt catalog.Option) bool {
	if opt.AnyRaw && !(unit.Kind == "material" && unit.FormKind == "raw") {
		return false
	}
	if len(opt.Forms) > 0 {
		ok := false
		for _, f := range opt.Forms {
			if matchForm(unit, f, opt.Tier) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(opt.Traits) > 0 && !containsFold(opt.Traits, unit.Trait) {
		return false
	}
	for _, tag := range opt.TagsAll {
		if !containsFold(unit.Tags, tag) {
			return false
		}
	}
	if len(opt.TagsAny) > 0 {
		ok := false
		for _, tag := range opt.TagsAny {
			if containsFold(unit.Tags, tag) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func matchForm(unit Unit, f catalog.Form, optionTier *int) bool {
	switch f.Kind {
	case "raw":
		return unit.Kind == "material" && unit.FormKind == "raw" && strings.EqualFold(unit.FormName, f.Name)
	case "item":
		if unit.Kind != "crafted" || unit.ItemID != f.ID {
			return false
		}
		if f.Mundane && unit.Magic {
			return false
		}
		if optionTier != nil && unit.Tier < *optionTier {
			return false
		}
		return true
	case "generic":
		if unit.Kind != "crafted" {
			return false
		}
		// Generic "any Weapon/Armor" bases mean mundane items. A recipe that
		// explicitly asks for a tiered base (e.g. Vicious Weapon's +2 Weapon)
		// may use a magic item once it reaches that tier.
		if unit.Magic && (optionTier == nil || unit.Tier < *optionTier) {
			return false
		}
		if !strings.EqualFold(unit.Category, f.Name) {
			return false
		}
		if optionTier != nil && unit.Tier < *optionTier {
			return false
		}
		return true
	}
	return false
}

func matchesAny(u Unit, options []catalog.Option) bool {
	for _, o := range options {
		if MatchOption(u, o) {
			return true
		}
	}
	return false
}

func containsFold(list []string, value string) bool {
	for _, s := range list {
		if strings.EqualFold(s, value) {
			return true
		}
	}
	return false
}

// groupUsesTraits reports whether a group is counted in trait units. A crafted
// item carrying Fiery (5) contributes five units to an "8x Fiery" requirement,
// but still counts as one unit for form-only requirements.
func groupUsesTraits(g catalog.Group) bool {
	if g.TraitMode == "variable" {
		return true
	}
	for _, o := range g.Options {
		if len(o.Traits) > 0 {
			return true
		}
	}
	return false
}

// unitTraitAmount is how many trait units one unit contributes when the group
// is trait-based. Raw stacks contribute one per item; crafted items contribute
// the full trait quantity they carry.
func unitTraitAmount(u Unit) int {
	if u.Kind == "crafted" {
		if u.TraitQty > 0 {
			return u.TraitQty
		}
		return 1
	}
	if u.Quantity > 0 {
		return u.Quantity
	}
	return 0
}

// Evaluate checks one item against the units. depth controls recursive sub-craft
// hints; pass 0 for a flat check.
func (c *Calculator) Evaluate(item *catalog.Item, units []Unit, depth int) Result {
	return c.evaluate(item, units, depth, map[string]bool{})
}

func (c *Calculator) evaluate(item *catalog.Item, units []Unit, depth int, seen map[string]bool) Result {
	res := Result{Item: item}
	if item == nil || item.Recipe == nil || item.Recipe.Kind != "groups" {
		return res
	}
	if seen[item.ID] {
		return res
	}
	seen[item.ID] = true
	defer delete(seen, item.ID)

	c.derive(units)

	groups := item.Recipe.Groups
	// Count matches per group and option. A unit can count toward every group.
	for gi, g := range groups {
		gp := GroupProgress{Position: g.Position, Group: g, Required: g.Quantity}
		if gp.Required == 0 {
			gp.Required = g.Quantity
		}
		traitBased := groupUsesTraits(g)
		gp.OptionHave = make([]int, len(g.Options))
		for ui := range units {
			u := units[ui]
			if u.Quantity <= 0 {
				continue
			}
			if g.MinRarity != "" && catalog.RarityRank(u.Rarity) < catalog.RarityRank(g.MinRarity) {
				continue
			}
			amount := u.Quantity
			if traitBased {
				amount = unitTraitAmount(u)
			}
			matchedGroup := false
			for oi, opt := range g.Options {
				if MatchOption(u, opt) {
					gp.OptionHave[oi] += amount
					matchedGroup = true
				}
			}
			if matchedGroup {
				gp.Have += amount
			}
		}
		if g.TraitMode == "variable" {
			// The variable group binds to the trait of the source group's material.
			// Only the source group chooses the trait; the N units in the variable
			// group may be any units carrying that same trait.
			sourceIdx := 0
			if g.TraitVar != nil {
				sourceIdx = *g.TraitVar - 1
			}
			candidates := map[string]bool{}
			if sourceIdx >= 0 && sourceIdx < len(groups) {
				for _, u := range units {
					if u.Trait == "" || u.Quantity <= 0 {
						continue
					}
					for _, opt := range groups[sourceIdx].Options {
						if MatchOption(u, opt) {
							candidates[u.Trait] = true
							break
						}
					}
				}
			}
			bestTrait, best := "", 0
			for candidate := range candidates {
				count := 0
				for _, u := range units {
					if strings.EqualFold(u.Trait, candidate) && u.Quantity > 0 {
						if len(g.Options) == 0 || matchesAny(u, g.Options) {
							count += unitTraitAmount(u)
						}
					}
				}
				if count > best {
					bestTrait, best = candidate, count
				}
			}
			gp.VariableTrait = bestTrait
			gp.Have = best
		}
		if gp.Have > gp.Required {
			gp.Have = gp.Required
		}
		gp.Missing = gp.Required - gp.Have
		if gp.Missing < 0 {
			gp.Missing = 0
		}
		res.MissingAll += gp.Missing
		res.Groups = append(res.Groups, gp)
		_ = gi
	}
	res.CanCraft = res.MissingAll == 0

	if !res.CanCraft && depth > 0 {
		res.Subcrafts = c.subcrafts(item, units, res, depth, seen)
	}
	return res
}

// derive fills fields that callers may only have supplied as raw columns.
func (c *Calculator) derive(units []Unit) {
	if c.Store == nil {
		return
	}
	for i := range units {
		if units[i].ItemID == "" {
			continue
		}
		if it, ok := c.Store.Item(units[i].ItemID); ok {
			if units[i].Category == "" {
				units[i].Category = it.Category
			}
			if units[i].Subtype == "" && it.Subtype != nil {
				units[i].Subtype = *it.Subtype
			}
			if units[i].Name == "" {
				units[i].Name = it.Name
			}
			units[i].Magic = units[i].Magic || it.Magic
			if units[i].Tier == 0 && units[i].Kind == "crafted" && it.Archetype != nil && it.Archetype.Tier != nil {
				units[i].Tier = *it.Archetype.Tier
			}
		}
	}
}

func (c *Calculator) subcrafts(item *catalog.Item, units []Unit, res Result, depth int, seen map[string]bool) []Subcraft {
	var out []Subcraft
	for _, gp := range res.Groups {
		if gp.Missing == 0 {
			continue
		}
		for _, opt := range gp.Group.Options {
			for _, f := range opt.Forms {
				if f.Kind != "item" || f.ID == "" {
					continue
				}
				cand, ok := c.Store.Item(f.ID)
				if !ok || cand.Recipe == nil || cand.Recipe.Kind != "groups" || seen[cand.ID] {
					continue
				}
				sub := c.evaluate(cand, units, depth-1, seen)
				if sub.CanCraft {
					out = append(out, Subcraft{Item: cand, Depth: depth})
				}
			}
		}
	}
	return dedupeSubcrafts(out)
}

func dedupeSubcrafts(in []Subcraft) []Subcraft {
	seen := map[string]bool{}
	var out []Subcraft
	for _, s := range in {
		if seen[s.Item.ID] {
			continue
		}
		seen[s.Item.ID] = true
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Item.Name < out[j].Item.Name })
	return out
}

// Craftable lists every catalog item that can currently be crafted.
func (c *Calculator) Craftable(units []Unit, limit int) []Result {
	var out []Result
	for _, it := range c.Store.Items() {
		if it.Recipe == nil || it.Recipe.Kind != "groups" || it.Template {
			continue
		}
		res := c.Evaluate(&it, units, 0)
		if res.CanCraft {
			out = append(out, res)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// RenderOption is a compact human-readable description of one option.
func RenderOption(o catalog.Option) string {
	var parts []string
	if o.AnyRaw {
		parts = append(parts, "Material")
	}
	var forms []string
	for _, f := range o.Forms {
		name := f.Name
		if f.Kind == "generic" {
			name = "any " + name
		}
		forms = append(forms, name)
	}
	if len(forms) > 0 {
		parts = append(parts, strings.Join(forms, "/"))
	}
	if o.Tier != nil {
		parts = append([]string{fmt.Sprintf("+%d", *o.Tier)}, parts...)
	}
	if len(o.Traits) > 0 {
		parts = append(parts, strings.Join(o.Traits, " + "))
	}
	if len(o.TagsAll) > 0 {
		parts = append(parts, "("+strings.Join(o.TagsAll, ", ")+")")
	}
	if len(o.TagsAny) > 0 {
		parts = append(parts, "("+strings.Join(o.TagsAny, "/")+")")
	}
	if len(parts) == 0 {
		return "any matching unit"
	}
	return strings.Join(parts, " ")
}
