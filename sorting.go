package main

import (
	"sort"
	"strings"

	"github.com/Nico-DM/craftingo/internal/catalog"
	"github.com/Nico-DM/craftingo/internal/crafting"
)

// compareMagic orders mundane items before magic ones.
func compareMagic(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}

// compareRarity orders Common -> Uncommon -> Rare -> Very Rare -> Legendary.
func compareRarity(a, b string) int {
	ra, rb := catalog.RarityRank(a), catalog.RarityRank(b)
	switch {
	case ra < rb:
		return -1
	case ra > rb:
		return 1
	}
	return 0
}

func compareFold(a, b string) int {
	return strings.Compare(strings.ToLower(a), strings.ToLower(b))
}

func itemSubtype(it catalog.Item) string {
	if it.Subtype == nil {
		return ""
	}
	return *it.Subtype
}

// compareItemCategory orders category, then subtype inside that category, then
// name.
func compareItemCategory(a, b catalog.Item) int {
	if c := compareFold(a.Category, b.Category); c != 0 {
		return c
	}
	if c := compareFold(itemSubtype(a), itemSubtype(b)); c != 0 {
		return c
	}
	return compareFold(a.Name, b.Name)
}

// compareDefaultItems is the "Default" order: mundane before magic, then
// Common to Legendary, then category/subtype, then name.
func compareDefaultItems(a, b catalog.Item) int {
	if c := compareMagic(a.Magic, b.Magic); c != 0 {
		return c
	}
	if c := compareRarity(a.Rarity, b.Rarity); c != 0 {
		return c
	}
	return compareItemCategory(a, b)
}

// sortItems orders catalog rows. sortKey selects the primary property; the
// Default order is always the fallback. Direction "desc" reverses the whole
// resulting list.
func sortItems(items []catalog.Item, sortKey, direction string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		var c int
		switch strings.ToLower(sortKey) {
		case "name":
			c = compareFold(a.Name, b.Name)
		case "category":
			c = compareItemCategory(a, b)
		case "rarity":
			c = compareRarity(a.Rarity, b.Rarity)
		case "source":
			c = compareFold(a.Source, b.Source)
		case "magic":
			c = compareMagic(a.Magic, b.Magic)
		}
		if c == 0 {
			c = compareDefaultItems(a, b)
		}
		return c < 0
	})
	if isReverse(direction) {
		reverseItems(items)
	}
}

func reverseItems(items []catalog.Item) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func compareDefaultUnits(a, b crafting.Unit) int {
	if c := compareMagic(a.Magic, b.Magic); c != 0 {
		return c
	}
	if c := compareRarity(a.Rarity, b.Rarity); c != 0 {
		return c
	}
	if c := compareFold(a.Category, b.Category); c != 0 {
		return c
	}
	if c := compareFold(a.Subtype, b.Subtype); c != 0 {
		return c
	}
	return compareFold(unitDisplayName(a), unitDisplayName(b))
}

// unitDisplayName avoids repeating the form name for raw materials.
func unitDisplayName(u crafting.Unit) string {
	if u.Kind == "material" && strings.EqualFold(u.Name, u.FormName) {
		return u.FormName
	}
	if u.Name == "" {
		return u.FormName
	}
	return u.Name
}

func sortUnits(units []crafting.Unit, sortKey, direction string) {
	sort.SliceStable(units, func(i, j int) bool {
		a, b := units[i], units[j]
		var c int
		switch strings.ToLower(sortKey) {
		case "name":
			c = compareFold(unitDisplayName(a), unitDisplayName(b))
		case "form":
			c = compareFold(a.FormName, b.FormName)
		case "trait":
			c = compareFold(a.Trait, b.Trait)
		case "qty":
			switch {
			case a.Quantity > b.Quantity:
				c = -1
			case a.Quantity < b.Quantity:
				c = 1
			}
		case "tier":
			switch {
			case a.Tier > b.Tier:
				c = -1
			case a.Tier < b.Tier:
				c = 1
			}
		case "category":
			c = compareFold(a.Category, b.Category)
			if c == 0 {
				c = compareFold(a.Subtype, b.Subtype)
			}
			if c == 0 {
				c = compareFold(unitDisplayName(a), unitDisplayName(b))
			}
		case "rarity":
			c = compareRarity(a.Rarity, b.Rarity)
		}
		if c == 0 {
			c = compareDefaultUnits(a, b)
		}
		return c < 0
	})
	if isReverse(direction) {
		reverseUnits(units)
	}
}

func reverseUnits(units []crafting.Unit) {
	for i, j := 0, len(units)-1; i < j; i, j = i+1, j-1 {
		units[i], units[j] = units[j], units[i]
	}
}

func isReverse(direction string) bool {
	return strings.EqualFold(direction, "desc") || strings.EqualFold(direction, "reverse")
}
