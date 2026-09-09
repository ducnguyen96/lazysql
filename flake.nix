{
  description = "Cross-platform TUI database management tool written in Go";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

      version = "0.5.6-unstable-${self.shortRev or self.dirtyShortRev or "dirty"}";
    in
    {
      overlays.default = final: _prev: {
        lazysql = final.callPackage ./package.nix { inherit version; };
      };

      packages = forAllSystems (pkgs: rec {
        lazysql = pkgs.callPackage ./package.nix { inherit version; };
        default = lazysql;
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          inputsFrom = [ self.packages.${pkgs.stdenv.hostPlatform.system}.lazysql ];
          packages = with pkgs; [
            go
            gopls
            golangci-lint
          ];
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt-tree);
    };
}
