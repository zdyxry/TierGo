package easytier

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// SuggestDeviceIP returns the first host address of cidr (an IPv4 prefix such
// as "10.14.14.0/24") that no entry of used occupies, written back with the
// prefix length. used holds device addresses in the same "addr/bits" form;
// entries that fail to parse are ignored. The network and broadcast
// addresses are never suggested (except on /31 and /32, where every address
// is a host address).
func SuggestDeviceIP(cidr string, used []string) (string, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil || !prefix.Addr().Is4() {
		return "", fmt.Errorf(locale.T("客户端网段 %q 无效，应为 IPv4/CIDR"), cidr)
	}
	prefix = prefix.Masked()
	taken := map[netip.Addr]bool{}
	for _, entry := range used {
		addr, _, _ := strings.Cut(strings.TrimSpace(entry), "/")
		if parsed, err := netip.ParseAddr(addr); err == nil {
			taken[parsed] = true
		}
	}
	bits := prefix.Bits()
	first, last := prefix.Addr(), prefixBroadcast(prefix)
	for addr := first; addr.Compare(last) <= 0; addr = addr.Next() {
		if bits <= 30 && (addr == first || addr == last) {
			continue
		}
		if !taken[addr] {
			return fmt.Sprintf("%s/%d", addr, bits), nil
		}
	}
	return "", fmt.Errorf(locale.T("网段 %s 内没有可分配的设备地址"), prefix)
}

func prefixBroadcast(prefix netip.Prefix) netip.Addr {
	addr := prefix.Addr().As4()
	shift := 32 - prefix.Bits()
	value := uint32(addr[0])<<24 | uint32(addr[1])<<16 | uint32(addr[2])<<8 | uint32(addr[3])
	value |= (uint32(1) << shift) - 1
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
}

// SuggestPortalEndpoints derives candidate public WireGuard endpoints from
// the instance's listener URLs (listeners and mapped_listeners), replacing
// their ports with the portal's listen port. Wildcard (0.0.0.0/::), loopback
// and empty hosts are never endpoints; the order of the sources is kept and
// duplicates collapse.
func SuggestPortalEndpoints(listeners []string, portalListen string) []string {
	port, err := portOf(portalListen)
	if err != nil || port <= 0 {
		return nil
	}
	seen := map[string]bool{}
	var endpoints []string
	for _, listener := range listeners {
		host := listenerHost(listener)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		endpoints = append(endpoints, net.JoinHostPort(host, fmt.Sprint(port)))
	}
	return endpoints
}

// listenerHost reads the host of a listener URL such as "tcp://1.2.3.4:11010"
// or a bare "1.2.3.4:11010"; wildcard and loopback hosts return "".
func listenerHost(listener string) string {
	listener = strings.TrimSpace(listener)
	if _, rest, found := strings.Cut(listener, "://"); found {
		listener = rest
	}
	listener = strings.TrimSuffix(listener, "/")
	host, _, err := net.SplitHostPort(listener)
	if err != nil {
		return ""
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return ""
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.IsUnspecified() || addr.IsLoopback() {
			return ""
		}
	}
	return host
}
