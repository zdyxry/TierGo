package owner

import (
	"os"
	"path/filepath"
	"testing"
)

// Keep does nothing without root, which is what these tests run as.
func TestKeepIsQuietWithoutPrivileges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	Keep(dir, path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("permissions changed to %v", info.Mode().Perm())
	}
}
