// Package corebin carries the official EasyTier binaries inside the
// application, so a release is one file.
//
// The binaries are not in the repository: `cmd/fetch-core` downloads the
// official release for one platform, compresses what the client needs and
// writes it under assets/. A build made afterwards embeds it. A build without
// it still works and looks for easytier-core on the machine instead.
package corebin

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zdyxry/easytier-mygo/internal/locale"
	"github.com/zdyxry/easytier-mygo/internal/owner"
)

const (
	// CoreName and CLIName are the two programs the client runs.
	CoreName = "easytier-core"
	CLIName  = "easytier-cli"

	provenanceName = "provenance.json"
)

// Platform is the directory assets are kept under, as "<goos>-<goarch>".
func Platform() string { return runtime.GOOS + "-" + runtime.GOARCH }

// Binary is where the embedded programs were written.
type Binary struct {
	Core    string
	CLI     string
	Dir     string
	Version string
}

// Provenance records what was embedded and where it came from.
type Provenance struct {
	Version string `json:"version"`
	Asset   string `json:"asset"`
	SHA256  string `json:"sha256"`
	Fetched string `json:"fetched"`
}

func assetDir() string { return path.Join("assets", Platform()) }

// Available reports whether this build carries the core for its platform.
func Available() bool {
	_, err := fs.Stat(assets, path.Join(assetDir(), CoreName+".gz"))
	return err == nil
}

// EmbeddedProvenance is what this build carries, if anything.
func EmbeddedProvenance() (Provenance, bool) {
	data, err := fs.ReadFile(assets, path.Join(assetDir(), provenanceName))
	if err != nil {
		return Provenance{}, false
	}
	var provenance Provenance
	if err := json.Unmarshal(data, &provenance); err != nil {
		return Provenance{}, false
	}
	return provenance, true
}

// Ensure writes the embedded binaries into dir and returns their paths.
//
// The write happens once per embedded version: the directory records what it
// holds, and a marker that matches is trusted. A marker that does not match
// is replaced, so upgrading the client replaces the core with it.
func Ensure(dir string) (Binary, error) {
	if !Available() {
		return Binary{}, errors.New(locale.T("这个构建没有内嵌 easytier-core"))
	}
	target := filepath.Join(dir, Platform())
	provenance, _ := EmbeddedProvenance()

	if marker, err := readProvenance(filepath.Join(target, provenanceName)); err == nil {
		if marker.SHA256 == provenance.SHA256 && marker.Version == provenance.Version {
			binary, err := paths(target, provenance)
			if err == nil {
				return binary, nil
			}
		}
	}

	entries, err := fs.ReadDir(assets, assetDir())
	if err != nil {
		return Binary{}, err
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return Binary{}, err
	}
	owner.Keep(filepath.Dir(target), target)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".gz") {
			continue
		}
		source, err := assets.Open(path.Join(assetDir(), name))
		if err != nil {
			return Binary{}, err
		}
		err = extractOne(source, filepath.Join(target, strings.TrimSuffix(name, ".gz")))
		source.Close()
		if err != nil {
			return Binary{}, err
		}
	}
	if err := writeProvenance(filepath.Join(target, provenanceName), provenance); err != nil {
		return Binary{}, err
	}
	return paths(target, provenance)
}

func paths(dir string, provenance Provenance) (Binary, error) {
	binary := Binary{
		Core:    filepath.Join(dir, executableName(CoreName)),
		CLI:     filepath.Join(dir, executableName(CLIName)),
		Dir:     dir,
		Version: provenance.Version,
	}
	for _, name := range []string{binary.Core, binary.CLI} {
		if _, err := os.Stat(name); err != nil {
			return Binary{}, fmt.Errorf(locale.T("内嵌的内核文件缺失：%w"), err)
		}
	}
	return binary, nil
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// extractOne unpacks one compressed program and makes it executable.
func extractOne(source io.Reader, target string) error {
	reader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()

	temporary := target + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, reader); err != nil {
		file.Close()
		os.Remove(temporary)
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(temporary)
		return err
	}
	if err := os.Chmod(temporary, executableMode(target)); err != nil {
		os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, target)
}

// executableMode marks programs executable and data files, such as the
// Windows driver, not.
func executableMode(name string) os.FileMode {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".dll", ".sys", ".pdb":
		return 0o600
	default:
		return 0o700
	}
}

func readProvenance(path string) (Provenance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Provenance{}, err
	}
	var provenance Provenance
	if err := json.Unmarshal(data, &provenance); err != nil {
		return Provenance{}, err
	}
	return provenance, nil
}

func writeProvenance(path string, provenance Provenance) error {
	if provenance.Fetched == "" {
		provenance.Fetched = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
