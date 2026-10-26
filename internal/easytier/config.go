package easytier

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// Config is everything the client remembers between runs.
type Config struct {
	ACL           *ACL       `json:"acl,omitempty"`
	VPNPortal     *VPNPortal `json:"vpn_portal,omitempty"`
	NetworkName   string     `json:"network_name"`
	NetworkSecret string     `json:"network_secret"`
	Hostname      string     `json:"hostname"`
	IPv4          string     `json:"ipv4"`
	Peers         []string   `json:"peers"`
	Listeners     []string   `json:"listeners"`
	ProxyNetworks []string   `json:"proxy_networks"`
	PortForwards  []string   `json:"port_forwards"`
	Encryption    bool       `json:"encryption"`
	SecureMode    bool       `json:"secure_mode"`
	// SecureModePrivateKey/PublicKey are the node's secure-mode identity:
	// generated once when secure mode is switched on, then kept stable so a
	// TOML round trip or a restart does not rename the node. Base64 X25519.
	SecureModePrivateKey string `json:"secure_mode_private_key,omitempty"`
	SecureModePublicKey  string `json:"secure_mode_public_key,omitempty"`
	// DHCP nil keeps the old inference (IPv4 empty && EnableTUN); non-nil is
	// an explicit dhcp = true/false in the exported TOML.
	DHCP         *bool `json:"dhcp,omitempty"`
	LatencyFirst bool  `json:"latency_first"`
	EnableTUN    bool  `json:"enable_tun"`
	// LogLevel is what the core writes to its log file, and what the window
	// shows: "error" by default, because the core is chatty at "info" and
	// writing that to disk all day wears it out.
	LogLevel string `json:"log_level"`

	// ---- 接口与性能 ----

	DevName string `json:"dev_name,omitempty"`
	// MTU 0 leaves the core's default.
	MTU int `json:"mtu,omitempty"`
	// MultiThread and BindDevice default to true in the core; they are
	// always serialized so an explicit off survives a save.
	MultiThread bool `json:"multi_thread"`
	BindDevice  bool `json:"bind_device"`
	Smoltcp     bool `json:"smoltcp,omitempty"`
	// RecvBpsLimit 0 means unlimited.
	RecvBpsLimit uint64 `json:"recv_bps_limit,omitempty"`
	// SocketMark is the Linux SO_MARK (fwmark) applied to every outbound
	// underlay socket; nil leaves it untouched, and a non-nil 0 is a real
	// mark, so it is a pointer ([flags] socket_mark, main only; v2.6.4
	// ignores the key).
	SocketMark *uint32 `json:"socket_mark,omitempty"`

	// ---- 路由与出口 ----

	ExitNodes       []string `json:"exit_nodes,omitempty"`
	EnableExitNode  bool     `json:"enable_exit_node,omitempty"`
	RelayWhitelist  string   `json:"relay_whitelist,omitempty"`
	MappedListeners []string `json:"mapped_listeners,omitempty"`
	// DisableRelayData/PreferPeerRelay are main-only [flags] keys
	// (disable_relay_data / prefer_peer_relay); v2.6.4 ignores them.
	DisableRelayData bool `json:"disable_relay_data,omitempty"`
	PreferPeerRelay  bool `json:"prefer_peer_relay,omitempty"`
	// ManualRoutesOn expresses "manual routes enabled with an empty list",
	// which the core reads as routes = []: meaningful, unlike a missing key.
	ManualRoutesOn bool     `json:"manual_routes_on,omitempty"`
	ManualRoutes   []string `json:"manual_routes,omitempty"`

	// ---- 代理 ----

	// Socks5Port 0 disables the SOCKS5 server.
	Socks5Port int `json:"socks5_port,omitempty"`
	// Socks5Host is the address the SOCKS5 server binds; "" means 0.0.0.0
	// (the previous behaviour).
	Socks5Host           string `json:"socks5_host,omitempty"`
	ProxyForwardBySystem bool   `json:"proxy_forward_by_system,omitempty"`

	// ---- P2P 与打洞 ----

	DisableP2P             bool `json:"disable_p2p,omitempty"`
	P2POnly                bool `json:"p2p_only,omitempty"`
	LazyP2P                bool `json:"lazy_p2p,omitempty"`
	NeedP2P                bool `json:"need_p2p,omitempty"`
	RelayAllPeerRPC        bool `json:"relay_all_peer_rpc,omitempty"`
	DisableTCPHolePunching bool `json:"disable_tcp_hole_punching,omitempty"`
	DisableUDPHolePunching bool `json:"disable_udp_hole_punching,omitempty"`
	DisableSymHolePunching bool `json:"disable_sym_hole_punching,omitempty"`

	// ---- 传输 ----

	KCPProxy         bool `json:"kcp_proxy,omitempty"`
	DisableKCPInput  bool `json:"disable_kcp_input,omitempty"`
	QUICProxy        bool `json:"quic_proxy,omitempty"`
	DisableQUICInput bool `json:"disable_quic_input,omitempty"`
	// Compression is "" or "zstd".
	Compression string `json:"compression,omitempty"`
	// DefaultProtocol is "" (tcp) or one of udp/ws/wss/wg/quic.
	DefaultProtocol string `json:"default_protocol,omitempty"`
	// EncryptionAlgorithm is "" (aes-gcm) or one of the core's algorithms.
	EncryptionAlgorithm string `json:"encryption_algorithm,omitempty"`

	// ---- 其他 ----

	IPv6Address string `json:"ipv6_address,omitempty"`
	// EnableIPv6 defaults to true in the core; always serialized.
	EnableIPv6     bool `json:"enable_ipv6"`
	IPv6PublicAuto bool `json:"ipv6_public_auto,omitempty"`
	// IPv6PublicProvider is the top-level ipv6_public_addr_provider key: this
	// node offers its public IPv6 to peers (main only; v2.6.4 ignores it).
	IPv6PublicProvider bool `json:"ipv6_public_provider,omitempty"`
	DisableUPnP        bool `json:"disable_upnp,omitempty"`
	MagicDNS           bool `json:"magic_dns,omitempty"`
	PrivateMode        bool `json:"private_mode,omitempty"`
	UDPBroadcastRelay  bool `json:"udp_broadcast_relay,omitempty"`
}

