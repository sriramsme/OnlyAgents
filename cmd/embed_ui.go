// embed_ui.go — only compiled when building with -tags ui
//go:build ui

package main

import "embed"

//go:embed all:web/dist
var webFS embed.FS
