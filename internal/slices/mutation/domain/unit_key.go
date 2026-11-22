package domain

import "github.com/donvargax/itos-cc/internal/lang"

// UnitKey is the one key a function's recorded results are paired with it
// by: its namespace#name, its hash, and its ordinal among the functions of
// the file sharing both, in file order (the rule 979960b1 settled). Of
// the functions sharing a name, the key tells reordered ones and edited
// ones apart; it holds no place in a list, so one entry missing from a
// snapshot shifts no other. Only the constructors below make one, from a
// parsed file's units or from a snapshot's entries, so no name string or
// slice index of another list stands in for it.
type UnitKey struct {
	id, hash string
	ordinal  int
}

// ID is the key's namespace#name.
func (k UnitKey) ID() string { return k.id }

// Hash is the key's function hash.
func (k UnitKey) Hash() string { return k.hash }

// keyer hands out the keys of a list of functions, in order, counting the
// ordinal of each among those sharing its name and hash.
type keyer map[[2]string]int

func (k keyer) next(id, hash string) UnitKey {
	at := [2]string{id, hash}
	ordinal := k[at]
	k[at]++
	return UnitKey{id: id, hash: hash, ordinal: ordinal}
}

// KeysOfFile is the key of each function of f, in file order.
func KeysOfFile(f *lang.File) []UnitKey {
	k, out := keyer{}, make([]UnitKey, 0, len(f.Units))
	for _, u := range f.Units {
		out = append(out, k.next(UnitID(u.Namespace, u.Name), UnitHash(f, u)))
	}
	return out
}

// KeysOfEntries is the key of each of entries, a snapshot's, in its order,
// by the hash each carries.
func KeysOfEntries(entries []UnitResult) []UnitKey {
	k, out := keyer{}, make([]UnitKey, 0, len(entries))
	for _, e := range entries {
		out = append(out, k.next(UnitID(e.Namespace, e.Name), e.Hash))
	}
	return out
}

// Results is a file's snapshot entries as its functions find them: the one
// lookup of a function's recorded results.
type Results struct {
	entries []UnitResult
	at      map[UnitKey]int
}

// PairResults pairs each function, by its key in keys, with its entry among
// entries. A function takes the entry that has its key, so reordering
// functions sharing a name changes nothing and those sharing a hash take
// their entries in file order; then each function left takes the first
// entry of its name left, which records it as it was before it changed.
// A function with no entry left has none.
func PairResults(keys []UnitKey, entries []UnitResult) Results {
	r := Results{entries: entries, at: map[UnitKey]int{}}
	recorded := KeysOfEntries(entries)
	byKey := make(map[UnitKey]int, len(recorded))
	for j, k := range recorded {
		byKey[k] = j
	}
	used := make([]bool, len(entries))
	var left []UnitKey
	for _, k := range keys {
		if j, ok := byKey[k]; ok {
			r.at[k], used[j] = j, true
			continue
		}
		left = append(left, k)
	}
	for _, k := range left {
		for j, e := range entries {
			if !used[j] && UnitID(e.Namespace, e.Name) == k.id {
				r.at[k], used[j] = j, true
				break
			}
		}
	}
	return r
}

// For is the entry paired with the function key names, nil for none. It
// is the snapshot's own entry, never to be changed.
func (r Results) For(k UnitKey) *UnitResult {
	if j, ok := r.at[k]; ok {
		return &r.entries[j]
	}
	return nil
}
