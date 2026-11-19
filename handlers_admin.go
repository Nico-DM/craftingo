package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/google/uuid"

	"github.com/Nico-DM/craftingo/internal/catalog"
	"github.com/Nico-DM/craftingo/internal/database"
)

func (cfg *apiConfig) handleAdmin(w http.ResponseWriter, r *http.Request) {
	items := cfg.Store.Items()
	itemsCount, _ := cfg.Q.CountCatalog(r.Context())
	recipesCount, _ := cfg.Q.CountRecipes(r.Context())
	users, _ := cfg.Q.ListUsers(r.Context())
	var unmatched int
	for _, it := range items {
		if it.MatchStatus != "matched" {
			unmatched++
		}
	}
	cfg.render(w, r, "admin.html", map[string]any{
		"Title":     "DM dashboard",
		"Items":     itemsCount,
		"Recipes":   recipesCount,
		"Users":     len(users),
		"Unmatched": unmatched,
		"Loaded":    len(items),
	})
}

func (cfg *apiConfig) handleAdminItems(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	items := cfg.Store.Items()
	var out []catalog.Item
	for _, it := range items {
		if q == "" || strings.Contains(strings.ToLower(it.Name), strings.ToLower(q)) {
			out = append(out, it)
		}
	}
	cfg.render(w, r, "admin_items.html", map[string]any{
		"Title": "Edit items",
		"Q":     q,
		"Items": out,
	})
}

func (cfg *apiConfig) handleAdminItem(w http.ResponseWriter, r *http.Request) {
	it, ok := cfg.Store.Item(chi.URLParam(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	recipeJSON := "null"
	if it.Recipe != nil {
		raw, _ := json.MarshalIndent(it.Recipe, "", "  ")
		recipeJSON = string(raw)
	}
	tags, _ := json.Marshal(it.IntrinsicTags)
	cfg.render(w, r, "admin_item.html", map[string]any{
		"Title":      "Edit " + it.Name,
		"Item":       it,
		"RecipeJSON": recipeJSON,
		"Tags":       string(tags),
		"Rarities":   rarities,
		"Error":      r.URL.Query().Get("error"),
		"Saved":      r.URL.Query().Get("saved") == "1",
		"UsedBy":     cfg.Store.UsedBy(it.ID),
	})
}

func (cfg *apiConfig) handleAdminItemSave(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	it, ok := cfg.Store.Item(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	tags := splitTags(r.FormValue("intrinsic_tags"))
	tagJSON, _ := json.Marshal(tags)
	params := database.UpdateItemMetaParams{
		Name:             strings.TrimSpace(r.FormValue("name")),
		Category:         strings.TrimSpace(r.FormValue("category")),
		Subtype:          nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("subtype")))),
		TypeRaw:          r.FormValue("type_raw"),
		Rarity:           r.FormValue("rarity"),
		Magic:            boolInt(r.FormValue("magic") != ""),
		Oota:             boolInt(r.FormValue("oota") != ""),
		Source:           r.FormValue("source"),
		ShortDescription: nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("short_description")))),
		VariantOf:        nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("variant_of")))),
		VariantLabel:     nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("variant_label")))),
		Attunement:       nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("attunement")))),
		Damage:           nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("damage")))),
		Properties:       nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("properties")))),
		Weight:           nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("weight")))),
		Value:            nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("value")))),
		Description:      nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("description")))),
		IntrinsicTags:    string(tagJSON),
		IsTemplate:       boolInt(it.Template),
		UpdatedAt:        now(),
		ID:               id,
	}
	archKind := strings.TrimSpace(r.FormValue("archetype_kind"))
	if archKind != "" {
		params.ArchetypeKind = sql.NullString{String: archKind, Valid: true}
		params.ArchetypeBaseKind = nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("archetype_base_kind"))))
		params.ArchetypeBaseName = nullString(strPtrOrNil(strings.TrimSpace(r.FormValue("archetype_base_name"))))
		if tier, err := strconv.Atoi(r.FormValue("archetype_tier")); err == nil {
			params.ArchetypeTier = sql.NullInt64{Int64: int64(tier), Valid: true}
		}
	}
	if params.Name == "" {
		redirectErr(w, r, "/admin/items/"+id+"?error=Name+is+required")
		return
	}
	if err := cfg.Q.UpdateItemMeta(r.Context(), params); err != nil {
		redirectErr(w, r, "/admin/items/"+id+"?error="+urlMsg(err.Error()))
		return
	}
	if err := cfg.Store.Load(r.Context()); err != nil {
		redirectErr(w, r, "/admin/items/"+id+"?error="+urlMsg(err.Error()))
		return
	}
	http.Redirect(w, r, "/admin/items/"+url.PathEscape(id)+"?saved=1", http.StatusSeeOther) // #nosec G710 -- catalog id
}

