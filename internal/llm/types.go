package llm

import "infuse/internal/core"

// Wire types shared with the rest of the application; aliases keep this package's code readable.
type (
	Block    = core.Block
	Message  = core.Message
	Usage    = core.Usage
	ToolSpec = core.ToolSpec
)
