package buildinfo

import (
	"encoding/json"
	"os"
	"testing"
)

func TestVersionMatchesPackageManifest(t *testing.T) {
	data, err := os.ReadFile("../../mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != Version {
		t.Fatalf("mygo.json version %q does not match application version %q", manifest.Version, Version)
	}
	if Commit() == "" {
		t.Fatal("empty commit")
	}
}
