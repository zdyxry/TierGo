package easytier

import (
	"path/filepath"
	"testing"
)

func TestAppSettingsRoundTrip(t *testing.T) {
	defer SetConfigFile(filepath.Join(t.TempDir(), "config.json"))()

	// Nothing saved yet is the zero value, not an error.
	settings, err := LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.OpenAtLogin {
		t.Fatal("open at login defaults to off")
	}

	settings.OpenAtLogin = true
	if err := settings.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.OpenAtLogin {
		t.Fatal("the saved setting did not come back")
	}
}

// The session — the network on screen and the ones to reconnect on launch —
// survives a restart.
func TestAppSettingsSessionRoundTrip(t *testing.T) {
	defer SetConfigFile(filepath.Join(t.TempDir(), "config.json"))()

	settings := AppSettings{
		SelectedProfile: "profile-b",
		EnabledNetworks: []string{"profile-a", "profile-b"},
	}
	if err := settings.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SelectedProfile != "profile-b" {
		t.Fatalf("selected profile = %q", loaded.SelectedProfile)
	}
	if len(loaded.EnabledNetworks) != 2 || loaded.EnabledNetworks[0] != "profile-a" {
		t.Fatalf("enabled networks = %v", loaded.EnabledNetworks)
	}
}
