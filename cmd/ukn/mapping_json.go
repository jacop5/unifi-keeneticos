// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// parseMappingJSON decodes a strict JSON object with unique string keys and
// string values. Unlike json.Unmarshal into map[string]string it rejects
// duplicate keys (including escaped equivalents such as "a" and "\u0061"), null
// or non-object input, null or non-string values, malformed JSON and trailing
// data. Error messages never include input keys or values because mapping
// keys may be SSIDs.
func parseMappingJSON(data []byte) (map[string]string, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, errors.New("mapping JSON is malformed")
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("mapping JSON must be an object with string keys and string values")
	}
	out := map[string]string{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, errors.New("mapping JSON is malformed")
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("mapping JSON is malformed")
		}
		valTok, err := dec.Token()
		if err != nil {
			return nil, errors.New("mapping JSON is malformed")
		}
		val, ok := valTok.(string)
		if !ok {
			return nil, errors.New("mapping JSON values must be strings")
		}
		if _, dup := out[key]; dup {
			return nil, errors.New("mapping JSON contains a duplicate key")
		}
		out[key] = val
	}
	if _, err := dec.Token(); err != nil { // consume closing '}'
		return nil, errors.New("mapping JSON is malformed")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("mapping JSON has trailing data")
	}
	return out, nil
}

// loadMappingFile reads path and decodes it with parseMappingJSON. File-read
// errors are returned unchanged; decode errors are wrapped with the flag name
// and never expose input keys or values.
func loadMappingFile(path, flagName string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := parseMappingJSON(b)
	if err != nil {
		return nil, fmt.Errorf("invalid %s mapping file: %w", flagName, err)
	}
	return m, nil
}
