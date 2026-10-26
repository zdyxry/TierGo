package easytier

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// freePort asks the kernel for a port free for both TCP and UDP on address.
func freePort(address string) (int, error) {
	for range 20 {
		listener, err := net.Listen("tcp", address+":0")
		if err != nil {
			return 0, err
		}
		port := listener.Addr().(*net.TCPAddr).Port
		packet, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", address, port))
		listener.Close()
		if err != nil {
			continue
		}
		packet.Close()
		return port, nil
	}
	return 0, errors.New("no free port")
}

// pickPorts returns the listening port and the management port. A listener
// the user configured wins; otherwise a free port is chosen so the client
// never fights another EasyTier on the machine.
func pickPorts(listeners []string) (int, int, error) {
	listenPort := 0
	if len(listeners) > 0 {
		if port, err := portOf(listeners[0]); err == nil {
			listenPort = port
		}
	}
	if listenPort == 0 {
		port, err := freePort("0.0.0.0")
		if err != nil {
			return 0, 0, err
		}
		listenPort = port
	}
	portal, err := freePort("127.0.0.1")
	if err != nil {
		return 0, 0, err
	}
	if portal == listenPort {
		return pickPorts(listeners)
	}
	return listenPort, portal, nil
}

// portOf reads the port of a listener, which may be a bare number as
// easytier-core accepts ("11010").
func portOf(endpoint string) (int, error) {
	index := strings.LastIndex(endpoint, ":")
	if index < 0 {
		return strconv.Atoi(strings.TrimSpace(endpoint))
	}
	return strconv.Atoi(strings.Trim(endpoint[index+1:], "/"))
}

// renderConfigTOML is the shared body of the instance configuration and the
// exported file: what differs is the runtime additions (instance name, and
// the ports the client picked when the user configured no listeners).
func renderConfigTOML(config Config, instanceName string, defaultListeners []string) (string, error) {
	if err := config.Validate(); err != nil {
		return "", err
	}
	var document strings.Builder
	if instanceName != "" {
		writeTOMLString(&document, "instance_name", instanceName)
	}
	if config.Hostname != "" {
		writeTOMLString(&document, "hostname", config.Hostname)
	}
	if config.IPv4 != "" {
		// A fixed address wins over DHCP; both appearing is accepted and
		// exported with the address only.
		writeTOMLString(&document, "ipv4", config.IPv4)
	} else if config.DHCP != nil {
		writeTOMLBool(&document, "dhcp", *config.DHCP)
	} else if config.EnableTUN {
		// Without a fixed address the core takes one from the network,
		// which it needs before it can create the device.
		writeTOMLBool(&document, "dhcp", true)
	}
	if config.IPv6Address != "" {
		writeTOMLString(&document, "ipv6", config.IPv6Address)
	}
	if config.IPv6PublicAuto {
		writeTOMLBool(&document, "ipv6_public_addr_auto", true)
	}
	if config.IPv6PublicProvider {
		writeTOMLBool(&document, "ipv6_public_addr_provider", true)
	}
	listeners := config.Listeners
	if len(listeners) == 0 {
		listeners = defaultListeners
	}
	if len(listeners) > 0 {
		writeTOMLStringArray(&document, "listeners", listeners)
	}
	if len(config.MappedListeners) > 0 {
		writeTOMLStringArray(&document, "mapped_listeners", config.MappedListeners)
	}
	if len(config.ExitNodes) > 0 {
		writeTOMLStringArray(&document, "exit_nodes", config.ExitNodes)
	}
	if len(config.ManualRoutes) > 0 {
		writeTOMLStringArray(&document, "routes", config.ManualRoutes)
	} else if config.ManualRoutesOn {
		// routes = [] is not a missing key: it disables learned routes.
		writeTOMLStringArray(&document, "routes", nil)
	}
	if config.Socks5Port > 0 {
		host := config.Socks5Host
		if host == "" {
			host = "0.0.0.0"
		}
		writeTOMLString(&document, "socks5_proxy",
			"socks5://"+net.JoinHostPort(host, strconv.Itoa(config.Socks5Port)))
	}
	for _, network := range config.ProxyNetworks {
		cidr, mapped, err := parseProxyNetwork(network)
		if err != nil {
			return "", err
		}
		// The core's table form; a bare "proxy_networks" key is silently
		// ignored, so never write one.
		document.WriteString("\n[[proxy_network]]\n")
		writeTOMLString(&document, "cidr", cidr)
		if mapped != "" {
			writeTOMLString(&document, "mapped_cidr", mapped)
		}
	}
	document.WriteString("\n[network_identity]\n")
	writeTOMLString(&document, "network_name", config.NetworkName)
	writeTOMLString(&document, "network_secret", config.NetworkSecret)
	for _, peer := range config.Peers {
		document.WriteString("\n[[peer]]\n")
		writeTOMLString(&document, "uri", peer)
	}
	for _, rule := range config.PortForwards {
		protocol, bind, destination, err := parsePortForward(rule)
		if err != nil {
			return "", err
		}
		document.WriteString("\n[[port_forward]]\n")
		writeTOMLString(&document, "bind_addr", bind)
		writeTOMLString(&document, "dst_addr", destination)
		writeTOMLString(&document, "proto", protocol)
	}
	if config.SecureMode {
		// secure_mode is a top-level table, not a flag. The stored key pair
		// keeps the node's identity stable across exports and restarts; a
		// fresh pair is only generated when the stored one is incomplete.
		privateKey, publicKey := config.SecureModePrivateKey, config.SecureModePublicKey
		if privateKey == "" || publicKey == "" {
			key, err := ecdh.X25519().GenerateKey(rand.Reader)
			if err != nil {
				return "", fmt.Errorf(locale.T("生成安全模式密钥失败：%w"), err)
			}
			privateKey = base64.StdEncoding.EncodeToString(key.Bytes())
			publicKey = base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
		}
		document.WriteString("\n[secure_mode]\n")
		writeTOMLBool(&document, "enabled", true)
		writeTOMLString(&document, "local_private_key", privateKey)
		writeTOMLString(&document, "local_public_key", publicKey)
	}
	document.WriteString("\n[flags]\n")
	writeTOMLBool(&document, "no_tun", !config.EnableTUN)
	writeTOMLBool(&document, "enable_encryption", config.Encryption)
	writeTOMLBool(&document, "latency_first", config.LatencyFirst)
	writeConfigFlags(&document, config)
	if config.VPNPortal != nil {
		portal := *config.VPNPortal
		if err := EnsurePortalKey(&portal); err != nil {
			return "", err
		}
		if portal.DeviceMode {
			portal.ClientCIDR = ""
		}
		data, err := toml.Marshal(struct {
			Portal *VPNPortal `toml:"vpn_portal_config"`
		}{&portal})
		if err != nil {
			return "", err
		}
		document.WriteByte('\n')
		document.Write(data)
	}
	if config.ACL != nil {
		data, err := toml.Marshal(struct {
			ACL *ACL `toml:"acl"`
		}{config.ACL.Sorted()})
		if err != nil {
			return "", fmt.Errorf(locale.T("编码 ACL 失败：%w"), err)
		}
		document.WriteByte('\n')
		document.Write(data)
	}
	return document.String(), nil
}

