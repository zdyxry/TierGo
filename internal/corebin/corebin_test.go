package corebin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A build without the assets cannot be made by a test, so the checks that
// need them are skipped instead of failing.
func TestEnsureExtractsTheEmbeddedCore(t *testing.T) {
	if !Available() {
		t.Skip("这个构建没有内嵌内核（先运行 go run ./cmd/fetch-core）")
	}
	dir := t.TempDir()
	binary, err := Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if binary.Version == "" {
		t.Error("the version was not recorded")
	}
	for name, path := range map[string]string{"easytier-core": binary.Core, "easytier-cli": binary.CLI} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s was not written: %v", name, err)
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s is not executable: %v", name, info.Mode())
		}
	}
	// The Windows driver files must not be executable.
	for _, entry := range []string{binary.Dir} {
		files, err := os.ReadDir(entry)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if filepath.Ext(file.Name()) == ".dll" || filepath.Ext(file.Name()) == ".sys" {
				info, err := file.Info()
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm()&0o100 != 0 {
					t.Errorf("%s should not be executable", file.Name())
				}
			}
		}
	}
}

// The extracted copy is reused while it matches what the build carries, so
// starting the client does not rewrite ten megabytes every time.
func TestEnsureIsIdempotent(t *testing.T) {
	if !Available() {
		t.Skip("这个构建没有内嵌内核")
	}
	dir := t.TempDir()
	first, err := Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(first.Dir, provenanceName)
	before, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}

	future := before.ModTime().Add(2 * 60 * 60 * 1e9)
	if err := os.Chtimes(first.Core, future, future); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(first.Core)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(future) {
		t.Error("the program was written again although nothing changed")
	}
}

// A marker from another version means the core must be replaced.
func TestEnsureReplacesAMismatchedCore(t *testing.T) {
	if !Available() {
		t.Skip("这个构建没有内嵌内核")
	}
	dir := t.TempDir()
	first, err := Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first.Dir, provenanceName), []byte(`{"version":"old","sha256":"nope"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.Core, []byte("stale"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(first.Core)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 1024 {
		t.Errorf("the stale program was kept: %d bytes", info.Size())
	}
}

func TestEmbeddedProvenance(t *testing.T) {
	if !Available() {
		t.Skip("这个构建没有内嵌内核")
	}
	provenance, ok := EmbeddedProvenance()
	if !ok {
		t.Fatal("no provenance")
	}
	if provenance.Version == "" || provenance.SHA256 == "" || provenance.Asset == "" {
		t.Errorf("incomplete provenance: %+v", provenance)
	}
}
