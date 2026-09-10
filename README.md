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

wher `path` is a store path or profile (default: `/run/current-system`).

Examples:

``` bash
nix-graph                               # current system closure
nix-graph /run/current-system/sw        # the sw profile
nix-graph /nix/store/x9...m-nix-2.34.8  # a single package closure
```

### Contributing

All contributions, issues and feature requests are welcome.

### Similar

- [nix-tree](https://github.com/utdemir/nix-tree) — Inspiration, similar functionality
- [nix-melt](https://github.com/nix-community/nix-melt) — A ranger-like flake.lock viewer
