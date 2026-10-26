package easytier

import (
	"strings"
	"testing"
)

func resetSecrets(t *testing.T) {
	t.Helper()
	secrets.mu.Lock()
	previous := secrets.owners
	secrets.owners = nil
	secrets.mu.Unlock()
	t.Cleanup(func() {
		secrets.mu.Lock()
		secrets.owners = previous
		secrets.mu.Unlock()
	})
}

func TestSecretsAreBoundedByOwner(t *testing.T) {
	resetSecrets(t)
	for range 20 {
		replaceSecrets("one", "alpha-secret", "alpha-secret", "x")
	}
	secrets.mu.Lock()
	owners := len(secrets.owners)
	values := len(secrets.owners["one"])
	secrets.mu.Unlock()
	if owners != 1 || values != 1 {
		t.Fatalf("repeated registration grew to %d owners and %d values", owners, values)
	}

	replaceSecrets("two", "beta-secret")
	forgetSecrets("one")
	got := redact("beta-secret and alpha-secret")
	if strings.Contains(got, "beta-secret") || !strings.Contains(got, "alpha-secret") {
		t.Fatalf("forgetting one owner affected another: %q", got)
	}
	forgetSecrets("two")
	secrets.mu.Lock()
	remaining := len(secrets.owners)
	secrets.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("%d secret owners remain after deletion", remaining)
	}
}
