package easytier

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRemoteProjectionRejectsUnsupportedFields(t *testing.T) {
	c := testConfig()
	c.MagicDNS = true
	if _, err := basicRemoteConfig(c, newProfileID()); err == nil {
		t.Fatal("advanced field silently dropped")
	}
	c = testConfig()
	c.ACL = DefaultACL()
	if _, err := basicRemoteConfig(c, newProfileID()); err == nil {
		t.Fatal("ACL silently dropped")
	}
}
func TestE2ERemoteCreateAndDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	ctl, _ := testController(t)
	ctl.engine = nativeEngine{}
	ctl.Start(testConfig())
	snap := waitForPhase(t, ctl, PhaseRunning, 30*time.Second)
	c := testConfig()
	c.NetworkName = "remote-created"
	// Force fragmented request with many valid peers. These do not expose
	// sockets or alter host routing and are removed on test cleanup.
	for i := 0; i < 40; i++ {
		c.Peers = append(c.Peers, "tcp://127.0.0.1:1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	id, err := CreateRemoteNetwork(ctx, snap.Portal, c)
	if err != nil {
		t.Fatal(err)
	}
	var created RemoteNetwork
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		networks, err := ListRemoteNetworks(ctx, snap.Portal)
		if err != nil {
			t.Fatal(err)
		}
		for _, network := range networks {
			if network.ID == id {
				created = network
			}
		}
		if created.ID != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if created.Name != "remote-created" {
		t.Fatalf("not created: %+v", created)
	}
	if err := DeleteRemoteNetwork(ctx, snap.Portal, created); err != nil {
		t.Fatal(err)
	}
	networks, err := ListRemoteNetworks(ctx, snap.Portal)
	if err != nil {
		t.Fatal(err)
	}
	for _, network := range networks {
		if network.ID == id {
			t.Fatal("remote deletion failed")
		}
	}
	if !ctl.Running() {
		t.Fatal("remote deletion stopped unrelated process")
	}
	_, err = basicRemoteConfig(c, newProfileID())
	if err != nil && !strings.Contains(err.Error(), "基础") {
		t.Fatal(err)
	}
}
