package ui

import "embed"

// Dist embeds the compiled frontend assets from ui/dist/.
// When the dist directory is empty (development), the server falls back
// to a placeholder page.
//
//go:embed all:dist
var Dist embed.FS
