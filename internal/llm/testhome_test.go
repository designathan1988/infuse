package llm

import (
	"encoding/json"
	"testing"
)

const fallbackContext = FallbackContext

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
}

func contextWindow(model string) int { return ContextWindow(model) }

func newProvider(name string, cfg ProviderConfig) (Provider, error) { return NewProvider(name, cfg) }

func toolSpecs() []ToolSpec {
	return []ToolSpec{{Name: "Read", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}}
}
