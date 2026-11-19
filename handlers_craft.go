package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi"

	"github.com/Nico-DM/craftingo/internal/catalog"
	"github.com/Nico-DM/craftingo/internal/crafting"
)

func (cfg *apiConfig) handleCraft(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	it, ok := cfg.Store.Item(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if it.Recipe == nil || it.Recipe.Kind != "groups" {
		if it.Archetype != nil {
			http.Redirect(w, r, "/generate/"+url.PathEscape(it.ID), http.StatusSeeOther) // #nosec G710 -- catalog id
			return
		}
		cfg.render(w, r, "craft.html", map[string]any{"Title": it.Name, "Item": it, "NoRecipe": true})
		return
	}
	units, err := cfg.loadUnits(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	calc := &crafting.Calculator{Store: cfg.Store}
	res := calc.Evaluate(it, units, 3)
	cfg.render(w, r, "craft.html", map[string]any{
		"Title":   "Craft " + it.Name,
		"Item":    it,
		"Result":  res,
		"Units":   units,
		"Checked": r.Method == http.MethodPost || r.URL.Query().Get("check") != "",
	})
}

func (cfg *apiConfig) handleGenerateIndex(w http.ResponseWriter, r *http.Request) {
	cfg.render(w, r, "generate.html", map[string]any{
		"Title":     "Generator",
		"Templates": cfg.Store.Templates(),
		"Modifiers": cfg.Store.Modifiers(),
	})
}

func (cfg *apiConfig) handleGenerateArchetype(w http.ResponseWriter, r *http.Request) {
	arch, ok := cfg.Store.Item(chi.URLParam(r, "id"))
	if !ok || arch.Archetype == nil {
		http.NotFound(w, r)
		return
	}
	bases := cfg.Store.BaseCandidates(arch.Archetype.Base.Kind, arch.Archetype.Base.Name)
	cfg.render(w, r, "generate_base.html", map[string]any{
		"Title": "Generate " + arch.Name,
		"Arch":  arch,
		"Bases": bases,
	})
}

func (cfg *apiConfig) handleGenerateCombo(w http.ResponseWriter, r *http.Request) {
	arch, ok := cfg.Store.Item(chi.URLParam(r, "id"))
	if !ok || arch.Archetype == nil {
		http.NotFound(w, r)
		return
	}
	base, ok := cfg.Store.Item(chi.URLParam(r, "base"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	combo := composeCombo(arch, base)

	var res crafting.Result
	checked := false
	if _, ok := currentUser(r); ok {
		if units, err := cfg.loadUnits(r); err == nil {
			checked = true
			if combo.Recipe != nil && combo.Recipe.Kind == "groups" {
				calc := &crafting.Calculator{Store: cfg.Store}
				res = calc.Evaluate(combo, units, 3)
			}
		}
	}
	cfg.render(w, r, "generate_combo.html", map[string]any{
		"Title":   arch.Name + " " + base.Name,
		"Arch":    arch,
		"Base":    base,
		"Combo":   combo,
		"Result":  res,
		"Checked": checked,
	})
}

// composeCombo builds the generated item page for a base + archetype pair.
//
// Tier archetypes ("Weapon +1") keep the base recipe and add the tier label.
// Modifier archetypes ("Vicious Weapon") replace their generic "+2 Weapon"
// requirement with the concrete base, so a Vicious Greataxe is crafted from a
// +2 Greataxe rather than being flattened into generic materials.
func composeCombo(arch, base *catalog.Item) *catalog.Item {
	combo := *base
	combo.ID = arch.ID + ":" + base.ID
	combo.Name = arch.Name + " " + base.Name
	combo.Rarity = arch.Rarity
	combo.Archetype = arch.Archetype
	combo.Template = false
	combo.ShortDescription = base.ShortDescription
	combo.VariantOf = nil
	combo.VariantLabel = nil

	if arch.Recipe == nil {
		combo.Recipe = nil
		return &combo
	}
	if arch.Recipe.Kind == "inherit" {
		combo.Recipe = cloneRecipe(base.Recipe)
		if combo.Recipe != nil {
			combo.Recipe.AddNotes("Tier " + arch.Name + " applied to " + base.Name)
		}
		return &combo
	}

	combo.Recipe = cloneRecipe(arch.Recipe)
	if combo.Recipe == nil {
		return &combo
	}
	combo.Recipe.AddNotes("Base: " + base.Name + " (craft the tiered base first)")
	baseCategory := ""
	if arch.Archetype != nil {
		baseCategory = arch.Archetype.Base.Name
	}
	for gi := range combo.Recipe.Groups {
		g := &combo.Recipe.Groups[gi]
		for oi := range g.Options {
			o := &g.Options[oi]
			for fi := range o.Forms {
				f := &o.Forms[fi]
				if f.Kind == "generic" && strings.EqualFold(f.Name, baseCategory) {
					o.Forms[fi] = catalog.Form{Kind: "item", Name: base.Name, ID: base.ID}
				}
			}
		}
	}
	return &combo
}

func cloneRecipe(r *catalog.Recipe) *catalog.Recipe {
	if r == nil {
		return nil
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return nil
	}
	var out catalog.Recipe
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return &out
}
