{
  description = "Nix packaging for hyprmoncfg";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
    in
    {
      packages = nixpkgs.lib.genAttrs systems (system:
        let
          pkgs = import nixpkgs { inherit system; };
          package = pkgs.callPackage ./default.nix { };
        in
        {
          default = package;
          hyprmoncfg = package;
        });
    };
}
