package easytier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewProfileGeneratesUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		profile := NewProfile("办公网")
		if seen[profile.ID] {
			t.Fatalf("duplicated profile id %q", profile.ID)
		}
		seen[profile.ID] = true
		if profile.Name != "办公网" {
			t.Errorf("name = %q", profile.Name)
		}
	}
	// An empty name still gets something to show.
	if got := NewProfile("  ").Name; got != "未命名网络" {
		t.Errorf("empty name became %q", got)
	}
}

func TestSaveAndLoadProfiles(t *testing.T) {
	defer SetConfigFile(filepath.Join(t.TempDir(), "config.json"))()

	one := NewProfile("公司")
	one.Config.NetworkName = "corp"
	one.Config.NetworkSecret = "corp-secret"
	two := NewProfile("家庭")
	two.Config.NetworkName = "home"

	// Saved out of name order on purpose: the listing sorts by name.
	if err := SaveProfile(one); err != nil {
		t.Fatal(err)
	}
	if err := SaveProfile(two); err != nil {
		t.Fatal(err)
	}

	profiles, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("profiles = %v", profiles)
	}
	if profiles[0].Name != "公司" || profiles[1].Name != "家庭" {
		t.Errorf("profiles are not sorted by name: %q, %q", profiles[0].Name, profiles[1].Name)
	}
	if profiles[0].Config.NetworkName != "corp" || profiles[0].Config.NetworkSecret != "corp-secret" {
		t.Errorf("the saved configuration did not come back: %+v", profiles[0].Config)
	}

	// Saving again updates in place instead of adding another network.
	one.Config.NetworkName = "corp2"
	if err := SaveProfile(one); err != nil {
		t.Fatal(err)
	}
	profiles, err = LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("re-saving added a network: %v", profiles)
	}
}

func TestDeleteProfile(t *testing.T) {
	defer SetConfigFile(filepath.Join(t.TempDir(), "config.json"))()

	profile := NewProfile("临时")
	if err := SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err := DeleteProfile(profile.ID); err != nil {
		t.Fatal(err)
	}
	profiles, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Fatalf("the profile survived deletion: %v", profiles)
	}
	// Deleting what is not there is not an error.
	if err := DeleteProfile(profile.ID); err != nil {
		t.Fatal(err)
	}
}

// The single-network release kept its configuration in config.json; the
// first run of this one turns it into a profile instead of losing it.
func TestLoadProfilesMigratesTheLegacyConfig(t *testing.T) {
	root := t.TempDir()
	defer SetConfigFile(filepath.Join(root, "config.json"))()

	legacy := DefaultConfig()
	legacy.NetworkName = "legacy-net"
	legacy.NetworkSecret = "legacy-secret"
	legacy.Peers = []string{"tcp://public.top:11010"}
	if err := legacy.Save(); err != nil {
		t.Fatal(err)
	}

	profiles, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles = %v", profiles)
	}
	if profiles[0].Name != "默认网络" {
		t.Errorf("migrated profile name = %q", profiles[0].Name)
	}
	if profiles[0].Config.NetworkName != "legacy-net" || len(profiles[0].Config.Peers) != 1 {
		t.Errorf("the legacy configuration was lost: %+v", profiles[0].Config)
	}

	// Migration happens once: the profile file drives from here on.
	profiles[0].Config.NetworkName = "renamed"
	if err := SaveProfile(profiles[0]); err != nil {
		t.Fatal(err)
	}
	again, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].Config.NetworkName != "renamed" {
		t.Errorf("the migration ran twice: %+v", again)
	}
	// The legacy file stays, so a downgrade does not lose the network.
	if _, err := os.Stat(filepath.Join(root, "config.json")); err != nil {
		t.Errorf("the legacy file was removed: %v", err)
	}
}

func TestLoadProfilesReportsACorruptFile(t *testing.T) {
	defer SetConfigFile(filepath.Join(t.TempDir(), "config.json"))()
	dir, err := profilesDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath(dir, "broken"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfiles(); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("a corrupt profile was not reported: %v", err)
	}
}

func TestInstanceNameIsUniquePerProfile(t *testing.T) {
	one, two := NewProfile("a"), NewProfile("b")
	if one.InstanceName() == two.InstanceName() {
		t.Fatal("two profiles share an instance name")
	}
	if !strings.HasPrefix(one.InstanceName(), "mygo-") {
		t.Errorf("instance name = %q", one.InstanceName())
	}
}
