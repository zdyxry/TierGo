package easytier

import (
	"crypto/ecdh"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// Validate checks the fields the core would reject or silently ignore, so a
// mistake is named when the user saves rather than at connect time.
func (config Config) Validate() error {
	if strings.TrimSpace(config.NetworkName) == "" {
		return fmt.Errorf("%s", locale.T("网络名称不能为空"))
	}
	if config.SecureMode && config.NetworkSecret == "" {
		return fmt.Errorf("%s", locale.T("安全模式需要设置网络密码"))
	}
	if err := validateSecureModeKeys(config); err != nil {
		return err
	}
	if config.MTU != 0 && (config.MTU < 400 || config.MTU > 65000) {
		return fmt.Errorf(locale.T("MTU %d 无效，留空使用内核默认，或填 400-65000"), config.MTU)
	}
	if config.Socks5Port < 0 || config.Socks5Port > 65535 {
		return fmt.Errorf(locale.T("SOCKS5 端口 %d 无效"), config.Socks5Port)
	}
	if host := config.Socks5Host; host != "" {
		if net.ParseIP(host) == nil && !validSocks5Hostname(host) {
			return fmt.Errorf(locale.T("SOCKS5 绑定地址 %q 无效，应为 IP 或主机名"), host)
		}
	}
	if config.IPv4 != "" {
		if _, err := netip.ParsePrefix(config.IPv4); err != nil {
			if net.ParseIP(config.IPv4) == nil {
				return fmt.Errorf(locale.T("虚拟 IP %q 无效，例如 10.144.0.10/24"), config.IPv4)
			}
		}
	}
	if config.IPv6Address != "" {
		if _, err := netip.ParsePrefix(config.IPv6Address); err != nil {
			return fmt.Errorf(locale.T("IPv6 地址 %q 无效，例如 fd00::1/64"), config.IPv6Address)
		}
	}
	for _, network := range config.ProxyNetworks {
		if _, _, err := parseProxyNetwork(network); err != nil {
			return err
		}
	}
	for _, rule := range config.PortForwards {
		if _, _, _, err := parsePortForward(rule); err != nil {
			return err
		}
	}
	for _, node := range config.ExitNodes {
		if net.ParseIP(strings.TrimSpace(node)) == nil {
			return fmt.Errorf(locale.T("出口节点 %q 无效，填对端的虚拟 IP，例如 10.144.0.8"), node)
		}
	}
	for _, route := range config.ManualRoutes {
		if _, err := netip.ParsePrefix(strings.TrimSpace(route)); err != nil {
			return fmt.Errorf(locale.T("手动路由 %q 无效，例如 192.168.0.0/16"), route)
		}
	}
	for _, listener := range config.MappedListeners {
		if _, err := portOf(listener); err != nil || !strings.Contains(listener, "://") {
			return fmt.Errorf(locale.T("映射监听地址 %q 无效，例如 tcp://1.2.3.4:12345"), listener)
		}
	}
	switch config.Compression {
	case "", "none", "zstd":
	default:
		return fmt.Errorf(locale.T("压缩算法 %q 无效，可选 none 或 zstd"), config.Compression)
	}
	switch config.DefaultProtocol {
	case "", "tcp", "udp", "ws", "wss", "wg", "quic":
	default:
		return fmt.Errorf(locale.T("默认协议 %q 无效"), config.DefaultProtocol)
	}
	switch config.EncryptionAlgorithm {
	case "", "xor", "chacha20", "aes-gcm", "aes-256-gcm",
		"openssl-aes128-gcm", "openssl-aes256-gcm", "openssl-chacha20":
	default:
		return fmt.Errorf(locale.T("加密算法 %q 无效"), config.EncryptionAlgorithm)
	}
	if err := config.VPNPortal.Validate(); err != nil {
		return err
	}
	return config.ACL.Validate()
}

// validateSecureModeKeys checks a stored secure-mode identity: both keys,
// each base64 of 32 bytes, and the public half matching the private half.
// A missing pair is fine — EnsureSecureModeKeys generates one.
func validateSecureModeKeys(config Config) error {
	if config.SecureModePrivateKey == "" && config.SecureModePublicKey == "" {
		return nil
	}
	if config.SecureModePrivateKey == "" || config.SecureModePublicKey == "" {
		return fmt.Errorf("%s", locale.T("安全模式密钥不完整，需要成对的私钥与公钥"))
	}
	privateBytes, err := base64.StdEncoding.DecodeString(config.SecureModePrivateKey)
	if err != nil || len(privateBytes) != 32 {
		return fmt.Errorf("%s", locale.T("安全模式私钥无效，应为 32 字节 X25519 密钥的 base64"))
	}
	publicBytes, err := base64.StdEncoding.DecodeString(config.SecureModePublicKey)
	if err != nil || len(publicBytes) != 32 {
		return fmt.Errorf("%s", locale.T("安全模式公钥无效，应为 32 字节 X25519 密钥的 base64"))
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		return fmt.Errorf("%s", locale.T("安全模式私钥无效，应为 32 字节 X25519 密钥的 base64"))
	}
	if !privateKey.PublicKey().Equal(mustPublicKey(publicBytes)) {
		return fmt.Errorf("%s", locale.T("安全模式公钥与私钥不匹配"))
	}
	return nil
}

func mustPublicKey(data []byte) *ecdh.PublicKey {
	key, err := ecdh.X25519().NewPublicKey(data)
	if err != nil {
		return nil
	}
	return key
}

// validSocks5Hostname reports whether host can name a machine: letters,
// digits, dots and dashes, no spaces or slashes.
func validSocks5Hostname(host string) bool {
	if strings.TrimSpace(host) != host || host == "" {
		return false
	}
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// parseProxyNetwork reads one proxy network as the command line spells it:
// "10.0.0.0/24", or "10.0.0.0/24->192.168.0.0/24" to present it under a
// different address.
func parseProxyNetwork(network string) (cidr, mapped string, err error) {
	cidr, mapped, _ = strings.Cut(strings.TrimSpace(network), "->")
	if _, err = netip.ParsePrefix(cidr); err != nil {
		return "", "", fmt.Errorf(locale.T("代理网段 %q 无效，例如 192.168.1.0/24"), network)
	}
	if mapped != "" {
		if _, err = netip.ParsePrefix(mapped); err != nil {
			return "", "", fmt.Errorf(locale.T("代理网段 %q 的映射地址无效"), network)
		}
	}
	return cidr, mapped, nil
}
