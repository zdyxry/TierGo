package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

// The window comes back to the network it was last showing.
func TestNewRestoresTheSelectedNetwork(t *testing.T) {
	t.Cleanup(easytier.SetConfigFile(filepath.Join(t.TempDir(), "config.json")))
	one := easytier.NewProfile("一")
	two := easytier.NewProfile("二")

	settings := easytier.AppSettings{SelectedProfile: two.ID}
	a := New(easytier.NewManager(), []easytier.Profile{one, two}, settings, "")
	if a.selected != two.ID {
		t.Fatalf("selected = %q, want %q", a.selected, two.ID)
	}

	// A stale id falls back to the first network.
	settings.SelectedProfile = "gone"
	a = New(easytier.NewManager(), []easytier.Profile{one, two}, settings, "")
	if a.selected != one.ID {
		t.Fatalf("selected = %q, want %q", a.selected, one.ID)
	}
}

// Clicking a network in the sidebar is remembered for the next launch.
func TestSelectingANetworkIsRemembered(t *testing.T) {
	a := newTestApp(t)
	other := easytier.NewProfile("家庭")
	a.profiles = append(a.profiles, other)

	tt := ui.NewTester(a.view, 1040, 700)
	if err := tt.Click("家庭"); err != nil {
		t.Fatal(err)
	}
	settings, err := easytier.LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.SelectedProfile != other.ID {
		t.Fatalf("selected profile = %q, want %q", settings.SelectedProfile, other.ID)
	}
}

// Connecting and disconnecting maintain the set the next launch reconnects.
func TestEnabledNetworksAreRemembered(t *testing.T) {
	a := newTestApp(t)
	id := a.profiles[0].ID

	a.setNetworkEnabled(id, true)
	settings, err := easytier.LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.EnabledNetworks) != 1 || settings.EnabledNetworks[0] != id {
		t.Fatalf("enabled networks = %v", settings.EnabledNetworks)
	}

	a.setNetworkEnabled(id, false)
	settings, err = easytier.LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.EnabledNetworks) != 0 {
		t.Fatalf("enabled networks = %v", settings.EnabledNetworks)
	}
}

// A deleted network must not come back on the next launch.
func TestDeletingANetworkForgetsIt(t *testing.T) {
	a := newTestApp(t)
	first := a.profiles[0]
	if err := easytier.SaveProfile(first); err != nil {
		t.Fatal(err)
	}
	other := easytier.NewProfile("临时")
	if err := easytier.SaveProfile(other); err != nil {
		t.Fatal(err)
	}
	a.profiles = append(a.profiles, other)
	a.setNetworkEnabled(other.ID, true)

	a.deleteProfile(other.ID)

	settings, err := easytier.LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range settings.EnabledNetworks {
		if id == other.ID {
			t.Fatalf("the deleted network is still enabled: %v", settings.EnabledNetworks)
		}
	}
	if settings.SelectedProfile != first.ID {
		t.Fatalf("selected profile = %q, want %q", settings.SelectedProfile, first.ID)
	}
}

// Launch reconnects the enabled networks, unless the system service owns
// them.
func TestRestoreSessionReconnectsEnabledNetworks(t *testing.T) {
	a := newTestApp(t)
	id := a.profiles[0].ID
	a.settings.EnabledNetworks = []string{id}
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceNotInstalled, nil
	}
	var restored []string
	a.restore = func(profiles []easytier.Profile, ids []string) error {
		restored = ids
		if len(profiles) != 1 || profiles[0].ID != id {
			t.Errorf("profiles = %v", profiles)
		}
		return nil
	}
	a.RestoreSession()
	flushAsync(t, a)
	if len(restored) != 1 || restored[0] != id {
		t.Fatalf("restored = %v", restored)
	}

	// A running service owns the networks; restoring would fight it.
	called := false
	a.restore = func([]easytier.Profile, []string) error { called = true; return nil }
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceRunning, nil
	}
	a.RestoreSession()
	flushAsync(t, a)
	if called {
		t.Fatal("networks were restored although the service runs")
	}
}

// Starting the service hands the networks over: the next launch must not
// reconnect them locally.
func TestStartingTheServiceForgetsEnabledNetworks(t *testing.T) {
	a := newTestApp(t)
	a.settings.EnabledNetworks = []string{a.profiles[0].ID}
	a.clearEnabledNetworks()

	settings, err := easytier.LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.EnabledNetworks) != 0 {
		t.Fatalf("enabled networks = %v", settings.EnabledNetworks)
	}
}
