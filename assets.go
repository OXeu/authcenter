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

// Brand contains the application icon in sizes for browsers and page headers.
//
//go:embed assets/icon-*.webp
var Brand embed.FS

// RenderedDocumentation contains build-time HTML and Shiki token styles.
//
//go:embed .tmp/docs/*.html .tmp/docs/shiki.css
var RenderedDocumentation embed.FS
