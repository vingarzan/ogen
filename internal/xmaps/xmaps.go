// Package xmaps provides some generic utilities missed from x/exp/maps.
package xmaps

import (
	"slices"

	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"
)

// SortedKeys returns a sorted slice of keys in the map.
func SortedKeys[M ~map[K]V, K constraints.Ordered, V any](m M) []K {
	r := maps.Keys(m)
	slices.Sort(r)
	return r
}

func SortedKeysFunc[M ~map[K]V, K comparable, V any](m M, cmp func(a K, b K) int) []K {
	keys := make([]K, 0, len(m))
	for ref := range m {
		keys = append(keys, ref)
	}
	slices.SortStableFunc(keys, cmp)
	return keys
}
