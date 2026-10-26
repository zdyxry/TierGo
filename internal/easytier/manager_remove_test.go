package easytier

import (
	"testing"
	"time"
)

func TestManagerRemoveReleasesOnlyStoppedControllers(t *testing.T) {
	resetSecrets(t)
	manager, _ := testManager(t)
	profile := NewProfile("临时")
	profile.Config = testConfig()
	replaceSecrets(profile.ID, profile.Config.NetworkSecret)
	manager.Start(profile)
	controller := manager.Controller(profile.ID)
	waitForPhase(t, controller, PhaseRunning, 5*time.Second)
	if err := manager.Remove(profile.ID); err == nil {
		t.Fatal("a running controller was removed")
	}
	if err := controller.StopAndWait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(profile.ID); err != nil {
		t.Fatal(err)
	}
	secrets.mu.Lock()
	_, retained := secrets.owners[profile.ID]
	secrets.mu.Unlock()
	if retained {
		t.Fatal("the removed controller retained its secrets")
	}
	if err := manager.Remove("missing"); err != nil {
		t.Fatal(err)
	}
}
