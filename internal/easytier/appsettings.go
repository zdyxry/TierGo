package easytier

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AppSettings is what the window itself remembers, independent of any
// network profile.
type AppSettings struct {
	// OpenAtLogin starts the client when the user logs in.
	OpenAtLogin bool          `json:"open_at_login"`
	RemoteRPC   string        `json:"remote_rpc,omitempty"`
	Central     CentralConfig `json:"central,omitempty"`
	Language    string        `json:"language,omitempty"`
	// Service are the install-time options of the system service.
	Service ServiceSettings `json:"service,omitempty"`
	// SelectedProfile is the network the window was last showing.
	SelectedProfile string `json:"selected_profile,omitempty"`
	// EnabledNetworks are the profiles the user had connected; the next
	// launch reconnects them.
	EnabledNetworks []string `json:"enabled_networks,omitempty"`
}

// appSettingsPath is <instance root>/app.json.
func appSettingsPath() (string, error) {
	root, err := InstanceRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "app.json"), nil
}

// LoadAppSettings reads the window's own settings; nothing saved yet is not
// an error.
func LoadAppSettings() (AppSettings, error) {
	path, err := appSettingsPath()
	if err != nil {
		return AppSettings{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return AppSettings{}, nil
	}
	if err != nil {
		return AppSettings{}, fmt.Errorf("read %s: %w", path, err)
	}
	var settings AppSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return AppSettings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

func (settings AppSettings) Save() error {
	path, err := appSettingsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode app settings: %w", err)
	}
	if err := writeFileDurable(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
