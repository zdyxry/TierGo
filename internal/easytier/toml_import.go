package easytier

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/pelletier/go-toml/v2"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// Import/export is a projection through the settings model, not a lossless
// editor for arbitrary Core fields. Unknown keys are not retained.
type tomlDocument struct {
	ACL       *ACL       `toml:"acl"`
	VPNPortal *VPNPortal `toml:"vpn_portal_config"`
	Hostname  string     `toml:"hostname"`
	IPv4      string     `toml:"ipv4"`
	// DHCP is a pointer so an explicit dhcp = false survives the trip
	// through the model; nil means the key was absent.
	DHCP               *bool    `toml:"dhcp"`
	IPv6               string   `toml:"ipv6"`
	IPv6PublicAuto     bool     `toml:"ipv6_public_addr_auto"`
	IPv6PublicProvider bool     `toml:"ipv6_public_addr_provider"`
	Listeners          []string `toml:"listeners"`
	MappedListeners    []string `toml:"mapped_listeners"`
	ExitNodes          []string `toml:"exit_nodes"`
	// Routes is a pointer so routes = [] (enabled, empty) differs from a
	// missing key.
	Routes      *[]string `toml:"routes"`
	Socks5Proxy string    `toml:"socks5_proxy"`
	// Accept the earlier client's key only to migrate its exports. Core
	// itself ignores it; exports always use [[proxy_network]].
	LegacyProxyNetworks []string `toml:"proxy_networks"`
	ProxyNetworks       []struct {
		CIDR       string `toml:"cidr"`
		MappedCIDR string `toml:"mapped_cidr"`
	} `toml:"proxy_network"`
	Identity struct {
		NetworkName   string `toml:"network_name"`
		NetworkSecret string `toml:"network_secret"`
	} `toml:"network_identity"`
	Peers []struct {
		URI string `toml:"uri"`
	} `toml:"peer"`
	PortForwards []struct {
		BindAddr string `toml:"bind_addr"`
		DstAddr  string `toml:"dst_addr"`
		Proto    string `toml:"proto"`
	} `toml:"port_forward"`
	SecureMode struct {
		Enabled         bool   `toml:"enabled"`
		LocalPrivateKey string `toml:"local_private_key"`
		LocalPublicKey  string `toml:"local_public_key"`
	} `toml:"secure_mode"`
	Flags tomlFlags `toml:"flags"`
}

// Concrete types make malformed flags an error instead of silently falling
// back to a default. Seed the Core's true defaults before decoding.
type tomlFlags struct {
	NoTUN                  bool    `toml:"no_tun"`
	EnableEncryption       bool    `toml:"enable_encryption"`
	SecureMode             bool    `toml:"secure_mode"`
	LatencyFirst           bool    `toml:"latency_first"`
	DevName                string  `toml:"dev_name"`
	MTU                    int     `toml:"mtu"`
	SocketMark             *uint32 `toml:"socket_mark"`
	MultiThread            bool    `toml:"multi_thread"`
	BindDevice             bool    `toml:"bind_device"`
	Smoltcp                bool    `toml:"use_smoltcp"`
	RecvBpsLimit           uint64  `toml:"instance_recv_bps_limit"`
	EnableExitNode         bool    `toml:"enable_exit_node"`
	DisableRelayData       bool    `toml:"disable_relay_data"`
	PreferPeerRelay        bool    `toml:"prefer_peer_relay"`
	RelayWhitelist         *string `toml:"relay_network_whitelist"`
	ProxyForwardBySystem   bool    `toml:"proxy_forward_by_system"`
	DisableP2P             bool    `toml:"disable_p2p"`
	P2POnly                bool    `toml:"p2p_only"`
	LazyP2P                bool    `toml:"lazy_p2p"`
	NeedP2P                bool    `toml:"need_p2p"`
	RelayAllPeerRPC        bool    `toml:"relay_all_peer_rpc"`
	DisableTCPHolePunching bool    `toml:"disable_tcp_hole_punching"`
	DisableUDPHolePunching bool    `toml:"disable_udp_hole_punching"`
	DisableSymHolePunching bool    `toml:"disable_sym_hole_punching"`
	KCPProxy               bool    `toml:"enable_kcp_proxy"`
	DisableKCPInput        bool    `toml:"disable_kcp_input"`
	QUICProxy              bool    `toml:"enable_quic_proxy"`
	DisableQUICInput       bool    `toml:"disable_quic_input"`
	Compression            int     `toml:"data_compress_algo"`
	DefaultProtocol        string  `toml:"default_protocol"`
	EncryptionAlgorithm    string  `toml:"encryption_algorithm"`
	EnableIPv6             bool    `toml:"enable_ipv6"`
	MagicDNS               bool    `toml:"accept_dns"`
	PrivateMode            bool    `toml:"private_mode"`
	DisableUPnP            bool    `toml:"disable_upnp"`
	UDPBroadcastRelay      bool    `toml:"enable_udp_broadcast_relay"`
}

