package crafting

import (
	"testing"

	"github.com/Nico-DM/craftingo/internal/catalog"
)

func intPtr(n int) *int { return &n }

func TestMatchOption(t *testing.T) {
	tests := []struct {
		name string
		unit Unit
		opt  catalog.Option
		want bool
	}{
		{
			name: "raw form",
			unit: Unit{Kind: "material", FormKind: "raw", FormName: "Metal", Quantity: 1},
			opt:  catalog.Option{Forms: []catalog.Form{{Kind: "raw", Name: "Metal"}}},
			want: true,
		},
		{
			name: "wrong raw form",
			unit: Unit{Kind: "material", FormKind: "raw", FormName: "Bone", Quantity: 1},
			opt:  catalog.Option{Forms: []catalog.Form{{Kind: "raw", Name: "Metal"}}},
			want: false,
		},
		{
			name: "trait required",
			unit: Unit{Kind: "material", FormKind: "raw", FormName: "Metal", Trait: "Fiery", Quantity: 1},
			opt:  catalog.Option{Traits: []string{"Vital"}},
			want: false,
		},
		{
			name: "tag all",
			unit: Unit{Kind: "material", FormKind: "raw", FormName: "Fluid", Tags: []string{"Bug"}, Quantity: 1},
			opt:  catalog.Option{Forms: []catalog.Form{{Kind: "raw", Name: "Fluid"}}, TagsAll: []string{"Bug"}},
			want: true,
		},
		{
			name: "tag any",
			unit: Unit{Kind: "material", FormKind: "raw", FormName: "Bone", Tags: []string{"Dragon"}, Quantity: 1},
			opt:  catalog.Option{AnyRaw: true, TagsAny: []string{"Dragon", "Fiend"}},
			want: true,
		},
		{
			name: "generic tier met",
			unit: Unit{Kind: "crafted", ItemID: "longsword", Category: "Weapon", Tier: 2, Tags: []string{"Sword"}, Quantity: 1},
			opt: catalog.Option{
				Forms:   []catalog.Form{{Kind: "generic", Name: "Weapon"}},
				TagsAll: []string{"Sword"}, Tier: intPtr(2),
			},
			want: true,
		},
		{
			name: "generic tier too low",
			unit: Unit{Kind: "crafted", ItemID: "longsword", Category: "Weapon", Tier: 1, Tags: []string{"Sword"}, Quantity: 1},
			opt: catalog.Option{
				Forms:   []catalog.Form{{Kind: "generic", Name: "Weapon"}},
				TagsAll: []string{"Sword"}, Tier: intPtr(2),
			},
			want: false,
		},
		{
			name: "generic base rejects magic item",
			unit: Unit{Kind: "crafted", ItemID: "greataxe", Category: "Weapon", Magic: true, Tier: 2, Quantity: 1},
			opt:  catalog.Option{Forms: []catalog.Form{{Kind: "generic", Name: "Weapon"}}},
			want: false,
		},
		{
			name: "mundane item form rejects magic variant",
			unit: Unit{Kind: "crafted", ItemID: "greataxe", Category: "Weapon", Magic: true, Tier: 2, Quantity: 1},
			opt:  catalog.Option{Forms: []catalog.Form{{Kind: "item", Name: "Greataxe", ID: "greataxe", Mundane: true}}},
			want: false,
		},
		{
			name: "mundane item form accepts base item",
			unit: Unit{Kind: "crafted", ItemID: "greataxe", Category: "Weapon", Quantity: 1},
			opt:  catalog.Option{Forms: []catalog.Form{{Kind: "item", Name: "Greataxe", ID: "greataxe", Mundane: true}}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchOption(tt.unit, tt.opt); got != tt.want {
				t.Fatalf("MatchOption() = %v, want %v", got, tt.want)
			}
		})
	}
}

func metalGroup(qty int) catalog.Group {
	return catalog.Group{
		Quantity: qty, TraitMode: "fixed",
		Options: []catalog.Option{{Forms: []catalog.Form{{Kind: "raw", Name: "Metal"}}}},
	}
}

