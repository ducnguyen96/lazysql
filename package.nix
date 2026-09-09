{
  lib,
  stdenv,
  buildGoModule,
  libx11,
  version ? "0.5.6-unstable",
}:

buildGoModule {
  pname = "lazysql";
  inherit version;

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./go.mod
      ./go.sum
      ./main.go
      ./app
      ./commands
      ./components
      ./drivers
      ./helpers
      ./internal
      ./keymap
      ./lib
      ./models
    ];
  };

  vendorHash = "sha256-FbAt/HsjoxqAKWQqqWN2xuyyTG2Ic4DcyEU4O0rjpQE=";

  ldflags = [ "-X main.version=${version}" ];

  buildInputs = lib.optionals stdenv.hostPlatform.isLinux [ libx11 ];

  meta = {
    description = "Cross-platform TUI database management tool written in Go";
    homepage = "https://github.com/ducnguyen96/lazysql";
    license = lib.licenses.mit;
    mainProgram = "lazysql";
  };
}
