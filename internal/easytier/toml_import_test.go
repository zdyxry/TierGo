package easytier

import (
	"reflect"
	"strings"
	"testing"
)

// What exports imports back unchanged: backup and sharing lose nothing the
// settings page knows about.
func TestConfigTOMLRoundTrip(t *testing.T) {
	config := Config{
		NetworkName:   "office",
		NetworkSecret: "s3cret",
		Hostname:      "yiran-laptop",
		IPv4:          "10.144.0.10/24",
		Peers:         []string{"tcp://public.top:11010", "udp://1.2.3.4:11010"},
		Listeners:     []string{"tcp://0.0.0.0:11010"},
		ProxyNetworks: []string{"192.168.1.0/24"},
		PortForwards:  []string{"tcp://127.0.0.1:5202/10.144.0.20:5201"},
		Encryption:    true,
		SecureMode:    true,
		LatencyFirst:  true,
		EnableTUN:     false,
		LogLevel:      "error", // 客户端本地设置，不参与导出
	}
	if err := EnsureSecureModeKeys(&config); err != nil {
		t.Fatal(err)
	}
	text, err := ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	// An export carries no runtime additions.
	if strings.Contains(text, "instance_name") {
		t.Errorf("export leaks the instance name:\n%s", text)
	}
	back, err := ParseConfigTOML(text)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config, back) {
		t.Errorf("round trip changed the configuration:\nhave %+v\nwant %+v", back, config)
	}
}

// A file written for the official client (or by hand) loads; keys the
// client does not model are ignored rather than fatal.
func TestParseConfigTOMLToleratesUnknownKeys(t *testing.T) {
	text := `instance_name = "default"
hostname = "nas"
ipv4 = "10.144.0.2/24"
listeners = ["tcp://0.0.0.0:11010"]
mapped_listeners = ["tcp://1.2.3.4:12345"]
rpc_portal = "0.0.0.0:15888"

[network_identity]
network_name = "office"
network_secret = "abc"

[[peer]]
uri = "tcp://public.top:11010"

[[port_forward]]
bind_addr = "0.0.0.0:8080"
dst_addr = "10.144.0.20:80"
proto = "tcp"

[flags]
no_tun = false
enable_encryption = false
secure_mode = false
latency_first = true
dev_name = "et0"
mtu = 1400
`
	config, err := ParseConfigTOML(text)
	if err != nil {
		t.Fatal(err)
	}
	if config.NetworkName != "office" || config.NetworkSecret != "abc" {
		t.Errorf("identity = %q/%q", config.NetworkName, config.NetworkSecret)
	}
	if config.Hostname != "nas" || config.IPv4 != "10.144.0.2/24" {
		t.Errorf("host = %q %q", config.Hostname, config.IPv4)
	}
	if len(config.Peers) != 1 || config.Peers[0] != "tcp://public.top:11010" {
		t.Errorf("peers = %v", config.Peers)
	}
	if len(config.PortForwards) != 1 ||
		config.PortForwards[0] != "tcp://0.0.0.0:8080/10.144.0.20:80" {
		t.Errorf("forwards = %v", config.PortForwards)
	}
	if !config.EnableTUN || config.Encryption || config.SecureMode || !config.LatencyFirst {
		t.Errorf("flags = %+v", config)
	}
}

// Defaults survive a file that never mentions them: encryption stays on.
func TestParseConfigTOMLKeepsDefaults(t *testing.T) {
	config, err := ParseConfigTOML("[network_identity]\nnetwork_name = \"n\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !config.Encryption || !config.EnableTUN {
		t.Errorf("defaults were lost: %+v", config)
	}
}

func TestParseConfigTOMLRequiresANetworkName(t *testing.T) {
	if _, err := ParseConfigTOML("hostname = \"x\"\n"); err == nil {
		t.Fatal("a config without a network name must be rejected")
	}
}

func TestParseConfigTOMLRejectsGarbage(t *testing.T) {
	if _, err := ParseConfigTOML("not = [valid"); err == nil {
		t.Fatal("broken TOML must be rejected")
	}
	if _, err := ParseConfigTOML(`
[network_identity]
network_name = "n"
[[port_forward]]
proto = "tcp"
`); err == nil {
		t.Fatal("an incomplete port forward must be rejected")
	}
}
