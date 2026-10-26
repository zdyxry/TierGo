// Command fetch-core puts the official EasyTier binaries in the place the
// build embeds them from.
//
//	go run ./cmd/fetch-core
//	go run ./cmd/fetch-core -platform windows/amd64
//
// Run it before `go tool mygo build`: the application then carries the core
// and needs nothing installed on the user's machine. A build made without it
// still works and looks for easytier-core on the machine instead.
package main

import (
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// The release layout of EasyTier: one zip per platform, holding a directory
// with the programs.
const (
	releaseBase      = "https://github.com/EasyTier/EasyTier/releases/download"
	latestReleaseURL = "https://api.github.com/repos/EasyTier/EasyTier/releases/latest"
	maxArchiveSize   = 256 << 20
	maxLatestSize    = 1 << 20
)

var httpClient = &http.Client{Timeout: 2 * time.Minute}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fetch-core:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		version  = flag.String("version", "", "release tag; empty uses core-lock.json, or latest")
		platform = flag.String("platform", runtime.GOOS+"/"+runtime.GOARCH, "GOOS/GOARCH to fetch for")
		out      = flag.String("out", filepath.Join("internal", "corebin", "assets"), "where to write the assets")
		lockPath = flag.String("lock", "core-lock.json", "release lock file; empty disables locking")
		pin      = flag.String("sha256", "", "expected SHA-256 when no lock entry matches")
		clean    = flag.Bool("clean", false, "remove the assets of other platforms")
	)
	flag.Parse()

	goos, goarch, err := splitPlatform(*platform)
	if err != nil {
		return err
	}
	asset, err := assetName(goos, goarch)
	if err != nil {
		return err
	}

	release, name, expected, err := resolveRelease(*version, *platform, asset, *lockPath, *pin)
	if err != nil {
		return err
	}
	if *version == "latest" {
		fmt.Printf("latest release is %s; pin it with -version %s\n", release, release)
	}
	url := fmt.Sprintf("%s/%s/%s", releaseBase, release, name)

	fmt.Printf("downloading %s\n", url)
	archive, digest, err := download(url)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := checkDigest(expected, digest); err != nil {
		return err
	}

	// Platforms accumulate, because each one is embedded only by the build
	// that targets it: a universal macOS app takes both of its halves, and a
	// Linux build ignores them both.
	platformDir := goos + "-" + goarch
	written, err := writeRelease(archive, *out, goos, goarch, provenance{
		Version: release,
		Asset:   name,
		SHA256:  digest,
	})
	if err != nil {
		return err
	}
	if *clean {
		if err := clearOtherPlatforms(*out, platformDir); err != nil {
			return err
		}
	}

	fmt.Printf("wrote %s for %s/%s:\n", filepath.Join(*out, platformDir), goos, goarch)
	for _, file := range written {
		fmt.Printf("  %s\n", file)
	}
	fmt.Println("now build the application: the core is embedded in it")
	return nil
}

type coreLock struct {
	Version   string                 `json:"version"`
	Platforms map[string]lockedAsset `json:"platforms"`
}

type lockedAsset struct {
	Asset  string `json:"asset"`
	SHA256 string `json:"sha256"`
}

type provenance struct {
	Version string `json:"version"`
	Asset   string `json:"asset"`
	SHA256  string `json:"sha256"`
}

func resolveRelease(version, platform, asset, lockPath, pin string) (string, string, string, error) {
	var lock coreLock
	if lockPath != "" {
		data, err := os.ReadFile(lockPath)
		if err != nil {
			return "", "", "", fmt.Errorf("读取锁文件：%w", err)
		}
		if err := json.Unmarshal(data, &lock); err != nil {
			return "", "", "", fmt.Errorf("解析锁文件：%w", err)
		}
	} else if version == "" {
		return "", "", "", errors.New("禁用 lock 时必须显式提供 -version 和 -sha256")
	}

	release := version
	if release == "" {
		release = lock.Version
		if release == "" {
			return "", "", "", errors.New("锁文件没有 version")
		}
	}
	if release == "latest" {
		var err error
		release, err = latestRelease()
		if err != nil {
			return "", "", "", err
		}
	}

	locked, matches := lock.Platforms[platform]
	matches = matches && lock.Version == release
	if matches {
		if locked.Asset == "" || locked.SHA256 == "" {
			return "", "", "", fmt.Errorf("锁文件中 %s %s 的 asset 或 sha256 为空", platform, release)
		}
		if pin != "" && !strings.EqualFold(pin, locked.SHA256) {
			return "", "", "", errors.New("-sha256 与锁文件中的 SHA-256 不一致")
		}
		return release, locked.Asset, locked.SHA256, nil
	}
	if pin == "" {
		return "", "", "", fmt.Errorf("锁文件没有 %s %s 的记录；必须显式提供 -sha256", platform, release)
	}
	return release, fmt.Sprintf("easytier-%s-%s.zip", asset, release), pin, nil
}

func checkDigest(expected, actual string) error {
	if expected == "" {
		return errors.New("必须提供 SHA-256 校验值")
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("校验和不匹配：期望 %s，实际 %s", expected, actual)
	}
	return nil
}

func splitPlatform(value string) (string, string, error) {
	goos, goarch, found := strings.Cut(value, "/")
	if !found || goos == "" || goarch == "" {
		return "", "", fmt.Errorf("平台应写作 GOOS/GOARCH，例如 linux/amd64，收到 %q", value)
	}
	return goos, goarch, nil
}

