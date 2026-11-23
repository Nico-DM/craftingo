package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/google/uuid"

	"github.com/Nico-DM/craftingo/internal/catalog"
	"github.com/Nico-DM/craftingo/internal/crafting"
	"github.com/Nico-DM/craftingo/internal/database"
)

var rawForms = []string{"Bone", "Fluid", "Food", "Glass", "Hide", "Metal", "Mineral", "Textile", "Wood"}

var traits = []string{
	"Acidic", "Chaotic", "Cold", "Dimensional", "Fiendish", "Fiery", "Forceful",
	"Inert", "Lightning", "Mercurial", "Necrotic", "Phantasmal", "Poisonous",
	"Psychic", "Radiant", "Theurgic", "Thundering", "Vital", "Void", "Zephyrous",
}

var rarities = []string{"Common", "Uncommon", "Rare", "Very Rare", "Legendary"}

func (cfg *apiConfig) loadUnits(r *http.Request) ([]crafting.Unit, error) {
	u, _ := currentUser(r)
	rows, err := cfg.Q.ListInventory(r.Context(), u.ID)
	if err != nil {
		return nil, err
	}
	out := make([]crafting.Unit, 0, len(rows))
	for _, row := range rows {
		unit := crafting.Unit{
			ID:       row.ID,
			Kind:     row.Kind,
			FormKind: row.FormKind,
			FormName: row.FormName,
			ItemID:   row.ItemID.String,
			Name:     row.Name,
			Rarity:   row.Rarity,
			Trait:    row.Trait.String,
			TraitQty: int(row.TraitQty),
			Tier:     int(row.Tier),
			Magic:    row.Magic == 1,
			Quantity: int(row.Quantity),
		}
		_ = json.Unmarshal([]byte(row.Tags), &unit.Tags)
		if unit.ItemID != "" {
			if it, ok := cfg.Store.Item(unit.ItemID); ok {
				unit.Category = it.Category
				if it.Subtype != nil {
					unit.Subtype = *it.Subtype
				}
				unit.Magic = unit.Magic || it.Magic
				if unit.Tier == 0 && it.Archetype != nil && it.Archetype.Tier != nil {
					unit.Tier = *it.Archetype.Tier
				}
			}
		}
		out = append(out, unit)
	}
	return out, nil
}

