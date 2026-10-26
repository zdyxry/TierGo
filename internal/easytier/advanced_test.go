package easytier

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func advancedConfig() Config {
	c := DefaultConfig()
	c.EnableTUN = false
	c.DevName, c.MTU, c.RecvBpsLimit = "et-test", 1400, 1048576
	c.MultiThread, c.BindDevice, c.EnableIPv6 = false, false, false
	c.Smoltcp, c.EnableExitNode, c.ProxyForwardBySystem = true, true, true
	c.ExitNodes = []string{"10.144.0.8"}
	c.MappedListeners = []string{"tcp://203.0.113.1:22334"}
	c.ManualRoutesOn = true
	c.ManualRoutes = []string{"192.168.0.0/16"}
	c.ProxyNetworks = []string{"172.16.0.0/24->192.168.1.0/24"}
	c.RelayWhitelist, c.Socks5Port = "-", 11080
	c.DisableP2P, c.P2POnly, c.LazyP2P, c.NeedP2P, c.RelayAllPeerRPC = true, true, true, true, true
	c.DisableTCPHolePunching, c.DisableUDPHolePunching, c.DisableSymHolePunching = true, true, true
	c.KCPProxy, c.DisableKCPInput, c.QUICProxy, c.DisableQUICInput = true, true, true, true
	c.Compression, c.DefaultProtocol, c.EncryptionAlgorithm = "zstd", "udp", "chacha20"
	c.IPv6Address, c.IPv6PublicAuto = "fd00::1/64", true
	c.DisableUPnP, c.MagicDNS, c.PrivateMode, c.UDPBroadcastRelay = true, true, true, true
	// main-only keys; v2.6.4 ignores them (verified: it starts with them set).
	c.DisableRelayData, c.PreferPeerRelay, c.IPv6PublicProvider = true, true, true
	socketMark := uint32(42)
	c.SocketMark = &socketMark
	return c
}

func TestAdvancedConfigRoundTrip(t *testing.T) {
	c := advancedConfig()
	text, err := ExportConfigTOML(c)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseConfigTOML(text)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, back) {
		t.Fatalf("round trip:\ngot %+v\nwant %+v", back, c)
	}
	if strings.Contains(text, "proxy_networks =") {
		t.Fatal("Core ignores proxy_networks")
	}
	if !strings.Contains(text, "data_compress_algo = 2") {
		t.Fatal("Core requires numeric compression enum")
	}
}

// socket_mark = 0 is a real mark, not "unset": the pointer must survive the
// TOML round trip, and an unset mark must not appear in the export.
func TestSocketMarkZeroAndUnset(t *testing.T) {
	c := testConfig()
	text, err := ExportConfigTOML(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "socket_mark") {
		t.Fatal("unset socket mark leaked into the export")
	}
	zero := uint32(0)
	c.SocketMark = &zero
	text, err = ExportConfigTOML(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "socket_mark = 0") {
		t.Fatalf("explicit zero mark missing:\n%s", text)
	}
	back, err := ParseConfigTOML(text)
	if err != nil {
		t.Fatal(err)
	}
	if back.SocketMark == nil || *back.SocketMark != 0 {
		t.Fatalf("zero mark did not round trip: %+v", back.SocketMark)
	}
}

func TestValidateAdvancedSettings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Config)
	}{
		{"MTU", func(c *Config) { c.MTU = 10 }},
		{"SOCKS5", func(c *Config) { c.Socks5Port = 65536 }},
		{"IPv4", func(c *Config) { c.IPv4 = "garbage" }},
		{"IPv6", func(c *Config) { c.IPv6Address = "garbage" }},
		{"proxy", func(c *Config) { c.ProxyNetworks = []string{"10.0.0.0/24->broken"} }},
		{"route", func(c *Config) { c.ManualRoutes = []string{"broken"} }},
		{"exit", func(c *Config) { c.ExitNodes = []string{"example.com"} }},
		{"compression", func(c *Config) { c.Compression = "gzip" }},
		{"protocol", func(c *Config) { c.DefaultProtocol = "bogus" }},
		{"cipher", func(c *Config) { c.EncryptionAlgorithm = "bogus" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultConfig()
			tc.change(&c)
			if c.Validate() == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

// Check not just acceptance (Core silently ignores unknown keys), but the
// normalized configuration returned by the actual v2.6.x Core over RPC.
func TestE2EAdvancedFlagsAreApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	core, cli := requireBinaries(t)
	root := t.TempDir()
	t.Cleanup(SetConfigFile(filepath.Join(root, "config.json")))
	config := advancedConfig()
	config.SecureMode, config.NetworkSecret = true, "integration-secret"
	// Do not change host DNS, expose a proxy, or enable an exit in tests.
	config.MagicDNS, config.EnableExitNode, config.IPv6PublicAuto = false, false, false
	config.Socks5Port, config.IPv6Address = 0, ""
	config.ExitNodes, config.ManualRoutes, config.MappedListeners = nil, nil, nil
	config.KCPProxy, config.QUICProxy, config.ProxyForwardBySystem = false, false, false
	controller := NewController("advanced")
	controller.Start(config)
	t.Cleanup(func() { controller.StopAndWait(10 * time.Second) })
	snap := waitForPhase(t, controller, PhaseRunning, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var node cliNode
	if err := cliQuery(ctx, cli, snap.Portal, "node", &node); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := toml.Unmarshal([]byte(node.Config), &doc); err != nil {
		t.Fatal(err)
	}
	secure, ok := doc["secure_mode"].(map[string]any)
	if !ok || secure["enabled"] != true {
		t.Fatalf("Core did not enable secure mode: %#v", doc["secure_mode"])
	}
	flags := doc["flags"].(map[string]any)
	for key, want := range map[string]any{
		"dev_name": "et-test", "mtu": int64(1400), "multi_thread": false,
		"bind_device": false, "enable_ipv6": false, "use_smoltcp": true,
		"instance_recv_bps_limit": int64(1048576), "data_compress_algo": int64(2),
		"disable_tcp_hole_punching": true, "lazy_p2p": true, "need_p2p": true,
	} {
		if !reflect.DeepEqual(flags[key], want) {
			t.Errorf("Core flag %s = %#v, want %#v", key, flags[key], want)
		}
	}
	if len(node.ProxyCIDRs) != 1 {
		t.Errorf("Core did not apply proxy networks: %v", node.ProxyCIDRs)
	}
	// Also verify the full form parses without launching privileged features.
	text, err := ExportConfigTOML(advancedConfig())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "check.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, core, "-c", path, "--check-config").CombinedOutput(); err != nil {
		t.Fatalf("Core rejected config: %v\n%s", err, output)
	}
}
