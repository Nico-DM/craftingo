package catalog

import "testing"

func TestBatchQuantity(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"Arrows (20)", 20},
		{"Bead of Force (7)", 7},
		{"Luck Blade (3, Sword)", 1},
		{"Potion of Healing", 1},
		{"Deck of Illusions (full)", 1},
	}
	for _, tt := range tests {
		if got := BatchQuantity(tt.name); got != tt.want {
			t.Errorf("BatchQuantity(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestUnitName(t *testing.T) {
	scraped := func(s string) *string { return &s }
	items := []struct {
		item Item
		want string
	}{
		{Item{Name: "Arrows +3 (20)", ScrapedName: scraped("+3 Arrow")}, "+3 Arrow"},
		{Item{Name: "Ammunition of Slaying, Aberrations (10)", ScrapedName: scraped("Arrow of Slaying")}, "Arrow of Slaying"},
		{Item{Name: "Arrows (20)", ScrapedName: scraped("Arrows (20)")}, "Arrows"},
		{Item{Name: "Caltrops (20)", ScrapedName: scraped("Caltrops (bag of 20)")}, "Caltrops"},
		{Item{Name: "Longsword"}, "Longsword"},
	}
	for _, tt := range items {
		if got := tt.item.UnitName(); got != tt.want {
			t.Errorf("UnitName(%q) = %q, want %q", tt.item.Name, got, tt.want)
		}
	}
}

func TestOutputQty(t *testing.T) {
	var nilRecipe *Recipe
	if got := nilRecipe.OutputQty(); got != 1 {
		t.Fatalf("nil recipe output quantity = %d, want 1", got)
	}
	if got := (&Recipe{}).OutputQty(); got != 1 {
		t.Fatalf("zero output quantity = %d, want 1", got)
	}
	if got := (&Recipe{OutputQuantity: 20}).OutputQty(); got != 20 {
		t.Fatalf("output quantity = %d, want 20", got)
	}
}

func TestTier(t *testing.T) {
	if got := TierFromName("Arrows +3 (20)"); got != 3 {
		t.Fatalf("TierFromName = %d, want 3", got)
	}
	if got := TierFromName("+2 Greataxe"); got != 2 {
		t.Fatalf("TierFromName = %d, want 2", got)
	}
	if got := TierFromName("Longsword"); got != 0 {
		t.Fatalf("TierFromName = %d, want 0", got)
	}
	tier := 4
	it := Item{Name: "Weapon +1", Archetype: &Archetype{Tier: &tier}}
	if got := it.Tier(); got != 4 {
		t.Fatalf("Item.Tier = %d, want archetype tier 4", got)
	}
}
