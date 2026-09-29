package web

import "embed"

//go:embed docs.html
var DocsHTML []byte

//go:embed static
var Static embed.FS
