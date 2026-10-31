package agent

import (
	"encoding/json"
	"testing"
)

// Every tool's schema is valid JSON: one broken schema breaks every request that lists the tools.
func TestEveryToolSchemaIsValidJSON(t *testing.T) {
	for _, sp := range toolSpecs() {
		var v map[string]any
		if err := json.Unmarshal(sp.InputSchema, &v); err != nil {
			t.Fatalf("%s: invalid schema: %v", sp.Name, err)
		}
		if v["type"] != "object" {
			t.Fatalf("%s: schema is not an object", sp.Name)
		}
	}
}
