package easytier

import (
	"crypto/ecdh"
	"encoding/base64"
	"strings"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

// A secure-mode identity survives export → import → export unchanged: the
// node keeps its name in the network instead of being rekeyed every time.
func TestSecureModeKeysRoundTrip(t *testing.T) {
	config := Config{NetworkName: "n", NetworkSecret: "s", SecureMode: true}
	if err := EnsureSecureModeKeys(&config); err != nil {
		t.Fatal(err)
	}
	first, err := ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, `local_private_key = "`+config.SecureModePrivateKey+`"`) {
		t.Fatalf("export does not carry the stored key:\n%s", first)
	}
	back, err := ParseConfigTOML(first)
	if err != nil {
		t.Fatal(err)
	}
	if back.SecureModePrivateKey != config.SecureModePrivateKey ||
		back.SecureModePublicKey != config.SecureModePublicKey {
		t.Fatal("import changed the secure-mode identity")
	}
	second, err := ExportConfigTOML(back)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(second, `local_private_key = "`+config.SecureModePrivateKey+`"`) {
		t.Fatalf("re-export changed the identity:\n%s", second)
	}
}

// A secure_mode section without keys is accepted; the pair appears on
// export, generated once by EnsureSecureModeKeys.
func TestSecureModeImportWithoutKeys(t *testing.T) {
	config, err := ParseConfigTOML(`
[network_identity]
network_name = "n"
network_secret = "s"

[secure_mode]
enabled = true
`)
	if err != nil {
		t.Fatal(err)
	}
	if !config.SecureMode || config.SecureModePrivateKey != "" {
		t.Fatalf("secure mode = %+v", config)
	}
	if err := EnsureSecureModeKeys(&config); err != nil {
		t.Fatal(err)
	}
	text, err := ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "local_private_key") {
		t.Fatalf("export lacks the generated keys:\n%s", text)
	}
}

