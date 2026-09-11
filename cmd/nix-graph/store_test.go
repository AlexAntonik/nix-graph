package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression: /run/current-system/sw/bin/nix resolves to a file inside a
// store path, which Load could not find. storeRoot must collapse such paths
func TestStoreRoot(t *testing.T) {
	pkg := storePrefix + "0123456789abcdefghij0123456789ab-nix-2.34.8"
	tests := []struct {
		in   string
		want string
	}{
		{pkg, pkg},
		{pkg + "/bin/nix", pkg},
		{pkg + "/share/doc/nix/nix.conf.example", pkg},
		{storePrefix + "0123456789abcdefghij0123456789ab-bash-5.2-p103.drv", storePrefix + "0123456789abcdefghij0123456789ab-bash-5.2-p103.drv"},
		{"/etc/hosts", "/etc/hosts"},
	}
	for _, tt := range tests {
		if got := storeRoot(tt.in); got != tt.want {
			t.Errorf("storeRoot(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveStoreRejectsNonStore(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(plain, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveStore(plain); err == nil {
		t.Errorf("resolveStore(%q) = nil error, want rejection of a path outside %s*", plain, storePrefix)
	}
}

func TestResolveStoreMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if _, err := resolveStore(missing); err == nil {
		t.Errorf("resolveStore(%q) = nil error, want a resolution failure", missing)
	}
}
