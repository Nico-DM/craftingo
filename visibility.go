package main

import (
	"net/http"
	"sort"

	"github.com/Nico-DM/craftingo/internal/catalog"
)

// isDM reports whether the current request belongs to a dungeon master, who
// can see hidden catalog items and edit their visibility.
func isDM(r *http.Request) bool {
	u, ok := currentUser(r)
	return ok && u.Role == "dm"
}

// filterVisibleItems removes items the DM has hidden. DMs get the full list.
func filterVisibleItems(items []catalog.Item, includeHidden bool) []catalog.Item {
	if includeHidden {
		return items
	}
	out := make([]catalog.Item, 0, len(items))
	for _, it := range items {
		if it.Visible {
			out = append(out, it)
		}
	}
	return out
}

// filterVisibleItemPtrs is the pointer-slice form of filterVisibleItems.
func filterVisibleItemPtrs(items []*catalog.Item, includeHidden bool) []*catalog.Item {
	if includeHidden {
		return items
	}
	out := make([]*catalog.Item, 0, len(items))
	for _, it := range items {
		if it.Visible {
			out = append(out, it)
		}
	}
	return out
}

func distinctItemValues(items []catalog.Item, value func(catalog.Item) string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		v := value(it)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func catalogRarities(items []catalog.Item) []string {
	out := distinctItemValues(items, func(it catalog.Item) string { return it.Rarity })
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := catalog.RarityRank(out[i]), catalog.RarityRank(out[j])
		if ri == 0 {
			ri = 6
		}
		if rj == 0 {
			rj = 6
		}
		return ri < rj
	})
	return out
}
