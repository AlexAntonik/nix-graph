package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usage = `
Nix dependency graph tui viewer

usage: nix-graph [path]

path is a store path or profile (default: /run/current-system)
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nix-graph:", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	target := flag.Arg(0)
	if target == "" {
		var err error
		target, err = defaultRoot()
		if err != nil {
			return err
		}
	}
	root, err := resolveStore(target)
	if err != nil {
		return err
	}

	restore, err := setupTerminal()
	if err != nil {
		return fmt.Errorf("need a terminal: %w", err)
	}
	defer restore()

	enterScreen()
	g, err := loadGraph(root)
	if err != nil {
		return err
	}
	return NewUI(g).loop()
}

func defaultRoot() (string, error) {
	for _, p := range []string{"/run/current-system", "/nix/var/nix/profiles/system"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no default profile found, pass a store path as argument")
}

func resolveStore(path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(real, storePrefix) {
		return "", fmt.Errorf("%s does not resolve to %s*", path, storePrefix)
	}
	return real, nil
}
