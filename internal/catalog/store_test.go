package catalog

import "testing"

func TestBuildUsedByDedupesRepeatedForms(t *testing.T) {
	arrowsForm := Form{Kind: "item", ID: "arrows", Name: "Arrows (20)"}
	items := []Item{
		{ID: "arrows", Name: "Arrows (20)"},
		{
			ID:   "unbreakable-arrow",
			Name: "Unbreakable Arrow (10)",
			Recipe: &Recipe{Kind: "groups", Groups: []Group{
				{Quantity: 4, Options: []Option{{Traits: []string{"Forceful"}}}},
				{Quantity: 10, Options: []Option{
					{Forms: []Form{arrowsForm}, TagsAll: []string{"Metal"}},
					{Forms: []Form{arrowsForm}, TagsAll: []string{"Mineral"}},
					{Forms: []Form{arrowsForm}, TagsAll: []string{"Bone"}},
				}},
			}},
		},
		{
			ID:   "quiver",
			Name: "Quiver",
			Recipe: &Recipe{Kind: "groups", Groups: []Group{
				{Quantity: 1, Options: []Option{{Forms: []Form{arrowsForm}}}},
			}},
		},
	}

	usedBy := buildUsedBy(items)
	got := usedBy["arrows"]
	if len(got) != 2 {
		t.Fatalf("usedBy[arrows] = %d entries, want 2", len(got))
	}
	if got[0].ID != "unbreakable-arrow" || got[1].ID != "quiver" {
		t.Fatalf("usedBy[arrows] = [%s %s], want [unbreakable-arrow quiver]", got[0].ID, got[1].ID)
	}
	if _, ok := usedBy["unbreakable-arrow"]; ok {
		t.Fatalf("unbreakable-arrow should not be listed as used")
	}
}
