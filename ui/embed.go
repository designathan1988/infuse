// Package ui embeds the window's assets so the binary stays self-contained.
package ui

import "embed"

//go:embed index.html app.js panes.js pages.js i18n.js vendor
var FS embed.FS
