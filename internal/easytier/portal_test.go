package easytier

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPortalRoundTripAndEndpoint(t *testing.T) {
	c := testConfig()
	c.VPNPortal = &VPNPortal{ClientCIDR: "10.14.14.0/24", Listen: "0.0.0.0:51820"}
	text, err := ExportConfigTOML(c)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseConfigTOML(text)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.VPNPortal, back.VPNPortal) {
		t.Fatal("Portal changed")
	}
	updated, err := SetPortalEndpoint("PrivateKey = secret\nEndpoint = 0.0.0.0:51820 # comment\n", "vpn.example.com:51820")
	if err != nil || !strings.Contains(updated, "Endpoint = vpn.example.com:51820\n") || !strings.Contains(updated, "PrivateKey = secret") {
		t.Fatalf("endpoint=%q err=%v", updated, err)
	}
	c.VPNPortal.ClientCIDR = "garbage"
	if c.Validate() == nil {
		t.Fatal("invalid portal accepted")
	}
}
func TestE2EVPNPortalClientConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	ctl, _ := testController(t)
	ctl.engine = nativeEngine{}
	config := testConfig()
	port, err := freePort("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	config.VPNPortal = &VPNPortal{ClientCIDR: "10.14.14.0/24", Listen: fmt.Sprintf("127.0.0.1:%d", port)}
	ctl.Start(config)
	snap := waitForPhase(t, ctl, PhaseRunning, 30*time.Second)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		info, err := ReadVPNPortal(context.Background(), snap.Portal)
		if err == nil && strings.Contains(info.ClientConfig, "[Interface]") && strings.Contains(info.ClientConfig, "PrivateKey") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("Portal did not produce client configuration")
}
