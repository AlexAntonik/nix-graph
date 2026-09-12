package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

const usage = `
Nix dependency graph tui viewer

usage: nix-graph [path]

path is a store path or profile (default: /run/current-system)
a .drv store path shows the build-time graph instead of the runtime one

Examples:

nix-graph                                             # current system closure
nix-graph "$(which bash)"                             # current bash closure
nix-graph /nix/store/x9...m-nix-2.34.8                # a specific package closure
nix-graph "$(nix eval --raw 'nixpkgs#bash.drvPath')"  # a .drv: build-time graph


Options:

  --help                  Print help
  --version               Print version
  --expand-limit <limit>  Max tree nodes for expand-all (default 60000, 0 = no limit)
                          With no limit, large closures can blow up the tree to tens of
                          millions of nodes, causing heavy RAM usage and performance degradation.

`

var version = "0.0.1"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nix-graph:", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version")
	flag.IntVar(&expandLimit, "expand-limit", expandLimit, "max tree nodes for expand-all, 0 for unlimited")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("nix-graph", version)
		return nil
	}

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