// writeConfigFlags writes the flags whose value differs from the core's
// default; an unset flag keeps the default, and a flag the core is too old
// to know is dropped by it, so only differences are ever written. Verified
// against the embedded v2.6.4 binary (2.6.4-8428a89d): a config carrying the
// main-only keys disable_relay_data, prefer_peer_relay, socket_mark and
// ipv6_public_addr_provider starts normally, the unknown keys are ignored.
func writeConfigFlags(document *strings.Builder, config Config) {
	if config.DevName != "" {
		writeTOMLString(document, "dev_name", config.DevName)
	}
	if config.MTU > 0 {
		fmt.Fprintf(document, "mtu = %d\n", config.MTU)
	}
	if config.SocketMark != nil {
		fmt.Fprintf(document, "socket_mark = %d\n", *config.SocketMark)
	}
	if !config.MultiThread {
		writeTOMLBool(document, "multi_thread", false)
	}
	if !config.BindDevice {
		writeTOMLBool(document, "bind_device", false)
	}
	if config.Smoltcp {
		writeTOMLBool(document, "use_smoltcp", true)
	}
	if config.RecvBpsLimit > 0 {
		fmt.Fprintf(document, "instance_recv_bps_limit = %d\n", config.RecvBpsLimit)
	}
	if config.EnableExitNode {
		writeTOMLBool(document, "enable_exit_node", true)
	}
	if config.DisableRelayData {
		writeTOMLBool(document, "disable_relay_data", true)
	}
	if config.PreferPeerRelay {
		writeTOMLBool(document, "prefer_peer_relay", true)
	}
	if config.RelayWhitelist != "" {
		value := config.RelayWhitelist
		if value == "-" {
			value = ""
		} // explicit deny-all, unlike the default '*'
		writeTOMLString(document, "relay_network_whitelist", value)
	}
	if config.ProxyForwardBySystem {
		writeTOMLBool(document, "proxy_forward_by_system", true)
	}
	if config.DisableP2P {
		writeTOMLBool(document, "disable_p2p", true)
	}
	if config.P2POnly {
		writeTOMLBool(document, "p2p_only", true)
	}
	if config.LazyP2P {
		writeTOMLBool(document, "lazy_p2p", true)
	}
	if config.NeedP2P {
		writeTOMLBool(document, "need_p2p", true)
	}
	if config.RelayAllPeerRPC {
		writeTOMLBool(document, "relay_all_peer_rpc", true)
	}
	if config.DisableTCPHolePunching {
		writeTOMLBool(document, "disable_tcp_hole_punching", true)
	}
	if config.DisableUDPHolePunching {
		writeTOMLBool(document, "disable_udp_hole_punching", true)
	}
	if config.DisableSymHolePunching {
		writeTOMLBool(document, "disable_sym_hole_punching", true)
	}
	if config.KCPProxy {
		writeTOMLBool(document, "enable_kcp_proxy", true)
	}
	if config.DisableKCPInput {
		writeTOMLBool(document, "disable_kcp_input", true)
	}
	if config.QUICProxy {
		writeTOMLBool(document, "enable_quic_proxy", true)
	}
	if config.DisableQUICInput {
		writeTOMLBool(document, "disable_quic_input", true)
	}
	if config.Compression == "zstd" {
		// The schema's enum serializes as a number: 1 is none, 2 is zstd.
		document.WriteString("data_compress_algo = 2\n")
	}
	if config.DefaultProtocol != "" {
		writeTOMLString(document, "default_protocol", config.DefaultProtocol)
	}
	if config.EncryptionAlgorithm != "" {
		writeTOMLString(document, "encryption_algorithm", config.EncryptionAlgorithm)
	}
	if !config.EnableIPv6 {
		writeTOMLBool(document, "enable_ipv6", false)
	}
	if config.MagicDNS {
		writeTOMLBool(document, "accept_dns", true)
	}
	if config.PrivateMode {
		writeTOMLBool(document, "private_mode", true)
	}
	if config.DisableUPnP {
		writeTOMLBool(document, "disable_upnp", true)
	}
	if config.UDPBroadcastRelay {
		writeTOMLBool(document, "enable_udp_broadcast_relay", true)
	}
}

