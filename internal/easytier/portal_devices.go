package easytier

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/netip"
	"os/exec"
	"regexp"
	"strings"

	"github.com/skip2/go-qrcode"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

type PortalDevice struct {
	Name      string   `json:"name" toml:"name"`
	VirtualIP string   `json:"virtual_ip" toml:"virtual_ip"`
	Groups    []string `json:"groups" toml:"groups"`
}
type PortalDeviceInfo struct {
	PortalDevice
	State        int    `json:"state"`
	ClientConfig string `json:"client_config"`
	Endpoint     string `json:"endpoint"`
	Error        string `json:"error"`
}

var deviceName = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func (device PortalDevice) Validate() error {
	if !deviceName.MatchString(device.Name) {
		return fmt.Errorf("%s", locale.T("设备名须为 1-63 位字母、数字或连字符"))
	}
	prefix, err := netip.ParsePrefix(device.VirtualIP)
	if err != nil || !prefix.Addr().Is4() {
		return fmt.Errorf("%s", locale.T("设备地址必须是 IPv4/CIDR"))
	}
	for _, group := range device.Groups {
		if strings.TrimSpace(group) == "" {
			return fmt.Errorf("%s", locale.T("组名不能为空"))
		}
	}
	return nil
}
func DevicePortalSupported(version string) bool {
	version = strings.TrimPrefix(strings.TrimSpace(version), "easytier-core ")
	var major, minor int
	if _, err := fmt.Sscanf(version, "%d.%d.", &major, &minor); err != nil {
		return false
	}
	return major > 2 || major == 2 && minor >= 7
}
func EnsurePortalKey(portal *VPNPortal) error {
	if portal == nil || !portal.DeviceMode || portal.PrivateKey != "" {
		return nil
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	portal.PrivateKey = base64.StdEncoding.EncodeToString(key.Bytes())
	return nil
}

func checkPortalVersion(ctx context.Context, core string, portal *VPNPortal) error {
	if portal == nil {
		return nil
	}
	output, err := exec.CommandContext(ctx, core, "--version").Output()
	if err != nil {
		return err
	}
	modern := DevicePortalSupported(string(output))
	if portal.DeviceMode != modern {
		if modern {
			return fmt.Errorf("%s", locale.T("当前 Core 使用逐设备 Portal schema，请在 VPN Portal 页选择设备模式"))
		}
		return fmt.Errorf("%s", locale.T("当前 Core 不支持逐设备 Portal，需要 v2.7/main"))
	}
	return nil
}
func PatchPortalDevice(ctx context.Context, address string, device PortalDevice, remove bool) error {
	return patchPortalDevice(ctx, address, "", device, remove)
}

// PatchRemotePortalDevice patches one instance behind a shared remote
// management endpoint.
func PatchRemotePortalDevice(ctx context.Context, address string, network RemoteNetwork, device PortalDevice, remove bool) error {
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return err
	}
	return patchPortalDevice(ctx, address, network.ID, device, remove)
}

func patchPortalDevice(ctx context.Context, address, instanceID string, device PortalDevice, remove bool) error {
	// Capability probe uses a field absent from the stable response. A version
	// guess alone cannot authorize mutation against an unrelated endpoint.
	info, err := readVPNPortalNative(ctx, address, instanceID)
	if err != nil {
		return err
	}
	if info.Listener == "" {
		return fmt.Errorf("%s", locale.T("远端 Core 未提供逐设备 Portal 能力"))
	}
	if !remove {
		if err := device.Validate(); err != nil {
			return err
		}
	} else if !deviceName.MatchString(device.Name) {
		return fmt.Errorf("%s", locale.T("设备名无效"))
	}
	value := wireBytes(nil, 1, []byte(device.Name))
	value = wireBytes(value, 2, []byte(device.VirtualIP))
	for _, group := range device.Groups {
		value = wireBytes(value, 3, []byte(group))
	}
	var action uint64
	if remove {
		action = 1
	}
	patch := wireInt(nil, 1, action)
	patch = wireBytes(patch, 2, value)
	// PatchConfigRequest{patch = 1, instance = 2}
	request := wireBytes(nil, 1, wireBytes(nil, 15, patch))
	selector, err := instanceSelector(instanceID)
	if err != nil {
		return err
	}
	if selector != nil {
		request = wireBytes(request, 2, selector)
	}
	_, err = managementRPC(ctx, address, "ConfigRpc", 1, request)
	return err
}
func ReadVPNPortalNative(ctx context.Context, address string) (PortalInfo, error) {
	return readVPNPortalNative(ctx, address, "")
}

// ReadRemoteVPNPortal reads the Portal of one instance behind a shared
// remote management endpoint.
func ReadRemoteVPNPortal(ctx context.Context, address string, network RemoteNetwork) (PortalInfo, error) {
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return PortalInfo{}, err
	}
	return readVPNPortalNative(ctx, address, network.ID)
}

func readVPNPortalNative(ctx context.Context, address, instanceID string) (PortalInfo, error) {
	// GetVpnPortalInfoRequest.instance
	selector, err := instanceSelector(instanceID)
	if err != nil {
		return PortalInfo{}, err
	}
	var request []byte
	if selector != nil {
		request = wireBytes(nil, 1, selector)
	}
	data, err := managementRPC(ctx, address, "VpnPortalRpc", 1, request)
	if err != nil {
		return PortalInfo{}, err
	}
	fields, err := wireFields(data)
	if err != nil {
		return PortalInfo{}, err
	}
	var info PortalInfo
	for _, outer := range fields {
		if outer.number != 1 {
			continue
		}
		values, err := wireFields(outer.bytes)
		if err != nil {
			return info, err
		}
		for _, field := range values {
			switch field.number {
			case 1:
				info.Type = string(field.bytes)
			case 2:
				info.ClientConfig = string(field.bytes)
			case 3:
				info.ConnectedClients = append(info.ConnectedClients, string(field.bytes))
			case 5:
				info.Listener = string(field.bytes)
			case 4:
				values, err := wireFields(field.bytes)
				if err != nil {
					return info, err
				}
				var device PortalDeviceInfo
				for _, v := range values {
					switch v.number {
					case 1:
						device.Name = string(v.bytes)
					case 2:
						device.VirtualIP = string(v.bytes)
					case 3:
						device.Groups = append(device.Groups, string(v.bytes))
					case 4:
						device.State = int(v.value)
					case 6:
						device.Endpoint = string(v.bytes)
					case 8:
						device.ClientConfig = string(v.bytes)
					case 9:
						device.Error = string(v.bytes)
					}
				}
				info.Clients = append(info.Clients, device)
			}
		}
	}
	return info, nil
}
func PortalQRCode(config string) ([]byte, error) {
	// QR is generated locally; never upload the private key to a QR service.
	return qrcode.Encode(config, qrcode.Medium, 320)
}
