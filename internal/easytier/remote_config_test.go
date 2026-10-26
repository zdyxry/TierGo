package easytier

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestRemoteJSONPreservesUnknownAndPinsID(t *testing.T) {
	id := NewMachineID()
	raw := wireBytes(nil, 1, []byte(id))
	raw = wireInt(raw, 999, 42)
	config := RemoteConfig{Network: RemoteNetwork{ID: id}, Wire: raw, Source: 2}
	edited, err := config.WithJSON(`{"instance_id":"wrong","network_name":"edited","mtu":1400,"acl":{"acl_v1":{"chains":[]}}}`)
	if err != nil {
		t.Fatal(err)
	}
	msg := dynamicpb.NewMessage(networkDescriptor)
	if err := proto.Unmarshal(edited.Wire, msg); err != nil {
		t.Fatal(err)
	}
	if len(msg.GetUnknown()) == 0 || edited.Source != 2 {
		t.Fatal("unknown fields or source lost")
	}
	text, err := edited.JSON()
	if err != nil || !strings.Contains(text, id) || !strings.Contains(text, "1400") {
		t.Fatalf("%s %v", text, err)
	}
	if _, err := config.WithJSON(`{"unknown_field":true}`); err == nil {
		t.Fatal("unknown JSON name accepted")
	}
}
func TestRemoteDisabledBackupSurvivesValidationFailure(t *testing.T) {
	t.Cleanup(SetConfigFile(filepath.Join(t.TempDir(), "config.json")))
	id := NewMachineID()
	config := RemoteConfig{Network: RemoteNetwork{ID: id, Disabled: true}, Wire: wireBytes(nil, 6, []byte("keep"))}
	if err := saveDisabledRemote("127.0.0.1:1", config); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := SaveRemoteConfig(ctx, "127.0.0.1:1", config); err == nil {
		t.Fatal("unreachable remote unexpectedly validated")
	}
	back, err := loadDisabledRemote("127.0.0.1:1", id)
	if err != nil || !bytes.Equal(back.Wire, config.Wire) {
		t.Fatal("backup lost after failed remote operation")
	}
}

func TestE2ERemoteAdvancedEditDisableEnable(t *testing.T) {
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
	edited, err := config.WithJSON(`{"network_name":"advanced-edited","hostname":"edited-host","no_tun":true,"networking_method":"Standalone","mtu":1400,"dev_name":"remote-edit","instance_recv_bps_limit":"65536","disable_p2p":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveRemoteConfig(ctx, snap.Portal, edited); err != nil {
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
	got, err := read.JSON()
	if err != nil || !strings.Contains(got, "1400") || !strings.Contains(got, "remote-edit") {
		t.Fatalf("advanced edit not applied: %s %v", got, err)
	}
	if err := SetRemoteDisabled(ctx, snap.Portal, network, true); err != nil {
		t.Fatal(err)
	}
	networks, err = ListRemoteNetworks(ctx, snap.Portal)
	if err != nil {
		t.Fatal(err)
	}
	var disabled RemoteNetwork
	for _, n := range networks {
		if n.ID == id {
			disabled = n
		}
	}
	if !disabled.Disabled {
		t.Fatal("disabled backup missing")
	}
	path, err := disabledRemotePath(snap.Portal, id)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("backup not owner-only")
	}
	other, _ := listDisabledRemote("127.0.0.1:1")
	if len(other) != 0 {
		t.Fatal("backup crossed endpoint boundary")
	}
	if err := SetRemoteDisabled(ctx, snap.Portal, disabled, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("enabled backup remains")
	}
	if !ctl.Running() {
		t.Fatal("unrelated instance stopped")
	}
	if err := DeleteRemoteNetwork(ctx, snap.Portal, network); err != nil {
		t.Fatal(err)
	}
}