func TestEvaluateCountsEachUnitInEveryGroup(t *testing.T) {
	item := &catalog.Item{
		ID: "test", Name: "Test",
		Recipe: &catalog.Recipe{Kind: "groups", Groups: []catalog.Group{metalGroup(2), metalGroup(3)}},
	}
	units := []Unit{{Kind: "material", FormKind: "raw", FormName: "Metal", Rarity: "Common", Quantity: 3}}
	res := (&Calculator{}).Evaluate(item, units, 0)
	if !res.CanCraft {
		t.Fatalf("expected craftable, groups: %+v", res.Groups)
	}
	if len(res.Groups) != 2 || res.Groups[0].Have != 2 || res.Groups[1].Have != 3 {
		t.Fatalf("groups should be satisfied by the shared units: %+v", res.Groups)
	}
}

func TestEvaluateMissing(t *testing.T) {
	item := &catalog.Item{
		ID: "test", Name: "Test",
		Recipe: &catalog.Recipe{Kind: "groups", Groups: []catalog.Group{metalGroup(2), metalGroup(3)}},
	}
	units := []Unit{{Kind: "material", FormKind: "raw", FormName: "Metal", Quantity: 2}}
	res := (&Calculator{}).Evaluate(item, units, 0)
	if res.CanCraft || res.MissingAll != 1 || res.Groups[1].Missing != 1 {
		t.Fatalf("expected one missing in second group, got %+v", res)
	}
}

func TestEvaluateVariableTrait(t *testing.T) {
	source := catalog.Group{
		Quantity: 1, TraitMode: "fixed",
		Options: []catalog.Option{{AnyRaw: true, TagsAll: []string{"Dragon"}}},
	}
	variable := catalog.Group{
		Quantity: 3, TraitMode: "variable", TraitVar: intPtr(1),
		Options: []catalog.Option{{}},
	}
	item := &catalog.Item{
		ID: "dragon", Name: "Dragon Test",
		Recipe: &catalog.Recipe{Kind: "groups", Groups: []catalog.Group{source, variable}},
	}
	units := []Unit{
		{Kind: "material", FormKind: "raw", FormName: "Bone", Trait: "Fiery", Tags: []string{"Dragon"}, Quantity: 1},
		{Kind: "material", FormKind: "raw", FormName: "Metal", Trait: "Fiery", Quantity: 2},
		{Kind: "material", FormKind: "raw", FormName: "Wood", Trait: "Cold", Quantity: 1},
	}
	res := (&Calculator{}).Evaluate(item, units, 0)
	if !res.CanCraft {
		t.Fatalf("expected craftable: %+v", res)
	}
	if res.Groups[1].VariableTrait != "Fiery" {
		t.Fatalf("bound trait = %q, want Fiery", res.Groups[1].VariableTrait)
	}
	if res.Groups[1].Have != 3 {
		t.Fatalf("variable group have = %d, want 3", res.Groups[1].Have)
	}
}

func TestCraftedTraitQuantityCountsTowardTraitGroup(t *testing.T) {
	item := &catalog.Item{
		ID: "flame-tongue", Name: "Flame Tongue",
		Recipe: &catalog.Recipe{Kind: "groups", Rarity: "Rare", Groups: []catalog.Group{
			{
				Quantity: 1, TraitMode: "fixed",
				Options: []catalog.Option{{
					Forms:   []catalog.Form{{Kind: "generic", Name: "Weapon"}},
					TagsAll: []string{"Sword"}, Tier: intPtr(2),
				}},
			},
			{
				Quantity: 8, TraitMode: "fixed",
				Options: []catalog.Option{{Traits: []string{"Fiery"}}},
			},
		}},
	}
	units := []Unit{
		{
			ID: "shortsword", Kind: "crafted", FormKind: "item", ItemID: "shortsword",
			Category: "Weapon", Name: "Shortsword", Rarity: "Rare", Trait: "Fiery",
			TraitQty: 5, Tier: 2, Tags: []string{"Sword"}, Quantity: 1,
		},
		{ID: "ore", Kind: "material", FormKind: "raw", FormName: "Mineral", Rarity: "Uncommon", Trait: "Fiery", Quantity: 3},
	}
	res := (&Calculator{}).Evaluate(item, units, 0)
	if !res.CanCraft {
		t.Fatalf("expected the sword's 5 fiery units plus 3 mineral to satisfy 8x Fiery: %+v", res.Groups)
	}
	plan := (&Calculator{}).Plan(item, units)
	if !plan.Valid {
		t.Fatalf("expected a valid plan: %s", plan.Error)
	}
	if plan.Output.Trait != "Fiery" || plan.Output.TraitQty != 8 {
		t.Fatalf("output trait = %q (%d), want Fiery (8)", plan.Output.Trait, plan.Output.TraitQty)
	}
	if len(plan.Consumed) != 2 {
		t.Fatalf("expected the sword and the mineral stack to be consumed, got %+v", plan.Consumed)
	}
}

