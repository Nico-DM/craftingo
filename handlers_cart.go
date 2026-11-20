package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi"
	"github.com/google/uuid"

	"github.com/Nico-DM/craftingo/internal/catalog"
	"github.com/Nico-DM/craftingo/internal/crafting"
	"github.com/Nico-DM/craftingo/internal/database"
)

type cartLineView struct {
	ID             string
	Item           *catalog.Item
	Plan           crafting.Plan
	PreferredTrait string
	TiedTraits     []string
}

// resolveCartItem accepts a catalog id, or "archetypeID:baseID" for a generated
// combo that is not itself stored in the catalog.
func (cfg *apiConfig) resolveCartItem(key string) (*catalog.Item, bool) {
	if it, ok := cfg.Store.Item(key); ok {
		return it, true
	}
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return nil, false
	}
	arch, ok := cfg.Store.Item(parts[0])
	if !ok || arch.Archetype == nil {
		return nil, false
	}
	base, ok := cfg.Store.Item(parts[1])
	if !ok {
		return nil, false
	}
	return composeCombo(arch, base), true
}

// cartState replays the cart in order. Each line's output is appended to the
// working set so later lines can use it, while the units it consumed disappear
// from that working set.
func (cfg *apiConfig) cartState(r *http.Request) ([]crafting.Unit, []cartLineView, error) {
	units, err := cfg.loadUnits(r)
	if err != nil {
		return nil, nil, err
	}
	u, _ := currentUser(r)
	rows, err := cfg.Q.ListCartLines(r.Context(), u.ID)
	if err != nil {
		return nil, nil, err
	}
	calc := &crafting.Calculator{Store: cfg.Store}
	working := units
	lines := make([]cartLineView, 0, len(rows))
	for _, row := range rows {
		line := cartLineView{ID: row.ID}
		it, ok := cfg.resolveCartItem(row.ItemID)
		if !ok {
			line.Plan.Error = "This catalog item no longer exists."
			lines = append(lines, line)
			continue
		}
		line.Item = it
		line.Plan = calc.PlanWithTrait(it, working, row.PreferredTrait)
		line.PreferredTrait = row.PreferredTrait
		line.TiedTraits = tiedTraits(line.Plan.TraitTotals)
		if line.Plan.Valid {
			working = crafting.ApplyPlan(working, line.Plan)
			out := line.Plan.Output
			out.ID = virtualUnitID(row.ID)
			working = append(working, out)
		}
		lines = append(lines, line)
	}
	return units, lines, nil
}

func virtualUnitID(lineID string) string { return "cart:" + lineID }

// tiedTraits lists the traits sharing the highest total. When more than one
// trait ties, the crafter picks which one the finished item keeps.
func tiedTraits(totals map[string]int) []string {
	best := 0
	for trait, total := range totals {
		if trait != "" && total > best {
			best = total
		}
	}
	var out []string
	for trait, total := range totals {
		if trait != "" && total == best {
			out = append(out, trait)
		}
	}
	if len(out) < 2 {
		return nil
	}
	sort.Strings(out)
	return out
}

// applyCartLines removes consumed units from an inventory snapshot and appends
// the virtual outputs of valid cart lines, in order.
func applyCartLines(units []crafting.Unit, lines []cartLineView) []crafting.Unit {
	working := units
	for _, line := range lines {
		if !line.Plan.Valid {
			continue
		}
		working = crafting.ApplyPlan(working, line.Plan)
		out := line.Plan.Output
		out.ID = virtualUnitID(line.ID)
		working = append(working, out)
	}
	return working
}