func (cfg *apiConfig) handleAdminRecipeSave(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	it, ok := cfg.Store.Item(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(r.FormValue("recipe"))
	if raw == "" || raw == "null" {
		if err := cfg.Q.DeleteRecipeForItem(r.Context(), id); err != nil {
			redirectErr(w, r, "/admin/items/"+id+"?error="+urlMsg(err.Error()))
			return
		}
		_ = cfg.Store.Load(r.Context())
		http.Redirect(w, r, "/admin/items/"+url.PathEscape(id)+"?saved=1", http.StatusSeeOther) // #nosec G710 -- catalog id
		return
	}
	var recipe catalog.Recipe
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&recipe); err != nil {
		redirectErr(w, r, "/admin/items/"+id+"?error="+urlMsg("JSON: "+err.Error()))
		return
	}
	if err := normalizeRecipe(&recipe); err != nil {
		redirectErr(w, r, "/admin/items/"+id+"?error="+urlMsg(err.Error()))
		return
	}
	tx, err := cfg.DB.BeginTx(r.Context(), nil)
	if err != nil {
		redirectErr(w, r, "/admin/items/"+id+"?error="+urlMsg(err.Error()))
		return
	}
	defer tx.Rollback()
	q := cfg.Q.WithTx(tx)
	if err := catalog.SaveRecipe(r.Context(), q, id, &recipe, it.Rarity); err != nil {
		redirectErr(w, r, "/admin/items/"+id+"?error="+urlMsg(err.Error()))
		return
	}
	if err := tx.Commit(); err != nil {
		redirectErr(w, r, "/admin/items/"+id+"?error="+urlMsg(err.Error()))
		return
	}
	if err := cfg.Store.Load(r.Context()); err != nil {
		redirectErr(w, r, "/admin/items/"+id+"?error="+urlMsg(err.Error()))
		return
	}
	http.Redirect(w, r, "/admin/items/"+url.PathEscape(id)+"?saved=1", http.StatusSeeOther) // #nosec G710 -- catalog id
}

func normalizeRecipe(r *catalog.Recipe) error {
	if r.Kind == "" {
		r.Kind = "groups"
	}
	switch r.Kind {
	case "groups":
		if len(r.Groups) == 0 {
			return fmt.Errorf("groups recipe needs at least one group")
		}
		for gi := range r.Groups {
			g := &r.Groups[gi]
			if g.Position == 0 {
				g.Position = gi + 1
			}
			if g.Quantity < 1 {
				return fmt.Errorf("group %d: quantity must be positive", gi+1)
			}
			if g.TraitMode == "" {
				g.TraitMode = "fixed"
			}
			if g.TraitMode != "fixed" && g.TraitMode != "variable" {
				return fmt.Errorf("group %d: trait_mode must be fixed or variable", gi+1)
			}
			if len(g.Options) == 0 {
				return fmt.Errorf("group %d: needs at least one option", gi+1)
			}
			for oi := range g.Options {
				o := &g.Options[oi]
				o.Position = oi + 1
				if !o.AnyRaw && len(o.Forms) == 0 && len(o.Traits) == 0 {
					return fmt.Errorf("group %d option %d: needs a form or trait", gi+1, oi+1)
				}
				for fi := range o.Forms {
					f := &o.Forms[fi]
					switch f.Kind {
					case "raw", "generic":
					case "item":
						if f.ID == "" && f.Name != "" {
							// Resolve a name to a catalog id when the DM types one.
							if it, ok := catalogResolveName(f.Name); ok {
								f.ID = it.ID
								f.Name = it.Name
							}
						}
						if f.ID == "" {
							return fmt.Errorf("group %d option %d: unknown item %q", gi+1, oi+1, f.Name)
						}
					default:
						return fmt.Errorf("group %d option %d: unknown form kind %q", gi+1, oi+1, f.Kind)
					}
				}
			}
		}
	case "inherit":
		if r.BaseSelector == nil || r.BaseSelector.Name == "" {
			return fmt.Errorf("inherit recipe needs a base_selector")
		}
	default:
		return fmt.Errorf("recipe kind must be groups or inherit")
	}
	for i := range r.Extras {
		if r.Extras[i].Kind == "" {
			return fmt.Errorf("extra %d: kind is required", i+1)
		}
	}
	return nil
}

// catalogResolveName is replaced at startup with a store-backed resolver.
var catalogResolveName = func(string) (*catalog.Item, bool) { return nil, false }

func (cfg *apiConfig) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := cfg.Q.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.render(w, r, "admin_users.html", map[string]any{
		"Title": "Users",
		"Users": users,
		"Error": r.URL.Query().Get("error"),
	})
}

func (cfg *apiConfig) handleAdminUserCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	role := r.FormValue("role")
	if role != "dm" && role != "player" {
		redirectErr(w, r, "/admin/users?error=Pick+a+role")
		return
	}
	if len(username) < 2 || len(password) < 6 {
		redirectErr(w, r, "/admin/users?error=Username+or+password+too+short")
		return
	}
	hash, err := hashPassword(password)
	if err != nil {
		redirectErr(w, r, "/admin/users?error="+urlMsg(err.Error()))
		return
	}
	_, err = cfg.Q.CreateUser(r.Context(), database.CreateUserParams{
		ID: uuid.NewString(), Username: username, PasswordHash: hash, Role: role,
		CreatedAt: now(), UpdatedAt: now(),
	})
	if err != nil {
		redirectErr(w, r, "/admin/users?error="+urlMsg(err.Error()))
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (cfg *apiConfig) handleAdminUserDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	if chi.URLParam(r, "id") == u.ID {
		redirectErr(w, r, "/admin/users?error=You+cannot+delete+yourself")
		return
	}
	_ = cfg.Q.DeleteUser(r.Context(), chi.URLParam(r, "id"))
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (cfg *apiConfig) handleAdminReload(w http.ResponseWriter, r *http.Request) {
	if err := cfg.Store.Load(r.Context()); err != nil {
		redirectErr(w, r, "/admin?error="+urlMsg(err.Error()))
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