func (cfg *apiConfig) handleInventory(w http.ResponseWriter, r *http.Request) {
	units, cart, err := cfg.cartState(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	mform, mtrait, mrarity := q.Get("mform"), q.Get("mtrait"), q.Get("mrarity")
	var materials, crafted []crafting.Unit
	totalQty := 0
	for _, u := range units {
		totalQty += u.Quantity
		if u.Kind == "material" {
			if mform != "" && !strings.EqualFold(u.FormName, mform) {
				continue
			}
			if mtrait != "" {
				trait := u.Trait
				if trait == "" {
					trait = "Basic"
				}
				if !strings.EqualFold(trait, mtrait) {
					continue
				}
			}
			if mrarity != "" && !strings.EqualFold(u.Rarity, mrarity) {
				continue
			}
			materials = append(materials, u)
			continue
		}
		crafted = append(crafted, u)
	}

	msort := q.Get("msort")
	mdir := q.Get("mdir")
	csort := q.Get("csort")
	cdir := q.Get("cdir")
	sortUnits(materials, msort, mdir)
	sortUnits(crafted, csort, cdir)

	includeHidden := isDM(r)
	calc := &crafting.Calculator{Store: cfg.Store, VisibleOnly: !includeHidden}
	working := applyCartLines(units, cart)
	craftable := calc.Craftable(working, 40)
	cartValid := true
	for _, line := range cart {
		if !line.Plan.Valid {
			cartValid = false
			break
		}
	}
	cfg.render(w, r, "inventory.html", map[string]any{
		"Title":     "Inventory",
		"Materials": materials,
		"Crafted":   crafted,
		"TotalQty":  totalQty,
		"Forms":     rawForms,
		"Traits":    traits,
		"Rarities":  rarities,
		"Items":     filterVisibleItems(cfg.Store.Items(), includeHidden),
		"Craftable": craftable,
		"Cart":      cart,
		"CartValid": cartValid,
		"CartCount": len(cart),
		"Filter":    map[string]string{"MForm": mform, "MTrait": mtrait, "MRarity": mrarity, "MSort": msort, "MDir": mdir, "CSort": csort, "CDir": cdir},
		"Error":     r.URL.Query().Get("error"),
		"CraftedOK": r.URL.Query().Get("crafted") != "",
	})
}

func (cfg *apiConfig) handleInventoryAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	u, _ := currentUser(r)
	kind := r.FormValue("kind")
	name := strings.TrimSpace(r.FormValue("name"))
	formName := strings.TrimSpace(r.FormValue("form_name"))
	rarity := r.FormValue("rarity")
	trait := strings.TrimSpace(r.FormValue("trait"))
	tags := splitTags(r.FormValue("tags"))
	tier, _ := strconv.Atoi(r.FormValue("tier"))
	traitQty, _ := strconv.Atoi(r.FormValue("trait_qty"))
	quantity, _ := strconv.Atoi(r.FormValue("quantity"))
	if quantity < 1 {
		quantity = 1
	}
	formKind := "raw"
	if kind == "material" {
		name = formName
		tier = 0
	} else {
		kind = "crafted"
		formKind = "item"
		if name == "" {
			redirectErr(w, r, "/inventory?error=Give+the+crafted+item+a+name")
			return
		}
		formName = name
		quantity = 1
	}
	if name == "" {
		redirectErr(w, r, "/inventory?error=Pick+a+form")
		return
	}
	if !containsString(rawForms, formName) && kind == "material" {
		redirectErr(w, r, "/inventory?error=Unknown+raw+form")
		return
	}
	var itemID *string
	var category string
	magic := false
	if it, ok := cfg.Store.ItemByName(name); ok && kind == "crafted" {
		if !isDM(r) && !it.Visible {
			redirectErr(w, r, "/inventory?error=That+item+is+hidden")
			return
		}
		itemID = &it.ID
		category = it.Category
		magic = it.Magic
		if rarity == "" {
			rarity = it.Rarity
		}
		if tier == 0 && it.Archetype != nil && it.Archetype.Tier != nil {
			tier = *it.Archetype.Tier
		}
		// A crafted item inherits its name/type tags (Sword, Axe, ...), which
		// recipes use to narrow a generic Weapon/Armor base.
		for _, tag := range it.IntrinsicTags {
			if !containsString(tags, tag) {
				tags = append(tags, tag)
			}
		}
	}
	if rarity == "" {
		rarity = "Common"
	}
	if kind == "crafted" && tier > 0 {
		magic = true
	}
	if trait != "" && traitQty == 0 {
		traitQty = 1
	}
	tagJSON, _ := json.Marshal(tags)
	note := strings.TrimSpace(r.FormValue("note"))
	if kind == "material" {
		// Materials stack by form + rarity + trait + tags.
		existing, _ := cfg.Q.ListInventory(r.Context(), u.ID)
		for _, row := range existing {
			if row.Kind == "material" && row.FormName == formName && row.Rarity == rarity &&
				row.Trait.String == trait && row.Tags == string(tagJSON) {
				_ = cfg.Q.UpdateInventoryQuantity(r.Context(), database.UpdateInventoryQuantityParams{
					Quantity: row.Quantity + int64(quantity), UpdatedAt: now(),
					ID: row.ID, OwnerID: u.ID,
				})
				if note != "" {
					merged := note
					if row.Note.Valid && row.Note.String != "" && row.Note.String != note {
						if strings.Contains(row.Note.String, note) {
							merged = row.Note.String
						} else {
							merged = row.Note.String + "; " + note
						}
					}
					if merged != row.Note.String {
						_ = cfg.Q.UpdateInventoryNote(r.Context(), database.UpdateInventoryNoteParams{
							Note: nullString(strPtrOrNil(merged)), UpdatedAt: now(),
							ID: row.ID, OwnerID: u.ID,
						})
					}
				}
				http.Redirect(w, r, "/inventory", http.StatusSeeOther)
				return
			}
		}
	}
	_, err := cfg.Q.CreateInventoryUnit(r.Context(), database.CreateInventoryUnitParams{
		ID:        uuid.NewString(),
		OwnerID:   u.ID,
		Kind:      kind,
		FormKind:  formKind,
		FormName:  formName,
		ItemID:    nullString(itemID),
		Name:      name,
		Rarity:    rarity,
		Trait:     nullString(strPtrOrNil(trait)),
		TraitQty:  int64(traitQty),
		Tier:      int64(tier),
		Magic:     boolInt(magic),
		Tags:      string(tagJSON),
		Quantity:  int64(quantity),
		Note:      nullString(strPtrOrNil(note)),
		CreatedAt: now(),
		UpdatedAt: now(),
	})
	if err != nil {
		redirectErr(w, r, "/inventory?error="+urlMsg(err.Error()))
		return
	}
	_ = category
	http.Redirect(w, r, "/inventory", http.StatusSeeOther)
}

