// Package ui embeds the window's assets so the binary stays self-contained.
package ui

import "embed"

//go:embed index.html js i18n.js vendor styles
var FS embed.FS
