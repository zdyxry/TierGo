package easytier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildInstanceTOML(t *testing.T) {
	config := DefaultConfig()
	config.NetworkName = "office"
	config.NetworkSecret = "s3cret"
	config.Hostname = "laptop"
	config.EnableTUN = false

	document, err := buildInstanceTOML(config, 11010, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`instance_name = "test"`,
		`hostname = "laptop"`,
		`listeners = ["tcp://0.0.0.0:11010", "udp://0.0.0.0:11010"]`,
		`network_name = "office"`,
		`network_secret = "s3cret"`,
		`no_tun = true`,
		`enable_encryption = true`,
	} {
		if !strings.Contains(document, want) {
			t.Errorf("configuration does not contain %q:\n%s", want, document)
		}
	}
	if strings.Contains(document, "ipv4 =") {
		t.Errorf("no address was asked for but one was written:\n%s", document)
	}
}

func TestBuildInstanceTOMLFixedAddress(t *testing.T) {
	config := DefaultConfig()
	config.NetworkName = "office"
	config.IPv4 = "10.144.0.10/24"
	config.Peers = []string{"tcp://public.easytier.top:11010"}
	config.ProxyNetworks = []string{"192.168.1.0/24"}
	config.SecureMode = true
	config.NetworkSecret = "test-secret"

	document, err := buildInstanceTOML(config, 11101, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`ipv4 = "10.144.0.10/24"`,
		`no_tun = false`,
		"[secure_mode]\nenabled = true",

		"[[peer]]\nuri = \"tcp://public.easytier.top:11010\"",
		"[[proxy_network]]\ncidr = \"192.168.1.0/24\"",
	} {
		if !strings.Contains(document, want) {
			t.Errorf("configuration does not contain %q:\n%s", want, document)
		}
	}
	if strings.Contains(document, "dhcp = true") {
		t.Error("a fixed address was given but DHCP was asked for")
	}
}

// A TUN needs an address, so an empty one asks the network for it.
func TestBuildInstanceTOMLAsksForDHCP(t *testing.T) {
	config := DefaultConfig()
	config.NetworkName = "office"
	config.EnableTUN = true

	document, err := buildInstanceTOML(config, 11101, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document, "dhcp = true") {
		t.Errorf("no address and a TUN did not ask for DHCP:\n%s", document)
	}
	if !strings.Contains(document, "no_tun = false") {
		t.Errorf("the TUN was disabled:\n%s", document)
	}
}

func TestBuildInstanceTOMLNeedsNetworkName(t *testing.T) {
	config := DefaultConfig()
	config.NetworkName = ""
	if _, err := buildInstanceTOML(config, 11101, "test"); err == nil {
		t.Fatal("an empty network name was accepted")
	}
}

func TestPortOf(t *testing.T) {
	for _, testCase := range []struct {
		endpoint string
		want     int
	}{
		{"tcp://0.0.0.0:11010", 11010},
		{"udp://[::]:11010", 11010},
		{"11010", 11010},
	} {
		got, err := portOf(testCase.endpoint)
		if err != nil || got != testCase.want {
			t.Errorf("portOf(%q) = %d, %v; want %d", testCase.endpoint, got, err, testCase.want)
		}
	}
}

func TestPickPortsAvoidsOtherEasyTier(t *testing.T) {
	listen, portal, err := pickPorts(nil)
	if err != nil {
		t.Fatal(err)
	}
	if listen == 0 || portal == 0 || listen == portal {
		t.Fatalf("listen %d, portal %d", listen, portal)
	}
	// The default ports of another EasyTier installation must not be taken.
	if listen == 11010 || portal == 15888 {
		t.Errorf("picked the default ports of another installation: %d, %d", listen, portal)
	}
}

// TestParseCLIOutput reads output recorded from a real easytier-cli, so the
// parsers follow the tool rather than a guess about it.
func TestParseCLIOutput(t *testing.T) {
	var node cliNode
	readFixture(t, "cli-node.json", &node)
	if node.Hostname != "mygo-b" {
		t.Errorf("hostname = %q", node.Hostname)
	}
	if node.IPv4Addr != "10.244.55.2/24" {
		t.Errorf("ipv4 = %q", node.IPv4Addr)
	}
	if node.StunInfo == nil || len(node.StunInfo.PublicIP) == 0 {
		t.Errorf("stun info = %+v", node.StunInfo)
	}

	var peers []cliPeer
	readFixture(t, "cli-peer.json", &peers)
	if len(peers) != 2 {
		t.Fatalf("peers = %d", len(peers))
	}
	views := peerViews(peers)
	if views[0].Local == views[1].Local {
		t.Errorf("the local node is not told apart: %+v", views)
	}
	remote := views[0]
	if remote.Local {
		remote = views[1]
	}
	if remote.Name != "mygo-a" {
		t.Errorf("remote name = %q", remote.Name)
	}
	if remote.LatencyMS <= 0 || !remote.Direct || remote.Tunnel != "tcp" {
		t.Errorf("remote = %+v", remote)
	}
	if remote.RxBytes != "2.69 kB" || remote.TxBytes != "3.01 kB" {
		t.Errorf("traffic = %q / %q", remote.RxBytes, remote.TxBytes)
	}

	var routes []cliRoute
	readFixture(t, "cli-route.json", &routes)
	if len(routes) == 0 {
		t.Fatal("no routes")
	}
	routeViews := routeViews(routes)
	if routeViews[0].Dest != "10.244.55.1/24" {
		t.Errorf("routes are not sorted by destination: %+v", routeViews)
	}
}

func readFixture(t *testing.T, name string, out any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}

func TestParseLogLine(t *testing.T) {
	for _, testCase := range []struct {
		line, level, message string
	}{
		{
			line:    "2026-10-22T17:05:17.822437+08:00 WARN easytier::tunnel::wireguard: wg connect: 169.254.83.66:11011",
			level:   "warn",
			message: "wireguard: wg connect: 169.254.83.66:11011",
		},
		{
			// The core pads its level with an extra space on some lines.
			line:    "2026-10-15T14:30:12.93474614+08:00  INFO CORE::INSTANCE: [id] listener added",
			level:   "info",
			message: "INSTANCE: [id] listener added",
		},
		{
			line:    "2026-10-22T17:05:25.090973+08:00 ERROR quinn::endpoint: I/O error: socket closed",
			level:   "error",
			message: "endpoint: I/O error: socket closed",
		},
		{
			// Lines the client writes itself have no timestamp.
			line:    "配置已写入 /tmp/config.toml",
			level:   "info",
			message: "配置已写入 /tmp/config.toml",
		},
	} {
		level, message := parseLogLine(testCase.line)
		if level != testCase.level || message != testCase.message {
			t.Errorf("parseLogLine(%q) = %q, %q; want %q, %q",
				testCase.line, level, message, testCase.level, testCase.message)
		}
	}
}

// The instance directory can be given, which is what an elevated copy is told
// so that it uses the user's files instead of root's.
func TestUseInstanceRoot(t *testing.T) {
	dir := t.TempDir()
	UseInstanceRoot(dir)
	t.Cleanup(func() { UseInstanceRoot("") })

	got, err := InstanceRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("instance root = %q, want %q", got, dir)
	}
}
