//go:build !nogui

package ui

import "embed"

// assetsFS is the page, packed into the binary: one file to install.
//
//go:embed assets
var assetsFS embed.FS
