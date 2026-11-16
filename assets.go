package configcenter

import "embed"

// Frontend contains the assets produced by Vite Plus before compiling Go.
//
//go:embed all:.tmp/ui
var Frontend embed.FS

// Documentation contains the public integration guides.
//
//go:embed docs/index.md docs/api.md docs/sdk.md
var Documentation embed.FS
