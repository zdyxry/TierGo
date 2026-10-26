package easytier

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// VPNPortal matches the stable v2.6.4 schema. Development main replaces
// client_cidr with individually managed devices; do not conflate the two.
type VPNPortal struct {
	ClientCIDR string `json:"client_cidr,omitempty" toml:"client_cidr,omitempty"`
	Listen     string `json:"wireguard_listen" toml:"wireguard_listen"`
	// DeviceMode selects the incompatible v2.7/main schema explicitly.
	DeviceMode bool           `json:"device_mode,omitempty" toml:"-"`
	PrivateKey string         `json:"wireguard_private_key,omitempty" toml:"wireguard_private_key,omitempty"`
	Clients    []PortalDevice `json:"clients,omitempty" toml:"clients,omitempty"`
}

func (p *VPNPortal) Validate() error {
	if p == nil {
		return nil
	}
	if !p.DeviceMode {
		prefix, err := netip.ParsePrefix(p.ClientCIDR)
		if err != nil || !prefix.Addr().Is4() {
			return fmt.Errorf("%s", locale.T("VPN Portal 客户端网段必须是 IPv4 CIDR"))
		}
	} else {
		seen := map[string]bool{}
		for _, client := range p.Clients {
			if err := client.Validate(); err != nil {
				return err
			}
			if seen[client.Name] {
				return fmt.Errorf("%s", locale.T("重复设备名"))
			}
			seen[client.Name] = true
		}
	}
	addr, err := netip.ParseAddrPort(p.Listen)
	if err != nil || addr.Port() == 0 {
		return fmt.Errorf("%s", locale.T("VPN Portal 监听格式应为 0.0.0.0:51820"))
	}
	return nil
}

type PortalInfo struct {
	Type             string             `json:"vpn_type"`
	ClientConfig     string             `json:"client_config"`
	ConnectedClients []string           `json:"connected_clients"`
	Listener         string             `json:"listener"`
	Clients          []PortalDeviceInfo `json:"clients"`
}

func ReadVPNPortal(ctx context.Context, address string) (PortalInfo, error) {
	root, err := InstanceRoot()
	if err != nil {
		return PortalInfo{}, err
	}
	_, cli, _, err := LocateBinaries(root)
	if err != nil {
		return PortalInfo{}, err
	}
	var info PortalInfo
	err = cliQuery(ctx, cli, address, "vpn-portal", &info)
	return info, err
}

// instanceSelector scopes an instance-level RPC request to one instance of a
// shared management portal (InstanceIdentifier.id); empty means the portal's
// default instance, which is what a single-instance local core serves.
func instanceSelector(instanceID string) ([]byte, error) {
	if instanceID == "" {
		return nil, nil
	}
	uuid, err := encodeUUID(instanceID)
	if err != nil {
		return nil, err
	}
	return wireBytes(nil, 1, uuid), nil
}

// SetPortalEndpoint replaces only Endpoint in the generated client config,
// leaving addresses, keys and allowed routes untouched. Core's stable
// template contains a wildcard bind address that isn't reachable remotely.
func SetPortalEndpoint(config, address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return "", fmt.Errorf("%s", locale.T("Endpoint 应为 主机:端口"))
	}
	lines := strings.Split(config, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Endpoint =") {
			lines[i] = "Endpoint = " + address
		}
	}
	return strings.Join(lines, "\n"), nil
}
