{
  description = "Terminal-native GitHub service health dashboard";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        rec {
          gh-pulse = pkgs.buildGoModule (finalAttrs: {
            pname = "gh-pulse";
            version = "0.1.0";

            src = pkgs.lib.fileset.toSource {
              root = ./.;
              fileset = pkgs.lib.fileset.unions [
                ./cmd
                ./internal
                ./go.mod
                ./go.sum
              ];
            };
            vendorHash = "sha256-A2RV8e62VAaSFgxIzLhKXWcv785tvJiNRJ2wUal6n/I=";
            subPackages = [ "cmd/gh-pulse" ];
            dontStrip = true;

            ldflags = [
              "-X main.version=${finalAttrs.version}"
            ];

            meta = {
              description = "Terminal-native GitHub service health and uptime dashboard";
              homepage = "https://github.com/cpcloud/gh-pulse";
              mainProgram = "gh-pulse";
            };
          });

          default = gh-pulse;
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          vhsFontsConf = pkgs.makeFontsConf {
            fontDirectories = [ "${pkgs.nerd-fonts.hack}/share/fonts/truetype" ];
          };
        in
        {
          default = pkgs.mkShellNoCC {
            packages = [
              pkgs.actionlint
              pkgs.ffmpeg-headless
              pkgs.go
              pkgs.golangci-lint
              pkgs.goreleaser
              pkgs.gopls
              pkgs.gotools
              pkgs.markdownlint-cli2
              pkgs.nerd-fonts.hack
              pkgs.nixfmt
              pkgs.prek
              pkgs.vhs
            ];

            FONTCONFIG_FILE = vhsFontsConf;
          };
        }
      );

      formatter = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        pkgs.nixfmt
      );
    };
}
