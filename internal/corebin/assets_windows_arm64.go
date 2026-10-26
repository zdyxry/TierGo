//go:build windows && arm64

package corebin

import "embed"

// Only this platform's programs are carried in this build, so a universal
// macOS app has the right core in each of its halves.
//
//go:embed assets/windows-arm64
var assets embed.FS
