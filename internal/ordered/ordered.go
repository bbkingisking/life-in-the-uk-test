// Package ordered provides a JSON object that remembers key insertion
// order.
//
// A plain Go map[string]V always marshals with its keys sorted
// lexicographically ("1", "10", "2", ...), unlike a JS object, which
// preserves insertion order under JSON.stringify. exams.json's "exams" and
// "categories" objects are iterated by key order in the front end
// (Object.keys(examsData).forEach(...) builds the exam buttons, in that
// order), so the generated JSON must keep exam numbers in numeric order
// (1, 2, ..., 10, 11, ...), not lexicographic. Map exists to make that
// order explicit and enforced, rather than relying on incidental map
// iteration order.
package ordered

import (
	"bytes"
	"sort"
	"strconv"

	"github.com/DHKLeung/life-in-the-uk-test/internal/jsonutil"
)

// Map is an ordered string-keyed JSON object.
type Map[V any] struct {
	keys   []string
	values map[string]V
}

// NewMap returns an empty Map.
func NewMap[V any]() *Map[V] {
	return &Map[V]{values: make(map[string]V)}
}

// Set inserts or updates key. New keys are appended to the iteration order;
// updating an existing key leaves its position unchanged.
func (m *Map[V]) Set(key string, value V) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

// Keys returns the keys in insertion order.
func (m *Map[V]) Keys() []string {
	return m.keys
}

// MarshalJSON writes the object with its keys in insertion order.
// json.MarshalIndent re-indents this compact output afterwards without
// disturbing key order, so callers get "pretty" output for free.
//
// Keys and values are marshaled through jsonutil.Marshal, not the
// top-level encoding/json.Marshal: a value's HTML-escaping has to be
// decided here, since a surrounding Encoder's SetEscapeHTML(false) cannot
// retroactively unescape bytes a nested Marshaler already returned escaped.
func (m Map[V]) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := jsonutil.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := jsonutil.Marshal(m.values[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// FromMapNumeric builds a Map from m with keys ordered numerically (as in,
// parsed as base-10 integers) rather than lexicographically, matching how
// exam numbers ("1", "2", ..., "10") need to sort. A key that isn't a
// plain integer sorts as though it were 0, before any numeric key.
func FromMapNumeric[V any](m map[string]V) *Map[V] {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, _ := strconv.Atoi(keys[i])
		nj, _ := strconv.Atoi(keys[j])
		return ni < nj
	})
	om := NewMap[V]()
	for _, k := range keys {
		om.Set(k, m[k])
	}
	return om
}
