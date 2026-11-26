// Package catalog holds the canonical data types shared by the importer, the
// web app and the crafting calculator.
package catalog

import (
	"regexp"
	"strings"
)

var (
	batchQuantityRE = regexp.MustCompile(`\((\d+)\)\s*$`)
	tierInNameRE    = regexp.MustCompile(`(?:^|\s)\+(\d+)\b`)
)

// BatchQuantity reads the output count encoded in a catalog name. A trailing
// bare `(N)` is how the dataset spells "one craft makes N of these"
// ("Arrows (20)", "Bead of Force (7)"). Parentheses with any other content
// ("Luck Blade (3, Sword)") describe the item, not a batch.
func BatchQuantity(name string) int {
	m := batchQuantityRE.FindStringSubmatch(name)
	if m == nil {
		return 1
	}
	n := 0
	for _, r := range m[1] {
		n = n*10 + int(r-'0')
	}
	if n < 1 {
		return 1
	}
	return n
}

// StripBatchQuantity removes a trailing output count from a name.
func StripBatchQuantity(name string) string {
	return strings.TrimSpace(batchQuantityRE.ReplaceAllString(name, ""))
}

// TierFromName recovers the `+N` tier from a generated item name
// ("Arrows +3 (20)", "+2 Greataxe"). It returns 0 when the name has no tier.
func TierFromName(name string) int {
	m := tierInNameRE.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	n := 0
	for _, r := range m[1] {
		n = n*10 + int(r-'0')
	}
	return n
}

// UnitName is the name a crafted inventory instance carries. Batch items drop
// the output count, and the scraped singular name wins when it is clean:
// "Ammunition of Slaying, Aberrations (10)" becomes "Arrow of Slaying", while
// "Arrows +3 (20)" becomes "+3 Arrow".
func (it *Item) UnitName() string {
	if it == nil {
		return ""
	}
	if BatchQuantity(it.Name) <= 1 {
		return it.Name
	}
	if it.ScrapedName != nil {
		n := strings.TrimSpace(*it.ScrapedName)
		if n != "" && !strings.ContainsAny(n, "()") {
			return n
		}
	}
	return StripBatchQuantity(it.Name)
}

// Tier is the item's generated tier, from its archetype or, for named batch
// rows such as "Arrows +3 (20)", from its name.
func (it *Item) Tier() int {
	if it == nil {
		return 0
	}
	if it.Archetype != nil && it.Archetype.Tier != nil {
		return *it.Archetype.Tier
	}
	return TierFromName(it.Name)
}

// Form is one allowed input form. Kind is "raw", "item" or "generic".
type Form struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
	// Mundane marks a specific item requirement that only accepts a mundane
	// instance, not an already-enchanted variant made from the same base.
	Mundane bool `json:"mundane,omitempty"`
}

// Option is one homogeneous choice for a requirement group.
type Option struct {
	AnyRaw        bool     `json:"any_raw"`
	Forms         []Form   `json:"forms"`
	Traits        []string `json:"traits"`
	TagsAll       []string `json:"tags_all"`
	TagsAny       []string `json:"tags_any"`
	OutputTags    []string `json:"output_tags"`
	OutputTagsAny []string `json:"output_tags_any"`
	Tier          *int     `json:"tier"`
	Position      int      `json:"position,omitempty"`
}

// Group is one requirement group in a recipe.
type Group struct {
	Quantity    int      `json:"quantity"`
	QuantityMax *int     `json:"quantity_max"`
	MinRarity   string   `json:"min_rarity"`
	TraitMode   string   `json:"trait_mode"`
	TraitVar    *int     `json:"trait_var"`
	Options     []Option `json:"options"`
	Position    int      `json:"position"`
}

// Extra is a non-material requirement (spell, currency, narrative).
type Extra struct {
	Kind     string `json:"kind"`
	Quantity *int   `json:"quantity"`
	Raw      string `json:"raw"`
}

// BaseSelector points at a generic base category or a specific item.
type BaseSelector struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Recipe is the canonical recipe tree for one item.
type Recipe struct {
	Kind string `json:"kind"`
	// OutputQuantity is how many units one craft produces. Zero is treated as
	// one so older recipes keep their behaviour.
	OutputQuantity int           `json:"output_quantity"`
	Rarity         string        `json:"rarity"`
	BaseSelector   *BaseSelector `json:"base_selector,omitempty"`
	Groups         []Group       `json:"groups"`
	Extras         []Extra       `json:"extras"`
	Notes          []string      `json:"notes"`
}

// OutputQty is OutputQuantity with the implicit default of one.
func (r *Recipe) OutputQty() int {
	if r == nil || r.OutputQuantity < 1 {
		return 1
	}
	return r.OutputQuantity
}

// Archetype describes generated (tier or modifier) combos.
type Archetype struct {
	Kind string       `json:"kind"`
	Base BaseSelector `json:"base"`
	Tier *int         `json:"tier"`
}

// Item is one catalog row plus its optional recipe.
type Item struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Category         string     `json:"category"`
	Subtype          *string    `json:"subtype"`
	TypeRaw          string     `json:"type_raw"`
	Rarity           string     `json:"rarity"`
	Magic            bool       `json:"magic"`
	Visible          bool       `json:"visible"`
	Source           string     `json:"source"`
	ShortDescription *string    `json:"short_description"`
	VariantOf        *string    `json:"variant_of"`
	VariantLabel     *string    `json:"variant_label"`
	Attunement       *string    `json:"attunement"`
	Damage           *string    `json:"damage"`
	Properties       *string    `json:"properties"`
	Weight           *string    `json:"weight"`
	Value            *string    `json:"value"`
	Description      *string    `json:"description"`
	ScrapedName      *string    `json:"scraped_name"`
	StatSourceCode   *string    `json:"stat_source_code"`
	StatSource       *string    `json:"stat_source"`
	MatchStatus      string     `json:"match_status"`
	MatchMethod      *string    `json:"match_method"`
	MatchDetail      *string    `json:"match_detail"`
	MatchConfidence  *float64   `json:"match_confidence"`
	IntrinsicTags    []string   `json:"intrinsic_tags"`
	Archetype        *Archetype `json:"archetype"`
	Template         bool       `json:"template"`
	Recipe           *Recipe    `json:"recipe"`
	Warnings         []string   `json:"warnings"`
}

// RarityRank orders rarities for min/max comparisons; unknown values rank 0.
func RarityRank(r string) int {
	switch r {
	case "Common":
		return 1
	case "Uncommon":
		return 2
	case "Rare":
		return 3
	case "Very Rare":
		return 4
	case "Legendary":
		return 5
	}
	return 0
}