func TestCraftedTraitStackConsumesOnlyNeededItems(t *testing.T) {
	item := &catalog.Item{
		ID: "test", Name: "Test",
		Recipe: &catalog.Recipe{Kind: "groups", Groups: []catalog.Group{{
			Quantity: 8, TraitMode: "fixed",
			Options: []catalog.Option{{Traits: []string{"Fiery"}}},
		}}},
	}
	units := []Unit{{
		ID: "swords", Kind: "crafted", ItemID: "shortsword", Name: "Shortsword",
		Trait: "Fiery", TraitQty: 5, Quantity: 3,
	}}
	plan := (&Calculator{}).Plan(item, units)
	if !plan.Valid {
		t.Fatalf("expected valid plan: %s", plan.Error)
	}
	if len(plan.Consumed) != 1 || plan.Consumed[0].Quantity != 2 {
		t.Fatalf("expected 2 of the 3 swords consumed for 8x Fiery, got %+v", plan.Consumed)
	}
	left := ApplyPlan(units, plan)
	if len(left) != 1 || left[0].Quantity != 1 {
		t.Fatalf("expected 1 sword left, got %+v", left)
	}
}

func TestPlanSharedGroupsConsumeOnce(t *testing.T) {
	item := &catalog.Item{
		ID: "test", Name: "Test",
		Recipe: &catalog.Recipe{Kind: "groups", Groups: []catalog.Group{metalGroup(2), metalGroup(3)}},
	}
	units := []Unit{{ID: "metal", Kind: "material", FormKind: "raw", FormName: "Metal", Rarity: "Common", Quantity: 3}}
	plan := (&Calculator{}).Plan(item, units)
	if !plan.Valid {
		t.Fatalf("3 metal satisfies 2 and 3 because units count toward every group: %s", plan.Error)
	}
	if len(plan.Consumed) != 1 || plan.Consumed[0].Quantity != 3 {
		t.Fatalf("expected the shared 3 metal to be consumed once, got %+v", plan.Consumed)
	}
	left := ApplyPlan(units, plan)
	if len(left) != 0 {
		t.Fatalf("expected the stack to be empty, got %+v", left)
	}

	units[0].Quantity = 5
	plan = (&Calculator{}).Plan(item, units)
	if !plan.Valid {
		t.Fatalf("5 metal should be allocatable: %s", plan.Error)
	}
	if plan.Consumed[0].Quantity != 3 {
		t.Fatalf("only the largest group requirement should be consumed, got %+v", plan.Consumed)
	}
}

func TestPlanOutputTraitAndRarity(t *testing.T) {
	item := &catalog.Item{
		ID: "test", Name: "Test", Rarity: "Common",
		Recipe: &catalog.Recipe{Kind: "groups", Rarity: "Common", Groups: []catalog.Group{{
			Quantity: 3, TraitMode: "fixed",
			Options: []catalog.Option{{Traits: []string{"Fiery"}}},
		}}},
	}
	units := []Unit{
		{ID: "a", Kind: "material", FormKind: "raw", FormName: "Metal", Rarity: "Rare", Trait: "Fiery", Quantity: 2},
		{ID: "b", Kind: "material", FormKind: "raw", FormName: "Wood", Rarity: "Rare", Trait: "Fiery", Quantity: 1},
	}
	plan := (&Calculator{}).Plan(item, units)
	if !plan.Valid {
		t.Fatalf("expected valid plan: %s", plan.Error)
	}
	if plan.Output.Trait != "Fiery" || plan.Output.TraitQty != 3 {
		t.Fatalf("output trait = %q (%d), want Fiery (3)", plan.Output.Trait, plan.Output.TraitQty)
	}
	if plan.Output.Rarity != "Rare" {
		t.Fatalf("output rarity = %q, want Rare (lowest consumed material)", plan.Output.Rarity)
	}
}