func (cfg *apiConfig) handleInventoryQuantity(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	qty, _ := strconv.Atoi(r.FormValue("quantity"))
	id := chi.URLParam(r, "id")
	if qty <= 0 {
		_ = cfg.Q.DeleteInventoryUnit(r.Context(), database.DeleteInventoryUnitParams{ID: id, OwnerID: u.ID})
		http.Redirect(w, r, "/inventory", http.StatusSeeOther)
		return
	}
	_ = cfg.Q.UpdateInventoryQuantity(r.Context(), database.UpdateInventoryQuantityParams{
		Quantity: int64(qty), UpdatedAt: now(), ID: id, OwnerID: u.ID,
	})
	http.Redirect(w, r, "/inventory", http.StatusSeeOther)
}

func (cfg *apiConfig) handleInventoryDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	_ = cfg.Q.DeleteInventoryUnit(r.Context(), database.DeleteInventoryUnitParams{
		ID: chi.URLParam(r, "id"), OwnerID: u.ID,
	})
	http.Redirect(w, r, "/inventory", http.StatusSeeOther)
}

func splitTags(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		t := strings.TrimSpace(part)
		if t != "" && !containsString(out, t) {
			out = append(out, t)
		}
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func redirectErr(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusSeeOther) // #nosec G710 -- callers pass local paths
}

func (cfg *apiConfig) handleInventoryUnit(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	row, err := cfg.Q.GetInventoryUnit(r.Context(), database.GetInventoryUnitParams{
		ID: chi.URLParam(r, "id"), OwnerID: u.ID,
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	unit := crafting.Unit{
		ID: row.ID, Kind: row.Kind, FormKind: row.FormKind, FormName: row.FormName,
		ItemID: row.ItemID.String, Name: row.Name, Rarity: row.Rarity,
		Trait: row.Trait.String, TraitQty: int(row.TraitQty), Tier: int(row.Tier),
		Quantity: int(row.Quantity),
	}
	_ = json.Unmarshal([]byte(row.Tags), &unit.Tags)
	var item *catalog.Item
	if unit.ItemID != "" {
		if found, ok := cfg.Store.Item(unit.ItemID); ok {
			item = found
			unit.Category = found.Category
			unit.Magic = found.Magic
			if unit.Tier == 0 && found.Archetype != nil && found.Archetype.Tier != nil {
				unit.Tier = *found.Archetype.Tier
			}
		}
	}
	includeHidden := isDM(r)
	if item != nil && !includeHidden && !item.Visible {
		item = nil
	}
	var res crafting.Result
	checked := false
	if item != nil && item.Recipe != nil && item.Recipe.Kind == "groups" {
		if units, err := cfg.loadUnits(r); err == nil {
			checked = true
			res = (&crafting.Calculator{Store: cfg.Store, VisibleOnly: !includeHidden}).Evaluate(item, units, 0)
		}
	}
	cfg.render(w, r, "inventory_unit.html", map[string]any{
		"Title":   row.Name,
		"Unit":    unit,
		"Item":    item,
		"Note":    row.Note.String,
		"Created": row.CreatedAt,
		"Result":  res,
		"Checked": checked,
	})
}
