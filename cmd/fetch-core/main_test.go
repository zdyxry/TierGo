package main

import (
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetName(t *testing.T) {
	for _, testCase := range []struct {
		goos, goarch, want string
		fails              bool
	}{
		{goos: "linux", goarch: "amd64", want: "linux-x86_64"},
		{goos: "linux", goarch: "arm64", want: "linux-aarch64"},
		{goos: "darwin", goarch: "arm64", want: "macos-aarch64"},
		{goos: "darwin", goarch: "amd64", want: "macos-x86_64"},
		{goos: "windows", goarch: "amd64", want: "windows-x86_64"},
		{goos: "windows", goarch: "arm64", want: "windows-arm64"},
		{goos: "plan9", goarch: "amd64", fails: true},
	} {
		got, err := assetName(testCase.goos, testCase.goarch)
		if testCase.fails {
			if err == nil {
				t.Errorf("%s/%s was accepted as %q", testCase.goos, testCase.goarch, got)
			}
			continue
		}
		if err != nil || got != testCase.want {
			t.Errorf("assetName(%s, %s) = %q, %v; want %q",
				testCase.goos, testCase.goarch, got, err, testCase.want)
		}
	}
}

// Only the files a client needs are taken out of the release, and the
// Windows driver only from the Windows one.
func TestWantedFile(t *testing.T) {
	for _, testCase := range []struct {
		name, goos string
		want       bool
	}{
		{"easytier-core", "linux", true},
		{"easytier-cli", "windows", true},
		{"easytier-web", "linux", false},
		{"easytier-web-embed", "linux", false},
		{"wintun.dll", "windows", true},
		{"wintun.dll", "linux", false},
		{"WinDivert64.sys", "windows", true},
		{"WinDivert64.sys", "darwin", false},
		{"Packet.dll", "windows", false},
	} {
		if got := wantedFile(testCase.name, testCase.goos); got != testCase.want {
			t.Errorf("wantedFile(%q, %q) = %v; want %v", testCase.name, testCase.goos, got, testCase.want)
		}
	}
}

func TestStrippedName(t *testing.T) {
	if got := strippedName("easytier-linux-x86_64-v2.6.4/easytier-core"); got != "easytier-core" {
		t.Errorf("strippedName = %q", got)
	}
	if got := strippedName("easytier-core"); got != "easytier-core" {
		t.Errorf("strippedName = %q", got)
	}
}

// unpack writes the wanted programs, compressed, and leaves the rest.
func TestUnpack(t *testing.T) {
	archive := makeReleaseZip(t, map[string]string{
		"easytier-linux-x86_64/easytier-core":   "core-bytes",
		"easytier-linux-x86_64/easytier-cli":    "cli-bytes",
		"easytier-linux-x86_64/easytier-web":    "web-bytes",
		"easytier-linux-x86_64/easytier-cli-v2": "other",
	})
	dir := t.TempDir()
	written, err := unpack(archive, dir, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 {
		t.Fatalf("wrote %v", written)
	}
	for _, entry := range []struct{ file, want string }{
		{"easytier-core.gz", "core-bytes"},
		{"easytier-cli.gz", "cli-bytes"},
	} {
		if got := readCompressed(t, filepath.Join(dir, entry.file)); got != entry.want {
			t.Errorf("%s holds %q, want %q", entry.file, got, entry.want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "easytier-web.gz")); err == nil {
		t.Error("the web server was taken although a client does not need it")
	}
}

func TestUnpackNeedsThePrograms(t *testing.T) {
	archive := makeReleaseZip(t, map[string]string{"easytier-x/easytier-web": "web"})
	if _, err := unpack(archive, t.TempDir(), "linux"); err == nil {
		t.Fatal("an archive without the core was accepted")
	}
}

func makeReleaseZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func readCompressed(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestResolveReleaseUsesLockByDefault(t *testing.T) {
	lockPath := makeLockFile(t)
	release, asset, pin, err := resolveRelease("", "linux/amd64", "linux-x86_64", lockPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if release != "v2.6.4" || asset != "locked-linux.zip" || pin != strings.Repeat("a", 64) {
		t.Fatalf("resolveRelease() = %q, %q, %q", release, asset, pin)
	}
}

func TestResolveReleaseRequiresPinWithoutMatchingLock(t *testing.T) {
	lockPath := makeLockFile(t)
	for _, testCase := range []struct {
		name, version, platform, lockPath, pin string
	}{
		{name: "other version", version: "v2.7.0", platform: "linux/amd64", lockPath: lockPath},
		{name: "other platform", version: "v2.6.4", platform: "linux/arm64", lockPath: lockPath},
		{name: "lock disabled", version: "v2.6.4", platform: "linux/amd64", pin: ""},
		{name: "lock disabled and no version", platform: "linux/amd64", pin: strings.Repeat("b", 64)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, _, _, err := resolveRelease(testCase.version, testCase.platform, "linux-x86_64", testCase.lockPath, testCase.pin); err == nil {
				t.Fatal("release without a matching lock and pin was accepted")
			}
		})
	}

	release, asset, pin, err := resolveRelease("v2.7.0", "linux/amd64", "linux-x86_64", lockPath, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if release != "v2.7.0" || asset != "easytier-linux-x86_64-v2.7.0.zip" || pin != strings.Repeat("b", 64) {
		t.Fatalf("unlocked release = %q, %q, %q", release, asset, pin)
	}
}

func TestResolveReleaseDoesNotOverrideLock(t *testing.T) {
	if _, _, _, err := resolveRelease("v2.6.4", "linux/amd64", "linux-x86_64", makeLockFile(t), strings.Repeat("b", 64)); err == nil {
		t.Fatal("a pin conflicting with the lock was accepted")
	}
}

func TestCheckDigest(t *testing.T) {
	if err := checkDigest(strings.Repeat("A", 64), strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := checkDigest(strings.Repeat("a", 64), strings.Repeat("b", 64)); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
	if err := checkDigest("", strings.Repeat("a", 64)); err == nil {
		t.Fatal("empty checksum was accepted")
	}
}

func TestUnpackWindowsExecutablesAndDrivers(t *testing.T) {
	archive := makeReleaseZip(t, map[string]string{
		"easytier-windows-x86_64/easytier-core.exe": "core-bytes",
		"easytier-windows-x86_64/easytier-cli.exe":  "cli-bytes",
		"easytier-windows-x86_64/wintun.dll":        "wintun-bytes",
		"easytier-windows-x86_64/WinDivert64.sys":   "windivert-bytes",
		"easytier-windows-x86_64/Packet.dll":        "npcap-bytes",
	})
	dir := t.TempDir()
	written, err := unpack(archive, dir, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 4 {
		t.Fatalf("wrote %v", written)
	}
	for _, entry := range []struct{ file, want string }{
		{"easytier-core.gz", "core-bytes"},
		{"easytier-cli.gz", "cli-bytes"},
		{"wintun.dll.gz", "wintun-bytes"},
		{"WinDivert64.sys.gz", "windivert-bytes"},
	} {
		if got := readCompressed(t, filepath.Join(dir, entry.file)); got != entry.want {
			t.Errorf("%s holds %q, want %q", entry.file, got, entry.want)
		}
	}
	for _, excluded := range []string{"easytier-core.exe.gz", "easytier-cli.exe.gz", "Packet.dll.gz"} {
		if _, err := os.Stat(filepath.Join(dir, excluded)); !os.IsNotExist(err) {
			t.Errorf("%s was unexpectedly written", excluded)
		}
	}
}

func TestUnpackRejectsEveryMissingRequiredFile(t *testing.T) {
	for _, testCase := range []struct {
		name, goos string
		files      map[string]string
		missing    string
	}{
		{name: "core", goos: "linux", files: map[string]string{"easytier-cli": "cli"}, missing: "easytier-core"},
		{name: "cli", goos: "linux", files: map[string]string{"easytier-core": "core"}, missing: "easytier-cli"},
		{name: "wintun", goos: "windows", files: map[string]string{"easytier-core.exe": "core", "easytier-cli.exe": "cli", "WinDivert64.sys": "driver"}, missing: "wintun.dll"},
		{name: "windivert", goos: "windows", files: map[string]string{"easytier-core.exe": "core", "easytier-cli.exe": "cli", "wintun.dll": "driver"}, missing: "WinDivert64.sys"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := makeReleaseZip(t, testCase.files)
			if _, err := unpack(archive, dir, testCase.goos); err == nil || !strings.Contains(err.Error(), testCase.missing) {
				t.Fatalf("unpack error = %v, want missing %s", err, testCase.missing)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("incomplete unpack wrote %v", entries)
			}
		})
	}
}

func TestWriteReleasePublishesCompleteDirectory(t *testing.T) {
	out := t.TempDir()
	target := filepath.Join(out, "linux-amd64")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old"), []byte("keep on failure"), 0o644); err != nil {
		t.Fatal(err)
	}

	badArchive := makeReleaseZip(t, map[string]string{"easytier-core": "core"})
	if _, err := writeRelease(badArchive, out, "linux", "amd64", provenance{}); err == nil {
		t.Fatal("incomplete release was accepted")
	}
	if data, err := os.ReadFile(filepath.Join(target, "old")); err != nil || string(data) != "keep on failure" {
		t.Fatalf("old platform was changed after failure: %q, %v", data, err)
	}
	assertNoStaging(t, out)

	archive := makeReleaseZip(t, map[string]string{"easytier-core": "core", "easytier-cli": "cli"})
	source := provenance{Version: "v2.6.4", Asset: "release.zip", SHA256: strings.Repeat("a", 64)}
	if _, err := writeRelease(archive, out, "linux", "amd64", source); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "old")); !os.IsNotExist(err) {
		t.Fatal("old platform contents survived replacement")
	}
	data, err := os.ReadFile(filepath.Join(target, "provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["version"] != source.Version || fields["asset"] != source.Asset || fields["sha256"] != source.SHA256 {
		t.Fatalf("provenance = %v", fields)
	}
	if _, exists := fields["fetched"]; exists {
		t.Fatal("provenance contains a fetch timestamp")
	}
	assertNoStaging(t, out)
}

func TestDownloadLimitAndFailureCleanup(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	for _, testCase := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "over limit", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			_, _ = w.Write([]byte("123456789"))
		}},
		{name: "interrupted", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("short"))
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(testCase.handler)
			defer server.Close()
			archive, _, err := downloadWithClient(server.Client(), server.URL, 8)
			if err == nil {
				os.Remove(archive)
				t.Fatal("invalid download was accepted")
			}
			assertTempEmpty(t, tempDir)
		})
	}
}

