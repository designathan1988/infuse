package agent

// registerBuiltins wires the built-in tools into the registry. Registration is explicit so the
// composition root can add connector and browser tools on top.
func registerBuiltins() {
	for _, t := range []Tool{bashTool{}, readTool{}, writeTool{}, editTool{}, multiEditTool{}, globTool{}, grepTool{}, lsTool{}, webFetchTool{}, todoTool{}, askTool{}, memoryTool{}, skillTool{}, webSearchTool{}, diagnosisTool{}, reportTool{}, contractTool{}, impasseTool{}, agentTool{}, browserToolAdapter{}, bashOutputTool{}, killShellTool{}} {
		registerTool(t)
	}
}
