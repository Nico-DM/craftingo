package crafting

import (
	"sort"
	"strings"

	"github.com/Nico-DM/craftingo/internal/catalog"
)

// Allocation is one unit consumed by a plan. Quantity is the number of items
// taken from the stack; a crafted stack may contribute only part of its items,
// or one item may satisfy several trait requirement units.
type Allocation struct {
	UnitID   string
	Name     string
	Kind     string
	Rarity   string
	Trait    string
	TraitQty int
	Quantity int
}

// GroupPlan is the resolved allocation for one requirement group.
type GroupPlan struct {
	Position    int
	Option      catalog.Option
	OptionIndex int
	Required    int
	Trait       string
	Consumed    []Allocation
}

// Plan is a concrete allocation of inventory units to one craft. A unit can
// satisfy several groups in the same recipe (the game counts it toward every
// group it qualifies for), but it is only removed from the inventory once.
type Plan struct {
	Item        *catalog.Item
	Groups      []GroupPlan
	Consumed    []Allocation
	TraitTotals map[string]int
	Output      Unit
	Valid       bool
	Error       string
}

const maxPlanAttempts = 20000

// Plan allocates units to an item's recipe, choosing the most prominent trait
// for the output.
func (c *Calculator) Plan(item *catalog.Item, units []Unit) Plan {
	return c.PlanWithTrait(item, units, "")
}

// PlanWithTrait is Plan with an explicit output trait for tie-breaking.
func (c *Calculator) PlanWithTrait(item *catalog.Item, units []Unit, preferredTrait string) Plan {
	p := Plan{Item: item}
	if item == nil || item.Recipe == nil || item.Recipe.Kind != "groups" {
		p.Error = "This item has no material recipe."
		return p
	}
	work := cloneUnits(units)
	c.derive(work)

	byID := make(map[string]Unit, len(work))
	for _, u := range work {
		byID[u.ID] = u
	}

	attempts := 0
	groups, ok := allocate(item.Recipe.Groups, 0, work, nil, &attempts)
	if !ok {
		p.Error = "Not enough materials to assign every requirement."
		return p
	}
	p.Groups = groups
	p.Consumed = mergeConsumed(groups)

	recipeRarity := item.Recipe.Rarity
	if recipeRarity == "" {
		recipeRarity = item.Rarity
	}
	outRank := catalog.RarityRank(recipeRarity)
	lowest := 0
	traitTotals := map[string]int{}
	for _, a := range p.Consumed {
		u := byID[a.UnitID]
		if r := catalog.RarityRank(u.Rarity); r > 0 && (lowest == 0 || r < lowest) {
			lowest = r
		}
		if u.Trait == "" {
			continue
		}
		amount := a.Quantity
		if u.Kind == "crafted" {
			amount = a.Quantity * craftedTraitPerItem(u)
		}
		traitTotals[u.Trait] += amount
	}
	if lowest > outRank {
		outRank = lowest
	}

	chosenTrait := preferredTrait
	if chosenTrait != "" {
		if _, ok := traitTotals[chosenTrait]; !ok {
			chosenTrait = ""
		}
	}
	if chosenTrait == "" {
		best := 0
		for trait, total := range traitTotals {
			if total > best || (total == best && (chosenTrait == "" || trait < chosenTrait)) {
				chosenTrait, best = trait, total
			}
		}
	}

	name := item.UnitName()
	out := Unit{
		Kind:     "crafted",
		FormKind: "item",
		FormName: name,
		ItemID:   item.ID,
		Category: item.Category,
		Subtype:  itemSubtypeOf(item),
		Name:     name,
		Rarity:   rarityName(outRank, recipeRarity),
		Trait:    chosenTrait,
		TraitQty: traitTotals[chosenTrait],
		Quantity: item.Recipe.OutputQty(),
		Magic:    item.Magic,
		Tags:     outputTags(item, groups, byID),
	}
	out.Tier = itemTier(item)

	p.TraitTotals = traitTotals
	p.Output = out
	p.Valid = true
	return p
}