func (cfg *apiConfig) handleCartAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	itemID := strings.TrimSpace(r.FormValue("item_id"))
	it, ok := cfg.resolveCartItem(itemID)
	if !ok || it.Recipe == nil || it.Recipe.Kind != "groups" {
		redirectErr(w, r, "/inventory?error=That+item+cannot+be+crafted")
		return
	}
	_, lines, err := cfg.cartState(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Rebuild the working set with every existing line applied, then check the
	// new line against what is left.
	working, err := cfg.loadUnits(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	working = applyCartLines(working, lines)
	if plan := (&crafting.Calculator{Store: cfg.Store}).Plan(it, working); !plan.Valid {
		redirectErr(w, r, "/inventory?error="+urlMsg("Not enough left for "+it.Name+": "+plan.Error))
		return
	}

	u, _ := currentUser(r)
	rows, err := cfg.Q.ListCartLines(r.Context(), u.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := cfg.Q.CreateCartLine(r.Context(), database.CreateCartLineParams{
		ID:             uuid.NewString(),
		OwnerID:        u.ID,
		ItemID:         itemID,
		Position:       int64(len(rows) + 1),
		PreferredTrait: "",
		CreatedAt:      now(),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/inventory#cart", http.StatusSeeOther)
}

func (cfg *apiConfig) handleCartDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if err := cfg.Q.DeleteCartLine(r.Context(), database.DeleteCartLineParams{
		ID: chi.URLParam(r, "id"), OwnerID: u.ID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/inventory#cart", http.StatusSeeOther)
}

// handleCartTrait records the crafter's tie-break choice for a cart line.
func (cfg *apiConfig) handleCartTrait(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id := chi.URLParam(r, "id")
	trait := strings.TrimSpace(r.FormValue("trait"))

	_, lines, err := cfg.cartState(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var line *cartLineView
	for i := range lines {
		if lines[i].ID == id {
			line = &lines[i]
			break
		}
	}
	if line == nil {
		http.NotFound(w, r)
		return
	}
	if trait != "" && !containsString(line.TiedTraits, trait) {
		redirectErr(w, r, "/inventory?error="+urlMsg("That trait is not tied for this craft"))
		return
	}
	if err := cfg.Q.SetCartLineTrait(r.Context(), database.SetCartLineTraitParams{
		PreferredTrait: trait, ID: id, OwnerID: u.ID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/inventory#cart", http.StatusSeeOther)
}

func (cfg *apiConfig) handleCartClear(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if err := cfg.Q.ClearCart(r.Context(), u.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/inventory#cart", http.StatusSeeOther)
}

func (cfg *apiConfig) handleCartFinalize(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	units, lines, err := cfg.cartState(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(lines) == 0 {
		redirectErr(w, r, "/inventory?error=Your+crafting+cart+is+empty")
		return
	}
	for _, line := range lines {
		if line.Item == nil || !line.Plan.Valid {
			name := "a cart line"
			if line.Item != nil {
				name = line.Item.Name
			}
			redirectErr(w, r, "/inventory?error="+urlMsg("Cannot craft "+name+": "+line.Plan.Error))
			return
		}
	}

	// Aggregate the final amount taken from every real inventory row. Virtual
	// outputs are simply not inserted when a later line consumes them.
	remaining := make(map[string]int, len(units))
	for _, unit := range units {
		remaining[unit.ID] = unit.Quantity
	}
	consumedVirtual := map[string]bool{}
	for _, line := range lines {
		for _, a := range line.Plan.Consumed {
			if strings.HasPrefix(a.UnitID, "cart:") {
				consumedVirtual[a.UnitID] = true
				continue
			}
			remaining[a.UnitID] -= a.Quantity
		}
	}

	tx, err := cfg.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()
	q := database.For(tx)

	for _, unit := range units {
		left, ok := remaining[unit.ID]
		if !ok || left == unit.Quantity {
			continue
		}
		if left <= 0 {
			if err := q.DeleteInventoryUnit(r.Context(), database.DeleteInventoryUnitParams{
				ID: unit.ID, OwnerID: u.ID,
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			continue
		}
		if err := q.UpdateInventoryQuantity(r.Context(), database.UpdateInventoryQuantityParams{
			Quantity: int64(left), UpdatedAt: now(), ID: unit.ID, OwnerID: u.ID,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	for _, line := range lines {
		out := line.Plan.Output
		if consumedVirtual[virtualUnitID(line.ID)] {
			continue
		}
		tagJSON, _ := json.Marshal(out.Tags)
		if _, err := q.CreateInventoryUnit(r.Context(), database.CreateInventoryUnitParams{
			ID:        uuid.NewString(),
			OwnerID:   u.ID,
			Kind:      "crafted",
			FormKind:  "item",
			FormName:  out.FormName,
			ItemID:    nullString(strPtrOrNil(out.ItemID)),
			Name:      out.Name,
			Rarity:    out.Rarity,
			Trait:     nullString(strPtrOrNil(out.Trait)),
			TraitQty:  int64(out.TraitQty),
			Tier:      int64(out.Tier),
			Magic:     boolInt(out.Magic),
			Tags:      string(tagJSON),
			Quantity:  1,
			Note:      nullString(nil),
			CreatedAt: now(),
			UpdatedAt: now(),
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := q.ClearCart(r.Context(), u.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/inventory?crafted=1", http.StatusSeeOther)
}
