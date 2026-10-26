//go:build linux && amd64

package corebin

import "embed"

// Only this platform's programs are carried in this build, so a universal
// macOS app has the right core in each of its halves.
//
//go:embed assets/linux-amd64
var assets embed.FS
