package main

import "encoding/json"

// catalogEntry is one item's display metadata, looked up by its full
// gvas object-path string (the same string stored in a BaseData
// ObjectProperty, and used as static/items.json's third field).
type catalogEntry struct {
	Name     string
	IconFile string // "" if no icon has been extracted for this item
}

// loadItemCatalog parses static/items.json's format: a JSON array of
// [name, category, objectPath] triples (the format used by the existing
// Tree-tab item picker), or [name, category, objectPath, iconFile] once
// tools/extracticons (Task 5) has run. The fourth field is read
// defensively -- entries may still have only 3 fields in a checkout
// where the icon pipeline hasn't been run yet -- so this never panics on
// the older format. Entries with fewer than 3 fields are skipped.
func loadItemCatalog(data []byte) (map[string]catalogEntry, error) {
	var raw [][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	catalog := make(map[string]catalogEntry, len(raw))
	for _, entry := range raw {
		if len(entry) < 3 {
			continue
		}
		name, objectPath := entry[0], entry[2]
		iconFile := ""
		if len(entry) > 3 {
			iconFile = entry[3]
		}
		catalog[objectPath] = catalogEntry{Name: name, IconFile: iconFile}
	}
	return catalog, nil
}
