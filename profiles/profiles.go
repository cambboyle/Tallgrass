// Package profiles bundles the game profile JSON files into the binary.
package profiles

import "embed"

//go:embed *.json
var FS embed.FS
