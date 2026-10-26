package main

import (
	"encoding/json"
	"image/png"
	"os"
	"testing"
)

func TestThirdPartyLicensesAreIncludedInPackages(t *testing.T) {
	required := []string{
		"LICENSE",
		"SECURITY.md",
		"THIRD_PARTY_NOTICES.md",
		"third_party/easytier/LICENSE",
		"third_party/lucide/LICENSE",
		"third_party/wintun/prebuilt-binaries-license.txt",
		"third_party/windivert/LICENSE",
		"third_party/npcap/LICENSE",
		"third_party/go/mygo/LICENSE",
		"third_party/go/gozxing/LICENSE",
		"third_party/go/go-toml/LICENSE",
		"third_party/go/go-qrcode/LICENSE",
		"third_party/go/purego/LICENSE",
		"third_party/go/typesetting/LICENSE",
		"third_party/go/typesetting/harfbuzz-LICENSE",
		"third_party/go/x-image/LICENSE",
		"third_party/go/x-sys/LICENSE",
		"third_party/go/x-text/LICENSE",
		"third_party/go/xerrors/LICENSE",
		"third_party/go/protobuf/LICENSE",
	}
	for _, name := range required {
		info, err := os.Stat(name)
		if err != nil {
			t.Errorf("missing %s: %v", name, err)
		} else if info.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
	}
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Resources []string `json:"resources"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	resources := map[string]bool{}
	for _, resource := range config.Resources {
		resources[resource] = true
	}
	for _, resource := range []string{"LICENSE", "SECURITY.md", "THIRD_PARTY_NOTICES.md", "third_party"} {
		if !resources[resource] {
			t.Errorf("mygo.json does not package %s", resource)
		}
	}
}

func TestApplicationIconIsReadyForPackaging(t *testing.T) {
	file, err := os.Open("resources/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 1024 || image.Bounds().Dy() != 1024 {
		t.Fatalf("icon dimensions = %v, want 1024x1024", image.Bounds())
	}
}
