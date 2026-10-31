package core

// The interface catalog: the same entries the browser renders, embedded once so Go and the UI read
// the same data. English and pt-BR live side by side; ids are stable (ui.<area>.<slug>.<hash>).

import (
	_ "embed"
	"encoding/json"
)

//go:embed ui.json
var uiCatalogJSON []byte

func init() {
	var catalog map[string]map[string]string
	if err := json.Unmarshal(uiCatalogJSON, &catalog); err != nil {
		panic("core: ui.json: " + err.Error())
	}
	RegisterMessages(catalog)
}