// ApplyPlan returns units with a plan's allocation removed.
func ApplyPlan(units []Unit, p Plan) []Unit {
	out := cloneUnits(units)
	idx := make(map[string]int, len(out))
	for i := range out {
		idx[out[i].ID] = i
	}
	for _, a := range p.Consumed {
		i, ok := idx[a.UnitID]
		if !ok {
			continue
		}
		out[i].Quantity -= a.Quantity
	}
	kept := out[:0]
	for _, u := range out {
		if u.Quantity > 0 {
			kept = append(kept, u)
		}
	}
	return kept
}

// allocate chooses an option for every group. Groups are independent at this
// stage: the same unit may satisfy more than one group, exactly like Evaluate.
func allocate(groups []catalog.Group, gi int, units []Unit, prior []GroupPlan, attempts *int) ([]GroupPlan, bool) {
	if gi >= len(groups) {
		return prior, true
	}
	*attempts++
	if *attempts > maxPlanAttempts {
		return nil, false
	}
	g := groups[gi]
	traitBased := groupUsesTraits(g)
	bound := ""
	if g.TraitMode == "variable" {
		bound = boundTrait(g, prior)
		if bound == "" {
			return nil, false
		}
	}

	options := g.Options
	if len(options) == 0 {
		options = []catalog.Option{{}}
	}
	order := make([]int, len(options))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return optionAvailable(units, g, options[order[a]], bound, traitBased) >
			optionAvailable(units, g, options[order[b]], bound, traitBased)
	})

	for _, oi := range order {
		opt := options[oi]
		allocs, ok := consumeOption(units, g, opt, bound, traitBased, groups)
		if !ok {
			continue
		}
		gp := GroupPlan{
			Position: g.Position, Option: opt, OptionIndex: oi,
			Required: g.Quantity, Trait: bound, Consumed: allocs,
		}
		nextPrior := append(append([]GroupPlan{}, prior...), gp)
		if final, ok := allocate(groups, gi+1, units, nextPrior, attempts); ok {
			return final, true
		}
	}
	return nil, false
}

func optionAvailable(units []Unit, g catalog.Group, opt catalog.Option, bound string, traitBased bool) int {
	total := 0
	for _, u := range units {
		if u.Quantity <= 0 {
			continue
		}
		if g.MinRarity != "" && catalog.RarityRank(u.Rarity) < catalog.RarityRank(g.MinRarity) {
			continue
		}
		if bound != "" && !strings.EqualFold(u.Trait, bound) {
			continue
		}
		if !MatchOption(u, opt) {
			continue
		}
		if traitBased {
			total += unitTraitAmount(u)
		} else {
			total += u.Quantity
		}
	}
	return total
}

func consumeOption(units []Unit, g catalog.Group, opt catalog.Option, bound string, traitBased bool, groups []catalog.Group) ([]Allocation, bool) {
	need := g.Quantity
	if need <= 0 {
		return nil, true
	}

	type candidate struct {
		unit        Unit
		amount      int
		flexibility int
	}
	var candidates []candidate
	for _, u := range units {
		if u.Quantity <= 0 {
			continue
		}
		if g.MinRarity != "" && catalog.RarityRank(u.Rarity) < catalog.RarityRank(g.MinRarity) {
			continue
		}
		if bound != "" && !strings.EqualFold(u.Trait, bound) {
			continue
		}
		if !MatchOption(u, opt) {
			continue
		}
		amount := u.Quantity
		if traitBased {
			amount = unitTraitAmount(u)
		}
		if amount <= 0 {
			continue
		}
		candidates = append(candidates, candidate{
			unit: u, amount: amount,
			flexibility: unitFlexibility(u, groups),
		})
	}
	// Prefer the least flexible stacks first, and among equals the smallest
	// contribution, so large stacks and versatile units stay available.
	sort.SliceStable(candidates, func(a, b int) bool {
		ca, cb := candidates[a], candidates[b]
		if ca.flexibility != cb.flexibility {
			return ca.flexibility < cb.flexibility
		}
		return ca.amount < cb.amount
	})

	var allocs []Allocation
	for _, cand := range candidates {
		if need <= 0 {
			break
		}
		u := cand.unit
		take := cand.amount
		if u.Kind == "crafted" && traitBased {
			// A crafted stack contributes TraitQty per item. Consume only as
			// many items as the remaining trait units need.
			perItem := craftedTraitPerItem(u)
			items := (need + perItem - 1) / perItem
			if items > u.Quantity {
				items = u.Quantity
			}
			if items < 1 {
				break
			}
			take = items * perItem
			allocs = append(allocs, Allocation{
				UnitID: u.ID, Name: u.Name, Kind: u.Kind,
				Rarity: u.Rarity, Trait: u.Trait, TraitQty: u.TraitQty, Quantity: items,
			})
			need -= take
			continue
		}
		if take > need {
			take = need
		}
		need -= take
		allocs = append(allocs, Allocation{
			UnitID: u.ID, Name: u.Name, Kind: u.Kind,
			Rarity: u.Rarity, Trait: u.Trait, TraitQty: u.TraitQty, Quantity: take,
		})
	}
	if need > 0 {
		return nil, false
	}
	return allocs, true
}

