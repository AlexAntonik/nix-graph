{
  lib,
  buildGoModule,
}:
buildGoModule (finalAttrs: {
  pname = "nix-graph";
  version = "0.0.6";

  src = lib.cleanSource ./.;
  subPackages = [ "cmd/nix-graph" ];
  vendorHash = null;
  env.CGO_ENABLED = "0";
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${finalAttrs.version}"
  ];

  meta = {
    homepage = "https://github.com/AlexAntonik/nix-graph";
    description = "Interactive TUI viewer for Nix dependency graphs";
    license = lib.licenses.mit;
    mainProgram = "nix-graph";
    platforms = lib.platforms.unix;
  };
})
