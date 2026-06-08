{
  description = "Hyprland monitor configuration that actually works";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
  }:
    flake-utils.lib.eachDefaultSystem (
      system: let
        pkgs = import nixpkgs {inherit system;};
        version = "1.7.0";
      in {
        packages = rec {
          default = hyprmoncfg;
          hyprmoncfg = pkgs.buildGoModule {
            pname = "hyprmoncfg";
            version = "v${version}";
            src = pkgs.fetchFromGitHub {
              owner = "crmne";
              repo = "hyprmoncfg";
              rev = "v${version}";
              hash = "sha256-6qupQ7/Uax6giaWC9o25EptyJNx6JdqrQX+w4WDBPTw=";
            };

            subpackages = ["cmd/hyprmoncfg" "cmd/hyprmoncfgd"];
            proxyVendor = true;
            vendorHash = "sha256-97z4+U/SumG5sidy62SW43E+Bi6FpvJKCI6wqwXts2g=";
            doCheck = false;

            postInstall = ''
              mkdir -p $out/share/applications
              cp $src/packaging/applications/hyprmoncfg.desktop $out/share/applications/

              mkdir -p $out/share/icons/hicolor/scalable/apps
              cp $src/packaging/icons/hyprmoncfg.svg $out/share/icons/hicolor/scalable/apps/
            '';
          };
        };
      }
    )
    // {
      nixosModules.default = {
        config,
        lib,
        pkgs,
        ...
      }: let
        cfg = config.programs.hyprmoncfg;
      in {
        options.programs.hyprmoncfg = {
          enable = lib.mkEnableOption "hyprmoncfg, a Hyprland monitor configuration daemon";
          package = lib.mkOption {
            type = lib.types.package;
            default = self.packages.${pkgs.system}.hyprmoncfg;
            defaultText = lib.literalExpression "inputs.hyprmoncfg.packages.\${pkgs.system}.hyprmoncfg";
            description = "The hyprmoncfg package to use.";
          };
        };

        config = lib.mkIf cfg.enable {
          environment.systemPackages = [cfg.package];

          systemd.user.services.hyprmoncfgd = {
            description = "Hyprland monitor profile daemon (hyprmoncfgd)";
            after = ["graphical-session.target"];
            wantedBy = ["default.target"];
            path = [pkgs.hyprland];
            serviceConfig = {
              Type = "simple";
              ExecStart = "${cfg.package}/bin/hyprmoncfgd";
              Restart = "on-failure";
              RestartSec = "2";
            };
          };
        };
      };
    };
}
