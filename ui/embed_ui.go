//go:build ui

// ui/embed_ui.go — only compiled when building with -tags ui
package ui

import "embed"

//go:embed all:dist
var WebFS embed.FS