func unitFlexibility(u Unit, groups []catalog.Group) int {
	n := 0
	for _, g := range groups {
		if g.TraitMode == "variable" {
			continue
		}
		for _, o := range g.Options {
			if MatchOption(u, o) {
				n++
				break
			}
		}
	}
	return n
}

func boundTrait(g catalog.Group, prior []GroupPlan) string {
	idx := 0
	if g.TraitVar != nil {
		idx = *g.TraitVar - 1
	}
	if idx < 0 || idx >= len(prior) {
		return ""
	}
	totals := map[string]int{}
	for _, a := range prior[idx].Consumed {
		if a.Trait == "" {
			continue
		}
		amount := a.Quantity
		if a.Kind == "crafted" {
			perItem := a.TraitQty
			if perItem <= 0 {
				perItem = 1
			}
			amount = a.Quantity * perItem
		}
		totals[a.Trait] += amount
	}
	best, bestTotal := "", 0
	for trait, total := range totals {
		if total > bestTotal || (total == bestTotal && (best == "" || trait < best)) {
			best, bestTotal = trait, total
		}
	}
	return best
}

// mergeConsumed keeps the largest amount any group took from a stack, because
// groups share the same pool instead of allocating it separately.
func mergeConsumed(groups []GroupPlan) []Allocation {
	order := []string{}
	merged := map[string]Allocation{}
	for _, gp := range groups {
		for _, a := range gp.Consumed {
			if existing, ok := merged[a.UnitID]; ok {
				if a.Quantity > existing.Quantity {
					existing.Quantity = a.Quantity
				}
				merged[a.UnitID] = existing
				continue
			}
			order = append(order, a.UnitID)
			merged[a.UnitID] = a
		}
	}
	out := make([]Allocation, 0, len(order))
	for _, id := range order {
		out = append(out, merged[id])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func outputTags(item *catalog.Item, groups []GroupPlan, byID map[string]Unit) []string {
	var tags []string
	for _, t := range item.IntrinsicTags {
		tags = appendUniqueFold(tags, t)
	}
	consumedTags := map[string]bool{}
	for _, gp := range groups {
		for _, a := range gp.Consumed {
			for _, t := range byID[a.UnitID].Tags {
				consumedTags[strings.ToLower(t)] = true
			}
		}
	}
	for _, gp := range groups {
		for _, t := range gp.Option.OutputTags {
			tags = appendUniqueFold(tags, t)
		}
		for _, t := range gp.Option.OutputTagsAny {
			if consumedTags[strings.ToLower(t)] {
				tags = appendUniqueFold(tags, t)
			}
		}
	}
	return tags
}

func appendUniqueFold(list []string, v string) []string {
	if v == "" || containsFold(list, v) {
		return list
	}
	return append(list, v)
}

func cloneUnits(units []Unit) []Unit {
	out := make([]Unit, len(units))
	copy(out, units)
	for i := range out {
		if units[i].Tags != nil {
			out[i].Tags = append([]string(nil), units[i].Tags...)
		}
	}
	return out
}

func itemSubtypeOf(it *catalog.Item) string {
	if it == nil || it.Subtype == nil {
		return ""
	}
	return *it.Subtype
}

// itemTier reads the tier from the archetype, falling back to the name for
// named batch rows such as "Arrows +3 (20)" that are not generated templates.
func itemTier(it *catalog.Item) int {
	return it.Tier()
}

func rarityName(rank int, fallback string) string {
	switch rank {
	case 1:
		return "Common"
	case 2:
		return "Uncommon"
	case 3:
		return "Rare"
	case 4:
		return "Very Rare"
	case 5:
		return "Legendary"
	}
	return fallback
}