// buildInstanceTOML writes the configuration the core reads. instanceName
// distinguishes this network's core when several run at once.
func buildInstanceTOML(config Config, listenPort int, instanceName string) (string, error) {
	defaults := []string{
		fmt.Sprintf("tcp://0.0.0.0:%d", listenPort),
		fmt.Sprintf("udp://0.0.0.0:%d", listenPort),
	}
	return renderConfigTOML(config, instanceName, defaults)
}

// parsePortForward reads one rule written as it is on the command line:
//
//	tcp://127.0.0.1:5202/10.144.0.20:5201
//
// which listens on the local address and forwards through the overlay to the
// other one. bind_addr is always a local address, so a forward is always
// outbound.
func parsePortForward(rule string) (protocol, bind, destination string, err error) {
	scheme, rest, found := strings.Cut(rule, "://")
	if !found {
		return "", "", "", fmt.Errorf(locale.T("端口转发 %q 缺少协议前缀（tcp:// 或 udp://）"), rule)
	}
	switch scheme {
	case "tcp", "udp":
	default:
		return "", "", "", fmt.Errorf(locale.T("端口转发 %q 的协议必须是 tcp 或 udp"), rule)
	}
	bind, destination, found = strings.Cut(rest, "/")
	if !found || bind == "" || destination == "" {
		return "", "", "", fmt.Errorf(locale.T("端口转发 %q 的格式应为 tcp://本机地址/对端地址"), rule)
	}
	if _, _, err := net.SplitHostPort(bind); err != nil {
		return "", "", "", fmt.Errorf(locale.T("端口转发 %q 的本机地址无效：%w"), rule, err)
	}
	if _, _, err := net.SplitHostPort(destination); err != nil {
		return "", "", "", fmt.Errorf(locale.T("端口转发 %q 的对端地址无效：%w"), rule, err)
	}
	return scheme, bind, destination, nil
}

func writeTOMLString(document *strings.Builder, key, value string) {
	fmt.Fprintf(document, "%s = %s\n", key, strconv.Quote(value))
}

func writeTOMLStringArray(document *strings.Builder, key string, values []string) {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	fmt.Fprintf(document, "%s = [%s]\n", key, strings.Join(quoted, ", "))
}

func writeTOMLBool(document *strings.Builder, key string, value bool) {
	fmt.Fprintf(document, "%s = %t\n", key, value)
}

// EnsureSecureModeKeys generates the node's secure-mode identity the first
// time secure mode is used and stores it in the configuration, so the key
// pair persists with the profile instead of changing on every export.
func EnsureSecureModeKeys(config *Config) error {
	if config == nil || !config.SecureMode {
		return nil
	}
	if config.SecureModePrivateKey != "" && config.SecureModePublicKey != "" {
		return nil
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf(locale.T("生成安全模式密钥失败：%w"), err)
	}
	config.SecureModePrivateKey = base64.StdEncoding.EncodeToString(key.Bytes())
	config.SecureModePublicKey = base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
	return nil
}
