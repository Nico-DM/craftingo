// Package catalog holds the canonical data types shared by the importer, the
// web app and the crafting calculator.
package catalog

// Form is one allowed input form. Kind is "raw", "item" or "generic".
type Form struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
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
	Kind         string        `json:"kind"`
	Rarity       string        `json:"rarity"`
	BaseSelector *BaseSelector `json:"base_selector,omitempty"`
	Groups       []Group       `json:"groups"`
	Extras       []Extra       `json:"extras"`
	Notes        []string      `json:"notes"`
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
	Oota             bool       `json:"oota"`
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
