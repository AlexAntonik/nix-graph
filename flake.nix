{
  description = "Interactive TUI viewer for Nix dependency graphs";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      name = "nix-graph";
      version = "0.0.1";
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          nix-graph = pkgs.buildGoModule {
            pname = name;
            inherit version;

            src = self;
            vendorHash = null;
            doCheck = true;
            env.CGO_ENABLED = "0";
            ldflags = [
              "-s"
              "-w"
              "-X main.version=${version}"
            ];

            nativeBuildInputs = [ pkgs.makeWrapper ];
            postInstall = ''
              wrapProgram $out/bin/nix-graph \
                --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.nix ]}
            '';

            meta = with pkgs.lib; {
              description = "Interactive TUI viewer for Nix dependency graphs";
              mainProgram = "nix-graph";
              platforms = platforms.linux;
            };
          };
        in
        {
          inherit nix-graph;
          default = nix-graph;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/nix-graph";
        };
      });

      devShells = forAllSystems (system: {
        default = nixpkgs.legacyPackages.${system}.mkShell {
          packages = with nixpkgs.legacyPackages.${system}; [
            go
            gopls
            nix
            nixfmt
          ];
        };
      });

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt-tree);

      checks = self.packages;
    };
}
