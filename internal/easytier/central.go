package easytier

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// CentralConfig belongs to the application, not to a network: one Core
// daemon connects to Config Server and owns the server-managed instances.
type CentralConfig struct {
	Endpoint  string `json:"endpoint"`
	MachineID string `json:"machine_id"`
}

func (c CentralConfig) Validate() error {
	if strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("%s", locale.T("Config Server 地址不能为空"))
	}
	if strings.Contains(c.Endpoint, "://") {
		u, err := url.Parse(c.Endpoint)
		if err != nil || u.Host == "" || u.Path == "" || u.Path == "/" {
			return fmt.Errorf("%s", locale.T("Config Server 需要 协议://主机:端口/用户令牌"))
		}
		switch u.Scheme {
		case "tcp", "udp", "ws", "wss":
		default:
			return fmt.Errorf("%s", locale.T("Config Server 协议无效"))
		}
	}
	if _, err := encodeUUID(c.MachineID); err != nil {
		return fmt.Errorf(locale.T("Machine ID 必须是稳定 UUID：%w"), err)
	}
	return nil
}
func NewMachineID() string { return newProfileID() }

// Centralized arguments are separate from the per-network TOML: the empty
// config directory is populated by Core when the server pushes networks.
func (c CentralConfig) args(root string, configDir string) []string {
	if configDir == "" {
		configDir = filepath.Join(root, "central-configs")
	}
	return []string{"--config-server", c.Endpoint, "--machine-id", c.MachineID, "--config-dir", configDir}
}

// ServiceInstallCentral installs the same system service in Config Server
// mode. Unlike per-profile mode, this never writes local network TOMLs.
func ServiceInstallCentral(ctx context.Context, c CentralConfig, settings ServiceSettings) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	root, err := InstanceRoot()
	if err != nil {
		return err
	}
	configDir, err := centralConfigsDir(settings)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	return installService(ctx, c.args(root, settings.ConfigDir), root, configDir, settings)
}

func ServiceRPCAddress() (string, error) {
	root, err := InstanceRoot()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(root, "service-rpc"))
	if err != nil {
		return "", err
	}
	return NormalizeRPCAddress(strings.TrimSpace(string(data)))
}
