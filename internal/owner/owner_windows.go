//go:build windows

package owner

// Windows has no such split: an elevated process writes as the
// administrator, and the same account reads it back.
func keep(string, string) {}
