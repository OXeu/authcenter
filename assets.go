package configcenter

import "embed"

// Frontend contains the assets produced by Vite Plus before compiling Go.
//
//go:embed all:.tmp/ui
var Frontend embed.FS
