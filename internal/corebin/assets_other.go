//go:build !(linux && (amd64 || arm64)) && !(darwin && (amd64 || arm64)) && !(windows && (amd64 || arm64))

package corebin

import "embed"

// This platform has no embedded programs; the client uses one installed on
// the machine instead.
var assets embed.FS