// Keys a file carries are checked, never silently replaced.
func TestSecureModeImportRejectsBadKeys(t *testing.T) {
	for _, key := range []string{"not-base64!!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		_, err := ParseConfigTOML(`
[network_identity]
network_name = "n"
network_secret = "s"

[secure_mode]
enabled = true
local_private_key = "` + key + `"
`)
		if err == nil {
			t.Fatalf("key %q was accepted", key)
		}
	}
}

// dhcp = true/false both survive the round trip when they are explicit.
func TestDHCPExplicitRoundTrip(t *testing.T) {
	for _, value := range []bool{true, false} {
		config := Config{NetworkName: "n", EnableTUN: true, DHCP: boolPtr(value)}
		text, err := ExportConfigTOML(config)
		if err != nil {
			t.Fatal(err)
		}
		want := "dhcp = false"
		if value {
			want = "dhcp = true"
		}
		if !strings.Contains(text, want) {
			t.Fatalf("export lacks %q:\n%s", want, text)
		}
		back, err := ParseConfigTOML(text)
		if err != nil {
			t.Fatal(err)
		}
		if back.DHCP == nil || *back.DHCP != value {
			t.Fatalf("DHCP = %v, want explicit %v", back.DHCP, value)
		}
	}
}

// Old profiles carry no DHCP field: the export keeps inferring dhcp = true
// from an empty IPv4 with the TUN on, and writes nothing with the TUN off.
func TestDHCPInferenceWithoutExplicitValue(t *testing.T) {
	config := Config{NetworkName: "n", EnableTUN: true}
	text, err := ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "dhcp = true") {
		t.Fatalf("old inference lost:\n%s", text)
	}
	config.EnableTUN = false
	text, err = ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "dhcp") {
		t.Fatalf("dhcp written without a reason:\n%s", text)
	}
	// A fixed address wins over an explicit dhcp = true: the file carries
	// both, the export only the address.
	back, err := ParseConfigTOML("ipv4 = \"10.144.0.2/24\"\ndhcp = true\n[network_identity]\nnetwork_name = \"n\"\n")
	if err != nil {
		t.Fatal(err)
	}
	text, err = ExportConfigTOML(back)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "ipv4") || strings.Contains(text, "dhcp") {
		t.Fatalf("ipv4 should win over dhcp:\n%s", text)
	}
}

// routes = [] is enabled manual routing with an empty list, not a missing
// key; a missing key is manual routing off.
func TestManualRoutesEmptyRoundTrip(t *testing.T) {
	config := Config{NetworkName: "n", EnableTUN: false, ManualRoutesOn: true}
	text, err := ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "routes = []") {
		t.Fatalf("empty routes not written:\n%s", text)
	}
	back, err := ParseConfigTOML(text)
	if err != nil {
		t.Fatal(err)
	}
	if !back.ManualRoutesOn || len(back.ManualRoutes) != 0 {
		t.Fatalf("routes = [] lost: %+v", back)
	}

	// Entries imply enabled, as before.
	back, err = ParseConfigTOML("routes = [\"192.168.0.0/16\"]\n[network_identity]\nnetwork_name = \"n\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !back.ManualRoutesOn || len(back.ManualRoutes) != 1 {
		t.Fatalf("routes with entries: %+v", back)
	}
	// A missing key stays off.
	back, err = ParseConfigTOML("[network_identity]\nnetwork_name = \"n\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if back.ManualRoutesOn {
		t.Fatalf("missing routes key enabled manual routing: %+v", back)
	}
}

// Any bind address survives, including IPv6; 0.0.0.0 normalizes to the
// model's default.
func TestSocks5RoundTrip(t *testing.T) {
	for _, testCase := range []struct{ host, written string }{
		{"127.0.0.1", "127.0.0.1"},
		{"::1", "[::1]"},
		{"localhost", "localhost"},
		{"", "0.0.0.0"},
	} {
		config := Config{NetworkName: "n", EnableTUN: false, Socks5Port: 1080, Socks5Host: testCase.host}
		text, err := ExportConfigTOML(config)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "socks5://"+testCase.written+":1080") {
			t.Fatalf("host %q exported as:\n%s", testCase.host, text)
		}
		back, err := ParseConfigTOML(text)
		if err != nil {
			t.Fatal(err)
		}
		if back.Socks5Host != testCase.host || back.Socks5Port != 1080 {
			t.Fatalf("host %q round-tripped to %q:%d", testCase.host, back.Socks5Host, back.Socks5Port)
		}
	}
	for _, bad := range []string{
		`socks5_proxy = "http://127.0.0.1:1080"`,
		`socks5_proxy = "socks5://:1080"`,
		`socks5_proxy = "socks5://127.0.0.1:0"`,
		`socks5_proxy = "socks5://127.0.0.1:70000"`,
	} {
		if _, err := ParseConfigTOML(bad + "\n[network_identity]\nnetwork_name = \"n\"\n"); err == nil {
			t.Fatalf("%s was accepted", bad)
		}
	}
}

// EnsureSecureModeKeys generates a valid pair once and then leaves it alone.
func TestEnsureSecureModeKeys(t *testing.T) {
	config := Config{NetworkName: "n", NetworkSecret: "s"}
	if err := EnsureSecureModeKeys(&config); err != nil {
		t.Fatal(err)
	}
	if config.SecureModePrivateKey != "" {
		t.Fatal("keys generated although secure mode is off")
	}
	config.SecureMode = true
	if err := EnsureSecureModeKeys(&config); err != nil {
		t.Fatal(err)
	}
	privateBytes, err := base64.StdEncoding.DecodeString(config.SecureModePrivateKey)
	if err != nil || len(privateBytes) != 32 {
		t.Fatalf("private key = %q", config.SecureModePrivateKey)
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		t.Fatal(err)
	}
	if base64.StdEncoding.EncodeToString(privateKey.PublicKey().Bytes()) != config.SecureModePublicKey {
		t.Fatal("the public key does not match the private key")
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	private, public := config.SecureModePrivateKey, config.SecureModePublicKey
	if err := EnsureSecureModeKeys(&config); err != nil {
		t.Fatal(err)
	}
	if config.SecureModePrivateKey != private || config.SecureModePublicKey != public {
		t.Fatal("EnsureSecureModeKeys is not idempotent")
	}
}

// Validate names broken identities and bind addresses instead of letting
// the core fail later.
func TestValidateSecureModeKeysAndSocks5Host(t *testing.T) {
	valid := Config{NetworkName: "n", NetworkSecret: "s", SecureMode: true}
	if err := EnsureSecureModeKeys(&valid); err != nil {
		t.Fatal(err)
	}
	other := Config{SecureMode: true}
	if err := EnsureSecureModeKeys(&other); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name   string
		change func(*Config)
	}{
		{"only private", func(c *Config) { c.SecureModePublicKey = "" }},
		{"bad base64", func(c *Config) { c.SecureModePrivateKey = "!!" }},
		{"short key", func(c *Config) {
			c.SecureModePrivateKey = base64.StdEncoding.EncodeToString([]byte("short"))
		}},
		{"mismatched pair", func(c *Config) { c.SecureModePublicKey = other.SecureModePublicKey }},
		{"host with space", func(c *Config) { c.Socks5Host = "bad host" }},
		{"host with slash", func(c *Config) { c.Socks5Host = "10.0.0.1/24" }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config := valid
			testCase.change(&config)
			if config.Validate() == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	for _, host := range []string{"", "127.0.0.1", "::1", "localhost", "proxy.lan"} {
		config := valid
		config.Socks5Host = host
		if err := config.Validate(); err != nil {
			t.Errorf("host %q rejected: %v", host, err)
		}
	}
}
