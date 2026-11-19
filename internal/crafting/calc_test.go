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
