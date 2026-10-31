// Package store owns the schema version of every document infuse persists and the migration chain
// between versions. Documents written before versioning have no schemaVersion and are version 0.
package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"os"
)

// Version is the schema version this build writes. A migration map keyed by the version it upgrades
// (0 upgrades a pre-versioning document to 1).
const Version = 1

// Versioned is embedded by the persisted document structs.
type Versioned struct {
	SchemaVersion int `json:"schemaVersion,omitempty"`
}

// Upgrade applies the migrations from the document's version up to Version and reports whether the
// document changed (and should be written back). A document from a newer build is read as is and left
// untouched, so a downgrade does not destroy it.
func (v *Versioned) Upgrade(migrations map[int]func() error) (bool, error) {
	if v.SchemaVersion > Version {
		return false, nil
	}
	changed := false
	for from := v.SchemaVersion; from < Version; from++ {
		if migrate := migrations[from]; migrate != nil {
			if err := migrate(); err != nil {
				return changed, fmt.Errorf("schema version %d: %w", from, err)
			}
		}
		v.SchemaVersion = from + 1
		changed = true
	}
	return changed, nil
}

// List is the envelope of a document that used to be a bare JSON array: {"schemaVersion":1,"items":[...]}.
type List[T any] struct {
	SchemaVersion int `json:"schemaVersion,omitempty"`
	Items         []T `json:"items"`
}

// LoadList reads an envelope or a legacy bare array (version 0).
func LoadList[T any](file string) (List[T], error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return List[T]{}, err
	}
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []T
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return List[T]{}, err
		}
		return List[T]{Items: items}, nil
	}
	var list List[T]
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return List[T]{}, err
	}
	if list.Items == nil {
		list.Items = []T{}
	}
	return list, nil
}

// SaveList writes the envelope at Version (or keeps a newer version read from disk).
func SaveList[T any](file string, list List[T]) error {
	if list.SchemaVersion < Version {
		list.SchemaVersion = Version
	}
	return core.WriteJSON(file, list)
}
