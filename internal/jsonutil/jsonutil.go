// Package jsonutil marshals JSON the way JS's JSON.stringify(v, null, 2)
// does, for output files meant to match what this project's previous
// Node-based generators produced byte-for-byte (question text and
// references contain plain "&"/"<"/">", e.g. category names like "Arts,
// Culture, Sport & Science").
//
// encoding/json's Marshal and MarshalIndent both HTML-escape those three
// characters to &/</> by default (a safety default for JSON
// embedded in HTML <script> tags, irrelevant here); disabling that requires
// going through an Encoder.
package jsonutil

import (
	"bytes"
	"encoding/json"
)

// Marshal renders v as compact JSON with HTML escaping disabled, matching
// JSON.stringify(v). Types with a custom MarshalJSON (such as ordered.Map)
// must build their output through this function rather than the top-level
// encoding/json.Marshal, whose default HTML escaping a surrounding
// Encoder's SetEscapeHTML(false) cannot override once nested bytes come
// back already escaped.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// MarshalIndent renders v as 2-space-indented JSON with HTML escaping
// disabled and no trailing newline, matching JSON.stringify(v, null, 2).
func MarshalIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encoder.Encode always appends a trailing newline; JSON.stringify
	// (and thus the files this replaces) never had one.
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
