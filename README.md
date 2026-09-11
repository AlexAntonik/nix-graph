## nix-graph

<img width="1400" height="800" alt="demo" src="https://github.com/user-attachments/assets/233c3b8c-beab-4cfb-a104-301786a2ce47" />

Interactive TUI viewer for Nix dependency graphs. Like [`nix-tree`](https://github.com/utdemir/nix-tree), but more feature rich (sort, filter, reverse (dependents) view) and not that cool (Not in Haskell).
Written in pure Go, no dependencies; only `nix` at runtime.

### How it works

It runs `nix path-info` and renders the closure of the target as an expandable tree.
You can inverse graph at any node so tree becomes dependents tree.

Columns:

- NAR Size: The uncompressed size (in bytes) of store path after it is serialized into a [Nix Archive (NAR)](https://nix.dev/manual/nix/2.22/protocols/nix-archive) format.
- Closure size: Size of the store path and all its transitive dependencies.
- Added size: Size of the store path, and all its transitive dependencies minus sizes of already present dependencies in store, i.e., the cost of having that store path on top of all other paths.

### Install

Run without installing:

``` bash
nix run github:AlexAntonik/nix-graph
```

Nix profile:

``` bash
nix profile add github:Alexantonik/nix-graph
```

Flakes:

```nix
# flake.nix
{
  inputs.nix-graph.url = "github:AlexAntonik/nix-graph";
  inputs.nix-graph.inputs.nixpkgs.follows = "nixpkgs";

  outputs = { self, nixpkgs, nix-graph }: {
    nixosConfigurations.yourhostname = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        ./configuration.nix
        ({ pkgs, ... }: {
          environment.systemPackages = [
            inputs.nix-graph.packages.${pkgs.stdenv.hostPlatform.system}.nix-graph
          ];
        })
      ];
    };
  };
}
```

### Usage

``` bash
nix-graph [path]
```

where `path` is a store path, profile, or derivation (default: `/run/current-system`).

Examples:

``` bash
nix-graph                                             # current system closure
nix-graph "$(which bash)"                             # current bash closure
nix-graph /nix/store/x9...m-nix-2.34.8                # a specific package closure
nix-graph "$(nix eval --raw 'nixpkgs#bash.drvPath')"  # a .drv: build-time graph
```

#### Reverse view (`p`)

Press `p` to flip the tree at selected node: the node you are on becomes root and the tree inverts, showing its dependents. Press `p` again and it flips back to dependencies.
That means you can walk down from the top level to some package, flip the view, trace your way back up to see what depends on it, flip again, and keep going back and forth as much as you want. `esc` resets to the default dependency tree, and `P` opens every package as its own root.

### Contributing

All contributions, issues and feature requests are welcome.

### Similar

- [nix-tree](https://github.com/utdemir/nix-tree) — Inspiration, similar functionality
- [nix-melt](https://github.com/nix-community/nix-melt) — A ranger-like flake.lock viewer
