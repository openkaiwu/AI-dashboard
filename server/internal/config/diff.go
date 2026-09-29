package config

import (
	"fmt"
	"sort"
)

// DiffEntry is one semantic difference between two canonical versions.
type DiffEntry struct {
	Op   string `json:"op"` // add | remove | change
	Path string `json:"path"`
	From any    `json:"from,omitempty"`
	To   any    `json:"to,omitempty"`
}

// flatten turns canonical JSON into path → scalar leaf map; objects/arrays are
// walked, leaves compared by value. Secret refs are opaque (already value-free).
func flatten(node any, path string, out map[string]any) {
	switch tv := node.(type) {
	case map[string]any:
		if len(tv) == 0 {
			out[path+"{}"] = nil
			return
		}
		keys := make([]string, 0, len(tv))
		for key := range tv {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			flatten(tv[key], path+"."+key, out)
		}
	case []any:
		if len(tv) == 0 {
			out[path+"[]"] = nil
			return
		}
		for i, v := range tv {
			flatten(v, fmt.Sprintf("%s[%d]", path, i), out)
		}
	default:
		out[path] = tv
	}
}

// Diff compares two canonical contents leaf by leaf; results are path-sorted.
func Diff(from, to map[string]any) []DiffEntry {
	left := map[string]any{}
	right := map[string]any{}
	flatten(from, "", left)
	flatten(to, "", right)
	paths := map[string]bool{}
	for p := range left {
		paths[p] = true
	}
	for p := range right {
		paths[p] = true
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	out := []DiffEntry{}
	for _, p := range sorted {
		l, lok := left[p]
		r, rok := right[p]
		switch {
		case lok && !rok:
			out = append(out, DiffEntry{Op: "remove", Path: p, From: l})
		case !lok && rok:
			out = append(out, DiffEntry{Op: "add", Path: p, To: r})
		case fmt.Sprint(l) != fmt.Sprint(r):
			out = append(out, DiffEntry{Op: "change", Path: p, From: l, To: r})
		}
	}
	return out
}
