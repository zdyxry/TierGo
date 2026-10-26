package easytier

import (
	"context"
	"testing"
	"time"
)

func TestNormalizeRPCAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:15888", "tcp://[::1]:15888"} {
		if _, err := NormalizeRPCAddress(address); err != nil {
			t.Fatal(err)
		}
	}
	for _, address := range []string{"http://example.com:80", "tcp://user:pass@example.com:80", "tcp://example.com:80/path", "example.com"} {
		if _, err := NormalizeRPCAddress(address); err == nil {
			t.Fatalf("accepted %q", address)
		}
	}
}
func TestE2ERemoteInspectionDoesNotOwnCore(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	ctl, _ := testController(t)
	ctl.engine = nativeEngine{}
	ctl.Start(testConfig())
	snap := waitForPhase(t, ctl, PhaseRunning, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	networks, err := ListRemoteNetworks(ctx, snap.Portal)
	if err != nil {
		t.Fatal(err)
	}
	if len(networks) != 1 || networks[0].Name != "test-net" {
		t.Fatalf("networks=%+v", networks)
	}
	remote, err := ReadRemoteStatus(ctx, snap.Portal, networks[0])
	if err != nil {
		t.Fatal(err)
	}
	if remote.Node.Hostname != "test-host" || remote.Node.InstID != networks[0].ID {
		t.Fatalf("wrong instance: %+v", remote.Node)
	}
	if remote.Counters == nil {
		t.Fatal("remote status carried no traffic counters")
	}
	// The instance-scoped Portal query must reach the same instance; what it
	// returns is the Core's own business (v2.6.4 answers a template even
	// without a configured Portal).
	if _, err := ReadRemoteVPNPortal(ctx, snap.Portal, networks[0]); err != nil {
		t.Fatal(err)
	}
	if !ctl.Running() {
		t.Fatal("inspection stopped Core")
	}
}

func TestE2ERemoteACLSaveAndReadBack(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	ctl, _ := testController(t)
	ctl.engine = nativeEngine{}
	ctl.Start(testConfig())
	snap := waitForPhase(t, ctl, PhaseRunning, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, err := CreateRemoteNetwork(ctx, snap.Portal, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	networks, err := ListRemoteNetworks(ctx, snap.Portal)
	if err != nil {
		t.Fatal(err)
	}
	var network RemoteNetwork
	for _, n := range networks {
		if n.ID == id {
			network = n
		}
	}
	if network.ID == "" {
		t.Fatal("created network missing")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = DeleteRemoteNetwork(ctx, snap.Portal, network)
	}()
	var config RemoteConfig
	ready := time.Now().Add(8 * time.Second)
	for time.Now().Before(ready) {
		config, err = GetRemoteConfig(ctx, snap.Portal, network)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	acl := DefaultACL()
	acl.V1.Chains[0].Rules = []ACLRule{{
		Name: "ssh", Priority: 100, Enabled: true, Protocol: 1,
		Ports: []string{"22"}, SourceIPs: []string{"10.0.0.0/24"}, Action: 1,
	}}
	acl.V1.Group.Declares = []ACLGroupIdentity{{Name: "ops", Secret: "s3cr3t"}}
	acl.V1.Group.Members = []string{"ops"}
	updated, err := config.WithACL(acl)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveRemoteConfig(ctx, snap.Portal, updated); err != nil {
		t.Fatal(err)
	}
	var read RemoteConfig
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		read, err = GetRemoteConfig(ctx, snap.Portal, network)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	got, err := read.ACL()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.V1.Chains) != 3 {
		t.Fatalf("acl = %+v", got)
	}
	rule := got.V1.Chains[0].Rules[0]
	if rule.Name != "ssh" || rule.Ports[0] != "22" || rule.SourceIPs[0] != "10.0.0.0/24" {
		t.Fatalf("rule = %+v", rule)
	}
	if got.V1.Group.Declares[0].Name != "ops" || got.V1.Group.Members[0] != "ops" {
		t.Fatalf("group = %+v", got.V1.Group)
	}
	if !ctl.Running() {
		t.Fatal("remote ACL save stopped the unrelated instance")
	}
}