// assetName maps a Go platform to the name EasyTier uses in its releases.
func assetName(goos, goarch string) (string, error) {
	names := map[string]string{
		"linux/amd64":   "linux-x86_64",
		"linux/arm64":   "linux-aarch64",
		"linux/riscv64": "linux-riscv64",
		"darwin/amd64":  "macos-x86_64",
		"darwin/arm64":  "macos-aarch64",
		"windows/amd64": "windows-x86_64",
		"windows/arm64": "windows-arm64",
		"windows/386":   "windows-i686",
	}
	asset, ok := names[goos+"/"+goarch]
	if !ok {
		return "", fmt.Errorf("EasyTier 没有为 %s/%s 发布二进制", goos, goarch)
	}
	return asset, nil
}

func latestRelease() (string, error) {
	return latestReleaseFrom(httpClient, latestReleaseURL, maxLatestSize)
}

func latestReleaseFrom(client *http.Client, url string, limit int64) (string, error) {
	response, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("查询最新版本失败：%s", response.Status)
	}
	data, err := readAtMost(response.Body, limit)
	if err != nil {
		return "", fmt.Errorf("读取最新版本信息：%w", err)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", errors.New("最新版本没有 tag")
	}
	return release.TagName, nil
}

func readAtMost(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("内容超过 %d 字节上限", limit)
	}
	return data, nil
}

// download saves the release zip to a temporary file and returns its SHA-256.
func download(url string) (string, string, error) {
	return downloadWithClient(httpClient, url, maxArchiveSize)
}

func downloadWithClient(client *http.Client, url string, limit int64) (string, string, error) {
	response, err := client.Get(url)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("下载失败：%s", response.Status)
	}
	if response.ContentLength > limit {
		return "", "", fmt.Errorf("下载内容超过 %d 字节上限", limit)
	}
	file, err := os.CreateTemp("", "easytier-*.zip")
	if err != nil {
		return "", "", err
	}
	complete := false
	defer func() {
		if !complete {
			file.Close()
			os.Remove(file.Name())
		}
	}()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, limit+1))
	if err != nil {
		return "", "", err
	}
	if written > limit {
		return "", "", fmt.Errorf("下载内容超过 %d 字节上限", limit)
	}
	if err := file.Close(); err != nil {
		return "", "", err
	}
	complete = true
	return file.Name(), hex.EncodeToString(hash.Sum(nil)), nil
}

func writeRelease(archive, out, goos, goarch string, source provenance) ([]string, error) {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	platformDir := goos + "-" + goarch
	staging, err := os.MkdirTemp(out, "."+platformDir+"-staging-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)

	written, err := unpack(archive, staging, goos)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(staging, "provenance.json"), append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	target := filepath.Join(out, platformDir)
	if placeholder, err := os.ReadFile(filepath.Join(target, "PLACEHOLDER.md")); err == nil {
		if err := os.WriteFile(filepath.Join(staging, "PLACEHOLDER.md"), placeholder, 0o644); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := publishDir(staging, target); err != nil {
		return nil, err
	}
	return written, nil
}

func publishDir(staging, target string) error {
	if _, err := os.Stat(target); err != nil {
		if os.IsNotExist(err) {
			return os.Rename(staging, target)
		}
		return err
	}

	backup, err := os.MkdirTemp(filepath.Dir(target), "."+filepath.Base(target)+"-backup-")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(staging, target); err != nil {
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			return fmt.Errorf("发布失败：%w；恢复旧目录失败：%v", err, rollbackErr)
		}
		return err
	}
	return os.RemoveAll(backup)
}

func clearOtherPlatforms(root, keep string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == keep {
			continue
		}
		platform := filepath.Join(root, entry.Name())
		files, err := os.ReadDir(platform)
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.Name() == "PLACEHOLDER.md" {
				continue
			}
			if err := os.RemoveAll(filepath.Join(platform, file.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// unpack compresses the wanted programs out of the release zip.
func unpack(archive, dir, goos string) ([]string, error) {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	files := make(map[string]*zip.File)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name, ok := canonicalName(strippedName(file.Name), goos)
		if !ok {
			continue
		}
		if files[name] != nil {
			return nil, fmt.Errorf("压缩包里重复出现 %s", name)
		}
		files[name] = file
	}
	required := requiredFiles(goos)
	for _, name := range required {
		if files[name] == nil {
			return nil, fmt.Errorf("压缩包缺少必需文件 %s", name)
		}
	}

	var written []string
	for _, name := range required {
		if err := writeCompressed(files[name], filepath.Join(dir, name+".gz")); err != nil {
			return nil, err
		}
		written = append(written, name+".gz")
	}
	sort.Strings(written)
	return written, nil
}

func requiredFiles(goos string) []string {
	required := []string{"easytier-core", "easytier-cli"}
	if goos == "windows" {
		required = append(required, "wintun.dll", "WinDivert64.sys")
	}
	return required
}

// strippedName drops the release's platform directory and any leading paths.
func strippedName(name string) string {
	name = path.Clean(name)
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	return name
}

func canonicalName(name, goos string) (string, bool) {
	lower := strings.ToLower(name)
	for _, candidate := range []string{"easytier-core", "easytier-cli"} {
		if lower == candidate || lower == candidate+".exe" {
			return candidate, true
		}
	}
	if goos == "windows" {
		for _, candidate := range []string{"wintun.dll", "WinDivert64.sys"} {
			if strings.EqualFold(name, candidate) {
				return candidate, true
			}
		}
	}
	return "", false
}

func wantedFile(name, goos string) bool {
	_, ok := canonicalName(name, goos)
	return ok
}

func writeCompressed(file *zip.File, target string) error {
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	out, err := os.Create(target)
	if err != nil {
		return err
	}
	compressor, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		out.Close()
		return err
	}
	if _, err := io.Copy(compressor, source); err != nil {
		compressor.Close()
		out.Close()
		return err
	}
	if err := compressor.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
