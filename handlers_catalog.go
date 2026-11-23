package main

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi"

	"github.com/Nico-DM/craftingo/internal/catalog"
)

type listFilter struct {
	Q        string
	Category string
	Rarity   string
	Source   string
	Magic    string
	Sort     string
	Dir      string
	Page     int
	PerPage  int
	Total    int
}

func matchesFilter(it catalog.Item, f listFilter) bool {
	if f.Q != "" {
		q := strings.ToLower(f.Q)
		if !strings.Contains(strings.ToLower(it.Name), q) &&
			!strings.Contains(strings.ToLower(it.Category), q) &&
			!strings.Contains(strings.ToLower(deref(it.Subtype)), q) {
			return false
		}
	}
	if f.Category != "" && it.Category != f.Category {
		return false
	}
	if f.Rarity != "" && it.Rarity != f.Rarity {
		return false
	}
	if f.Source != "" && it.Source != f.Source {
		return false
	}
	if f.Magic == "yes" && !it.Magic {
		return false
	}
	if f.Magic == "no" && it.Magic {
		return false
	}
	return true
}

func (cfg *apiConfig) handleHome(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := listFilter{
		Q:        strings.TrimSpace(q.Get("q")),
		Category: q.Get("category"),
		Rarity:   q.Get("rarity"),
		Source:   q.Get("source"),
		Magic:    q.Get("magic"),
		Sort:     q.Get("sort"),
		Dir:      q.Get("dir"),
		Page:     1,
		PerPage:  60,
	}
	includeHidden := isDM(r)
	items := filterVisibleItems(cfg.Store.Items(), includeHidden)
	var filtered []catalog.Item
	for _, it := range items {
		if matchesFilter(it, f) {
			filtered = append(filtered, it)
		}
	}
	sortItems(filtered, f.Sort, f.Dir)
	f.Total = len(filtered)
	if p := q.Get("page"); p != "" {
		if n, err := parsePositiveInt(p); err == nil {
			f.Page = n
		}
	}
	start := (f.Page - 1) * f.PerPage
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + f.PerPage
	if end > len(filtered) {
		end = len(filtered)
	}
	cfg.render(w, r, "home.html", map[string]any{
		"Title":      "Catalog",
		"Filter":     f,
		"Items":      filtered[start:end],
		"Categories": distinctItemValues(items, func(it catalog.Item) string { return it.Category }),
		"Rarities":   catalogRarities(items),
		"Sources":    distinctItemValues(items, func(it catalog.Item) string { return it.Source }),
		"PrevPage":   f.Page - 1,
		"NextPage":   f.Page + 1,
		"HasPrev":    start > 0,
		"HasNext":    end < len(filtered),
	})
}

func (cfg *apiConfig) handleItem(w http.ResponseWriter, r *http.Request) {
	includeHidden := isDM(r)
	it, ok := cfg.Store.Item(chi.URLParam(r, "id"))
	if !ok || (!includeHidden && !it.Visible) {
		http.NotFound(w, r)
		return
	}
	usedBy := filterVisibleItemPtrs(cfg.Store.UsedBy(it.ID), includeHidden)
	var variants []*catalog.Item
	if it.VariantOf != nil {
		all := cfg.Store.Items()
		for i := range all {
			cand := &all[i]
			if cand.ID == it.ID || cand.VariantOf == nil || *cand.VariantOf != *it.VariantOf {
				continue
			}
			if !includeHidden && !cand.Visible {
				continue
			}
			variants = append(variants, cand)
		}
	}
	cfg.render(w, r, "item.html", map[string]any{
		"Title":    it.Name,
		"Item":     it,
		"UsedBy":   usedBy,
		"Variants": variants,
	})
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errBadInt
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 {
		return 0, errBadInt
	}
	return n, nil
}

var errBadInt = &badIntError{}

type badIntError struct{}

func (*badIntError) Error() string { return "not a positive integer" }
