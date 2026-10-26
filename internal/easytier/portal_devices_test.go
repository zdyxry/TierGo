package easytier

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/makiuchi-d/gozxing"
	qrdecode "github.com/makiuchi-d/gozxing/qrcode"
)

func TestDevicePortalRoundTripAndQR(t *testing.T) {
	c := testConfig()
	c.VPNPortal = &VPNPortal{Listen: "127.0.0.1:51820", DeviceMode: true, Clients: []PortalDevice{{Name: "laptop", VirtualIP: "10.14.14.2/24", Groups: []string{"admin"}}}}
	text, err := ExportConfigTOML(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "client_cidr") || strings.Contains(text, "device_mode") {
		t.Fatal("legacy key leaked into modern schema")
	}
	back, err := ParseConfigTOML(text)
	if err != nil || !back.VPNPortal.DeviceMode || len(back.VPNPortal.Clients) != 1 {
		t.Fatalf("roundtrip: %+v %v", back, err)
	}
	image, err := PortalQRCode("[Interface]\nPrivateKey = test\n")
	if err != nil {
		t.Fatal(err)
	}
	decodedImage, err := png.Decode(bytes.NewReader(image))
	if err != nil {
		t.Fatal(err)
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(decodedImage)
	if err != nil {
		t.Fatal(err)
	}
	result, err := qrdecode.NewQRCodeReader().Decode(bitmap, nil)
	if err != nil || result.GetText() != "[Interface]\nPrivateKey = test\n" {
		t.Fatalf("QR roundtrip failed: %v", err)
	}
	for _, version := range []string{"2.7.0-54f2850d", "easytier-core 2.8.0"} {
		if !DevicePortalSupported(version) {
			t.Fatal(version)
		}
	}
	if DevicePortalSupported("2.6.4-8428a89d") {
		t.Fatal("stable incorrectly supports devices")
	}
}
func TestE2EDevicePortalMain(t *testing.T) {
	if testing.Short() {
		t.Skip("main Core integration")
	}
	core := os.Getenv("EASYTIER_NEXT_CORE")
	cli := os.Getenv("EASYTIER_NEXT_CLI")
	if core == "" || cli == "" {
		t.Skip("set EASYTIER_NEXT_CORE/EASYTIER_NEXT_CLI to v2.7/main binaries")
	}
	t.Setenv(coreBinaryEnv, core)
	t.Setenv(cliBinaryEnv, cli)
	t.Cleanup(SetConfigFile(filepath.Join(t.TempDir(), "config.json")))
	port, err := freePort("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	config := testConfig()
	config.ACL = DefaultACL()
	config.ACL.V1.Group.Declares = []ACLGroupIdentity{{Name: "admin", Secret: "test-group-secret"}}
	config.VPNPortal = &VPNPortal{DeviceMode: true, Listen: fmt.Sprintf("127.0.0.1:%d", port)}
	ctl := NewController("device-portal")
	ctl.Start(config)
	t.Cleanup(func() { ctl.StopAndWait(5 * time.Second) })
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("snapshot: %+v", ctl.Snapshot())
		}
	})
	snap := waitForPhase(t, ctl, PhaseRunning, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		info, err := ReadVPNPortalNative(ctx, snap.Portal)
		if err == nil && info.Listener != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	device := PortalDevice{Name: "test-laptop", VirtualIP: "10.14.14.2/24", Groups: []string{"admin"}}
	if err := PatchPortalDevice(ctx, snap.Portal, device, false); err != nil {
		t.Fatal(err)
	}
	info, err := ReadVPNPortalNative(ctx, snap.Portal)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Clients) != 1 || info.Clients[0].Name != device.Name || len(info.Clients[0].Groups) != 1 || !strings.Contains(info.Clients[0].ClientConfig, "PrivateKey") {
		t.Fatalf("bad client response: %+v", info)
	}
	if _, err := PortalQRCode(info.Clients[0].ClientConfig); err != nil {
		t.Fatal(err)
	}
	if err := PatchPortalDevice(ctx, snap.Portal, device, true); err != nil {
		t.Fatal(err)
	}
	info, err = ReadVPNPortalNative(ctx, snap.Portal)
	if err != nil || len(info.Clients) != 0 {
		t.Fatalf("delete failed: %+v %v", info, err)
	}
}