// ParseConfigTOML reads a core configuration into the client's model.
func ParseConfigTOML(data string) (Config, error) {
	document := tomlDocument{Flags: tomlFlags{
		EnableEncryption: true, MultiThread: true, BindDevice: true, EnableIPv6: true,
	}}
	if err := toml.Unmarshal([]byte(data), &document); err != nil {
		return Config{}, fmt.Errorf(locale.T("TOML 解析失败：%w"), err)
	}
	if document.VPNPortal != nil && document.VPNPortal.ClientCIDR == "" {
		document.VPNPortal.DeviceMode = true
	}
	f := document.Flags
	config := Config{
		ACL:         document.ACL,
		VPNPortal:   document.VPNPortal,
		NetworkName: document.Identity.NetworkName, NetworkSecret: document.Identity.NetworkSecret,
		Hostname: document.Hostname, IPv4: document.IPv4, IPv6Address: document.IPv6,
		IPv6PublicAuto: document.IPv6PublicAuto, IPv6PublicProvider: document.IPv6PublicProvider,
		Listeners:       document.Listeners,
		MappedListeners: document.MappedListeners, ExitNodes: document.ExitNodes,
		DHCP: document.DHCP, LogLevel: "error",
		EnableTUN: !f.NoTUN, Encryption: f.EnableEncryption, SecureMode: f.SecureMode || document.SecureMode.Enabled,
		LatencyFirst: f.LatencyFirst, DevName: f.DevName, MTU: f.MTU, SocketMark: f.SocketMark,
		MultiThread: f.MultiThread, BindDevice: f.BindDevice, Smoltcp: f.Smoltcp,
		RecvBpsLimit: f.RecvBpsLimit, EnableExitNode: f.EnableExitNode,
		DisableRelayData: f.DisableRelayData, PreferPeerRelay: f.PreferPeerRelay,
		ProxyForwardBySystem: f.ProxyForwardBySystem,
		DisableP2P:           f.DisableP2P, P2POnly: f.P2POnly, LazyP2P: f.LazyP2P, NeedP2P: f.NeedP2P,
		RelayAllPeerRPC: f.RelayAllPeerRPC, DisableTCPHolePunching: f.DisableTCPHolePunching,
		DisableUDPHolePunching: f.DisableUDPHolePunching, DisableSymHolePunching: f.DisableSymHolePunching,
		KCPProxy: f.KCPProxy, DisableKCPInput: f.DisableKCPInput,
		QUICProxy: f.QUICProxy, DisableQUICInput: f.DisableQUICInput,
		DefaultProtocol: f.DefaultProtocol, EncryptionAlgorithm: f.EncryptionAlgorithm,
		EnableIPv6: f.EnableIPv6, MagicDNS: f.MagicDNS, PrivateMode: f.PrivateMode,
		DisableUPnP: f.DisableUPnP, UDPBroadcastRelay: f.UDPBroadcastRelay,
	}
	if f.RelayWhitelist != nil {
		config.RelayWhitelist = *f.RelayWhitelist
		if config.RelayWhitelist == "" {
			config.RelayWhitelist = "-"
		}
	}
	if document.Routes != nil {
		// A routes key, even an empty one, is manual routing switched on.
		config.ManualRoutesOn = true
		config.ManualRoutes = *document.Routes
	}
	if config.SecureMode {
		// Keys a file carries belong to the node's identity; they are
		// checked, never silently replaced. An enabled section without
		// keys gets a pair from EnsureSecureModeKeys later.
		privateKey, err := decodeSecureModeKey(document.SecureMode.LocalPrivateKey)
		if err != nil {
			return Config{}, err
		}
		publicKey, err := decodeSecureModeKey(document.SecureMode.LocalPublicKey)
		if err != nil {
			return Config{}, err
		}
		config.SecureModePrivateKey, config.SecureModePublicKey = privateKey, publicKey
	}
	switch f.Compression {
	case 0, 1:
	case 2:
		config.Compression = "zstd"
	default:
		return Config{}, fmt.Errorf(locale.T("不支持的压缩算法编号 %d"), f.Compression)
	}
	if document.Socks5Proxy != "" {
		proxy, err := url.Parse(document.Socks5Proxy)
		if err != nil || proxy.Scheme != "socks5" || proxy.Hostname() == "" {
			return Config{}, fmt.Errorf(locale.T("SOCKS5 代理地址 %q 无效，应为 socks5://地址:端口"), document.Socks5Proxy)
		}
		port, err := strconv.Atoi(proxy.Port())
		if err != nil || port < 1 || port > 65535 {
			return Config{}, errors.New(locale.T("SOCKS5 端口无效"))
		}
		// Hostname() strips the brackets an IPv6 literal wears. 0.0.0.0 is
		// the default bind, which the model spells "".
		config.Socks5Host = proxy.Hostname()
		if config.Socks5Host == "0.0.0.0" {
			config.Socks5Host = ""
		}
		config.Socks5Port = port
	}
	config.ProxyNetworks = document.LegacyProxyNetworks
	for _, network := range document.ProxyNetworks {
		value := network.CIDR
		if network.MappedCIDR != "" {
			value += "->" + network.MappedCIDR
		}
		config.ProxyNetworks = append(config.ProxyNetworks, value)
	}
	for _, peer := range document.Peers {
		if peer.URI != "" {
			config.Peers = append(config.Peers, peer.URI)
		}
	}
	for _, forward := range document.PortForwards {
		config.PortForwards = append(config.PortForwards,
			fmt.Sprintf("%s://%s/%s", forward.Proto, forward.BindAddr, forward.DstAddr))
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// ExportConfigTOML excludes runtime additions (instance name and chosen ports).
func ExportConfigTOML(config Config) (string, error) {
	return renderConfigTOML(config, "", nil)
}

// decodeSecureModeKey checks one secure_mode key: base64 of a 32-byte X25519
// key, or empty when the key was not given.
func decodeSecureModeKey(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return "", fmt.Errorf(locale.T("安全模式密钥 %q 无效，应为 32 字节 X25519 密钥的 base64"), value)
	}
	return value, nil
}
