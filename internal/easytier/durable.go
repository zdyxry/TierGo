package easytier

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/zdyxry/easytier-mygo/internal/owner"
)

// Secrets are kept out of the activity log: the core prints its own
// configuration in some paths, and a window that shows it would leak the
// network secret to anyone looking at the screen.
var secrets struct {
	mu     sync.Mutex
	owners map[string]map[string]struct{}
}

func replaceSecrets(owner string, values ...string) {
	set := map[string]struct{}{}
	for _, value := range values {
		if len(value) >= 4 {
			set[value] = struct{}{}
		}
	}
	secrets.mu.Lock()
	defer secrets.mu.Unlock()
	if secrets.owners == nil {
		secrets.owners = map[string]map[string]struct{}{}
	}
	if len(set) == 0 {
		delete(secrets.owners, owner)
		return
	}
	secrets.owners[owner] = set
}

func forgetSecrets(owner string) {
	secrets.mu.Lock()
	delete(secrets.owners, owner)
	secrets.mu.Unlock()
}

// rememberSecret registers values to redact from log entries.
func rememberSecret(values ...string) {
	replaceSecrets("", values...)
}

// redact replaces every known secret in a message.
func redact(message string) string {
	secrets.mu.Lock()
	var values []string
	for _, owned := range secrets.owners {
		for value := range owned {
			values = append(values, value)
		}
	}
	secrets.mu.Unlock()
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) != len(values[j]) {
			return len(values[i]) > len(values[j])
		}
		return values[i] < values[j]
	})
	for _, value := range values {
		message = strings.ReplaceAll(message, value, "****")
	}
	return message
}

// writeFileDurable writes a file so that a crash leaves either the old or the
// new content, never a half-written one: a temporary file beside it, flushed,
// then renamed over the target.
func writeFileDurable(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	owner.Keep(filepath.Dir(dir), dir)
	temporary, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name) // no-op once renamed

	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(perm); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	owner.Keep(dir, path)
	syncDir(dir)
	return nil
}
