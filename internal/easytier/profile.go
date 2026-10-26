package easytier

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// Profile is one network the user configured: a display name and the
// configuration the core runs with. Profiles are identified by a UUID, like
// the official GUI's network instances, so renaming one never loses it, and
// several profiles may run at the same time.
type Profile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Config Config `json:"config"`
}

// newProfileID makes a random UUID (version 4), without taking a dependency
// for sixteen bytes.
func newProfileID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand only fails when the system is out of entropy sources;
		// fall back to time and process noise rather than no identifier.
		return fmt.Sprintf("fallback-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// profilesDir is <instance root>/networks: one JSON file per profile.
func profilesDir() (string, error) {
	root, err := InstanceRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "networks"), nil
}

// profilePath is where one profile lives.
func safeInstanceKey(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	for _, char := range id {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

func profilePath(dir, id string) string {
	return filepath.Join(dir, id+".json")
}

// NewProfile makes a profile with the default configuration.
func NewProfile(name string) Profile {
	name = strings.TrimSpace(name)
	if name == "" {
		name = locale.T("未命名网络")
	}
	return Profile{ID: newProfileID(), Name: name, Config: DefaultConfig()}
}

// LoadProfiles lists every saved profile, sorted by name. The first run
// after the single-network client migrates its config.json into a profile
// called 默认网络, so nothing the user configured is lost.
func LoadProfiles() ([]Profile, error) {
	dir, err := profilesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return migrateLegacyConfig(dir)
	}
	if err != nil {
		return nil, fmt.Errorf(locale.T("读取网络列表失败：%w"), err)
	}
	var profiles []Profile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		profile, err := readProfile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if len(profiles) == 0 {
		return migrateLegacyConfig(dir)
	}
	sortProfiles(profiles)
	return profiles, nil
}

// migrateLegacyConfig turns the single-network config.json into the first
// profile. The old file is left in place: it no longer drives anything, and
// removing it would make a downgrade lose the network.
func migrateLegacyConfig(dir string) ([]Profile, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	profile := NewProfile(locale.T("默认网络"))
	profile.Config = config
	if err := SaveProfile(profile); err != nil {
		return nil, err
	}
	return []Profile{profile}, nil
}

func readProfile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf(locale.T("读取网络配置 %s 失败：%w"), path, err)
	}
	// Fields absent from the file keep the defaults, so an old profile gains
	// the new options (multi-thread, IPv6, …) as the client had them on.
	profile := Profile{Config: DefaultConfig()}
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, fmt.Errorf(locale.T("解析网络配置 %s 失败：%w"), path, err)
	}
	if !safeInstanceKey(profile.ID) || filepath.Base(path) != profile.ID+".json" {
		return Profile{}, fmt.Errorf(locale.T("网络配置 %s 的 id 无效或与文件名不符"), path)
	}
	if !validLogLevel(profile.Config.LogLevel) {
		profile.Config.LogLevel = "error"
	}
	return profile, nil
}

func sortProfiles(profiles []Profile) {
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
}

// SaveProfile writes one profile durably.
func SaveProfile(profile Profile) error {
	if !safeInstanceKey(profile.ID) {
		return errors.New(locale.T("网络 id 无效"))
	}
	dir, err := profilesDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf(locale.T("编码网络配置失败：%w"), err)
	}
	if err := writeFileDurable(profilePath(dir, profile.ID), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf(locale.T("写入网络配置失败：%w"), err)
	}
	return nil
}

// DeleteProfile removes one profile's file. The caller stops its instance
// first; the file is gone even if a later save would have failed.
func DeleteProfile(id string) error {
	if !safeInstanceKey(id) {
		return errors.New(locale.T("网络 id 无效"))
	}
	dir, err := profilesDir()
	if err != nil {
		return err
	}
	if err := os.Remove(profilePath(dir, id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf(locale.T("删除网络配置失败：%w"), err)
	}
	return nil
}

// instanceName is the name a core runs under, unique per profile so the
// CLI can address one instance of several.
func instanceName(key string) string {
	return "mygo-" + key
}

// InstanceName is the name this profile's core runs under.
func (profile Profile) InstanceName() string {
	return instanceName(profile.ID)
}

// DisplayName renders the profile name in the current language. The default
// and fallback names are created through locale.T, so a profile made under
// one language keeps that language's word stored; map the known sentinels
// at display time instead of rewriting what was saved.
func (profile Profile) DisplayName() string {
	switch profile.Name {
	case "默认网络", locale.In("en", "默认网络"):
		return locale.T("默认网络")
	case "未命名网络", locale.In("en", "未命名网络"):
		return locale.T("未命名网络")
	}
	return profile.Name
}