func TestDownloadSuccess(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("release"))
	}))
	defer server.Close()
	archive, digest, err := downloadWithClient(server.Client(), server.URL, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(archive)
	hash := sha256.Sum256([]byte("release"))
	if digest != hex.EncodeToString(hash[:]) {
		t.Fatalf("digest = %s", digest)
	}
	data, err := os.ReadFile(archive)
	if err != nil || string(data) != "release" {
		t.Fatalf("archive = %q, %v", data, err)
	}
}

func TestLatestReleaseLimitAndClientTimeout(t *testing.T) {
	if httpClient.Timeout == 0 {
		t.Fatal("HTTP client has no timeout")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"` + strings.Repeat("x", 100) + `"}`))
	}))
	defer server.Close()
	if _, err := latestReleaseFrom(server.Client(), server.URL, 16); err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("oversized latest response error = %v", err)
	}
}

func TestWriteReleasePreservesPlaceholders(t *testing.T) {
	out := t.TempDir()
	target := filepath.Join(out, "linux-amd64")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	placeholder := []byte("keep this directory in source archives\n")
	if err := os.WriteFile(filepath.Join(target, "PLACEHOLDER.md"), placeholder, 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(out, "darwin-arm64")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "PLACEHOLDER.md"), placeholder, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "easytier-core.gz"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := makeReleaseZip(t, map[string]string{"easytier-core": "core", "easytier-cli": "cli"})
	if _, err := writeRelease(archive, out, "linux", "amd64", provenance{}); err != nil {
		t.Fatal(err)
	}
	if err := clearOtherPlatforms(out, "linux-amd64"); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{target, other} {
		got, err := os.ReadFile(filepath.Join(dir, "PLACEHOLDER.md"))
		if err != nil || string(got) != string(placeholder) {
			t.Fatalf("placeholder in %s = %q, %v", dir, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(other, "easytier-core.gz")); !os.IsNotExist(err) {
		t.Fatal("clean retained another platform's generated core")
	}
}

func makeLockFile(t *testing.T) string {
	t.Helper()
	lock := coreLock{
		Version: "v2.6.4",
		Platforms: map[string]lockedAsset{
			"linux/amd64": {Asset: "locked-linux.zip", SHA256: strings.Repeat("a", 64)},
		},
	}
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "core-lock.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertTempEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary files remain: %v", entries)
	}
}

func assertNoStaging(t *testing.T, out string) {
	t.Helper()
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "-staging-") || strings.Contains(entry.Name(), "-backup-") {
			t.Fatalf("temporary release directory remains: %s", entry.Name())
		}
	}
}