func TestPlanBatchOutput(t *testing.T) {
	item := &catalog.Item{
		ID: "arrows-plus-3", Name: "Arrows +3 (20)", Rarity: "Very Rare", Category: "Ammunition",
		ScrapedName: strPtr("+3 Arrow"), Magic: true,
		Recipe: &catalog.Recipe{Kind: "groups", Rarity: "Very Rare", OutputQuantity: 20, Groups: []catalog.Group{{
			Quantity: 2, TraitMode: "fixed",
			Options: []catalog.Option{{Forms: []catalog.Form{{Kind: "raw", Name: "Metal"}}}},
		}}},
	}
	units := []Unit{{ID: "metal", Kind: "material", FormKind: "raw", FormName: "Metal", Rarity: "Common", Quantity: 2}}
	plan := (&Calculator{}).Plan(item, units)
	if !plan.Valid {
		t.Fatalf("expected a valid plan: %s", plan.Error)
	}
	if plan.Output.Quantity != 20 {
		t.Fatalf("output quantity = %d, want 20", plan.Output.Quantity)
	}
	if plan.Output.Name != "+3 Arrow" {
		t.Fatalf("output name = %q, want +3 Arrow", plan.Output.Name)
	}
	if plan.Output.Tier != 3 {
		t.Fatalf("output tier = %d, want 3 from the name", plan.Output.Tier)
	}
}

func TestBatchConsumptionAndOutput(t *testing.T) {
	slaying := &catalog.Item{
		ID: "slaying", Name: "Ammunition of Slaying, Aberrations (10)", Category: "Ammunition",
		ScrapedName: strPtr("Arrow of Slaying"), Magic: true,
		Recipe: &catalog.Recipe{Kind: "groups", Rarity: "Very Rare", OutputQuantity: 10, Groups: []catalog.Group{{
			Quantity: 10, TraitMode: "fixed",
			Options: []catalog.Option{{
				Forms: []catalog.Form{{Kind: "generic", Name: "Ammunition"}},
				Tier:  intPtr(3),
			}},
		}}},
	}
	units := []Unit{{
		ID: "arrows", Kind: "crafted", FormKind: "item", ItemID: "arrows-plus-3",
		Category: "Ammunition", Name: "+3 Arrow", Rarity: "Very Rare",
		Tier: 3, Magic: true, Quantity: 20,
	}}
	plan := (&Calculator{}).Plan(slaying, units)
	if !plan.Valid {
		t.Fatalf("expected 20 arrows to satisfy 10x +3 ammunition: %s", plan.Error)
	}
	if len(plan.Consumed) != 1 || plan.Consumed[0].Quantity != 10 {
		t.Fatalf("expected 10 arrows consumed, got %+v", plan.Consumed)
	}
	left := ApplyPlan(units, plan)
	if len(left) != 1 || left[0].Quantity != 10 {
		t.Fatalf("expected 10 arrows left, got %+v", left)
	}
	if plan.Output.Quantity != 10 || plan.Output.Name != "Arrow of Slaying" {
		t.Fatalf("output = %dx %q, want 10x Arrow of Slaying", plan.Output.Quantity, plan.Output.Name)
	}
}

func strPtr(s string) *string { return &s }

func TestMinRarity(t *testing.T) {
	item := &catalog.Item{
		ID: "rare", Name: "Rare Test",
		Recipe: &catalog.Recipe{Kind: "groups", Groups: []catalog.Group{{
			Quantity: 1, MinRarity: "Rare", TraitMode: "fixed",
			Options: []catalog.Option{{Forms: []catalog.Form{{Kind: "raw", Name: "Metal"}}}},
		}}},
	}
	res := (&Calculator{}).Evaluate(item, []Unit{{Kind: "material", FormKind: "raw", FormName: "Metal", Rarity: "Uncommon", Quantity: 1}}, 0)
	if res.CanCraft {
		t.Fatal("uncommon unit must not satisfy a rare minimum")
	}
	res = (&Calculator{}).Evaluate(item, []Unit{{Kind: "material", FormKind: "raw", FormName: "Metal", Rarity: "Very Rare", Quantity: 1}}, 0)
	if !res.CanCraft {
		t.Fatal("very rare unit should satisfy a rare minimum")
	}
}
