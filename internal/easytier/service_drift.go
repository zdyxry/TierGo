package easytier

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// coreVersionMarkerName is the marker file inside the service's config
// directory recording which embedded core the service was installed with.
// corebin.Ensure replaces the core binary in place when the client upgrades,
// so the service silently runs a newer core after its next restart; the
// marker lets the window notice and offer to re-sync the service config.
const coreVersionMarkerName = "core-version"

// writeCoreVersionMarker records the core version at install time. An
// external core (no embedded version) writes nothing: there is no drift to
// detect when the client does not own the binary.
func writeCoreVersionMarker(dir, version string) error {
	if version == "" {
		return nil
	}
	return writeFileDurable(filepath.Join(dir, coreVersionMarkerName), []byte(version+"\n"), 0o600)
}

// coreVersionDrift compares the marker in dir with the current version.
// A missing or empty marker, or an unknown current version, is not drift.
func coreVersionDrift(dir, current string) (installed string, drift bool, err error) {
	data, err := os.ReadFile(filepath.Join(dir, coreVersionMarkerName))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	installed = strings.TrimSpace(string(data))
	if installed == "" || current == "" {
		return installed, false, nil
	}
	return installed, installed != current, nil
}

// ServiceCoreDrift compares the core version recorded when the service was
// installed with the embedded core this build carries. ok is false when
// there is nothing to compare (no marker, external core, unreadable state).
func ServiceCoreDrift(settings ServiceSettings) (installed, current string, ok bool) {
	root, err := InstanceRoot()
	if err != nil {
		return "", "", false
	}
	_, _, current, err = LocateBinaries(root)
	if err != nil || current == "" {
		return "", "", false
	}
	for _, dir := range serviceStateDirs(settings, root) {
		marked, drift, err := coreVersionDrift(dir, current)
		if err == nil && drift {
			return marked, current, true
		}
	}
	return "", current, false
}

// serviceStateDirs are the directories a service install may have written
// its marker into: the per-profile config directory and the Config Server
// mode one. With a configured directory both are the same entry.
func serviceStateDirs(settings ServiceSettings, root string) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, dir := range []string{
		settings.ConfigDir,
		filepath.Join(root, "service-configs"),
		filepath.Join(root, "central-configs"),
	} {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	return dirs
}