func DefaultConfig() Config {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "easytier-client"
	}
	return Config{
		NetworkName: "easytier",
		Hostname:    hostname,
		Encryption:  true,
		EnableTUN:   true,
		MultiThread: true,
		BindDevice:  true,
		EnableIPv6:  true,
		LogLevel:    "error",
	}
}

// configFile, when set, overrides the configuration path; tests use it.
var configFile string

// SetConfigFile points the client at another configuration, returning a
// function that restores the previous one. Tests use it to stay out of the
// user's own files.
func SetConfigFile(path string) func() {
	previous := configFile
	configFile = path
	return func() { configFile = previous }
}

// instanceRootOverride is set when the client was told where its files are.
var instanceRootOverride string

// UseInstanceRoot points the client at a directory of its own.
//
// A restarted copy needs this: pkexec, sudo and osascript all hand the new
// process root's home, so without being told, the elevated copy would read and
// write a different configuration from the one the user just filled in.
func UseInstanceRoot(path string) {
	instanceRootOverride = path
}

// instanceRoot is the directory this client owns: its configuration, and
// everything the core it starts writes.
func InstanceRoot() (string, error) {
	if instanceRootOverride != "" {
		return instanceRootOverride, nil
	}
	if configFile != "" {
		return filepath.Dir(configFile), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "easytier-mygo"), nil
}

// configPath is <instance root>/config.json.
func configPath() (string, error) {
	if configFile != "" {
		return configFile, nil
	}
	root, err := InstanceRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.json"), nil
}

// loadConfig reads the saved configuration, or returns the defaults when
// nothing was saved yet. A corrupt file is reported instead of replaced, so
// the user does not lose it silently.
func LoadConfig() (Config, error) {
	path, err := configPath()
	if err != nil {
		return DefaultConfig(), err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return DefaultConfig(), fmt.Errorf("read %s: %w", path, err)
	}
	config := DefaultConfig()
	if err := json.Unmarshal(data, &config); err != nil {
		return DefaultConfig(), fmt.Errorf("parse %s: %w", path, err)
	}
	if !validLogLevel(config.LogLevel) {
		config.LogLevel = "error"
	}
	return config, nil
}

func (config Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// LogLevels are the levels the core understands, in the order the settings
// page offers them.
var LogLevels = []string{"off", "error", "warn", "info", "debug", "trace"}

// LogLevelName is what the settings page calls one level.
func LogLevelName(level string) string {
	switch level {
	case "off":
		return locale.T("关闭")
	case "debug":
		return locale.T("调试")
	case "trace":
		return locale.T("跟踪")
	case "warn":
		return locale.T("警告")
	case "info":
		return locale.T("信息")
	default:
		return locale.T("仅错误")
	}
}

func validLogLevel(level string) bool {
	for _, candidate := range LogLevels {
		if level == candidate {
			return true
		}
	}
	return false
}

// coreLogLevel is the level to pass to the core, defaulting to error.
func (config Config) coreLogLevel() string {
	if validLogLevel(config.LogLevel) {
		return config.LogLevel
	}
	return "error"
}

// lines splits a multi-line editor's text into trimmed, non-empty entries.
func SplitLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func JoinLines(values []string) string { return strings.Join(values, "\n") }
