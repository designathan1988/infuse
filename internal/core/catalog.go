package core

// The message catalog: protocol, gates, tools, guard, shell trailer and every interface string, with
// English and pt-BR side by side. It is data, not code; stable ui.* ids are shared with the browser and
// served by /api/i18n, so Go and the UI can never drift.

import (
	_ "embed"
	"encoding/json"
)

//go:embed catalog.json
var catalogJSON []byte

func init() {
	var messages map[string]map[string]string
	if err := json.Unmarshal(catalogJSON, &messages); err != nil {
		panic("core: catalog.json: " + err.Error())
	}
	RegisterMessages(messages)
}
