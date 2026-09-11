package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func resolveStore(path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(real, storePrefix) {
		return "", fmt.Errorf("%s does not resolve to %s*", path, storePrefix)
	}
	return storeRoot(real), nil
}

// storeRoot truncates a resolved store path to the package itself
func storeRoot(path string) string {
	rest, ok := strings.CutPrefix(path, storePrefix)
	if !ok {
		return path
	}
	if root, _, cut := strings.Cut(rest, "/"); cut && len(root) > hashLen {
		return storePrefix + root
	}
	return path
}
