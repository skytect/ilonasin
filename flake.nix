{
  description = "Local OpenAI-compatible LLM router";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f (import nixpkgs { inherit system; }));
    in
    {
      packages = forAllSystems (
        pkgs:
        let
          ilonasin = pkgs.callPackage ./nix/package.nix {
            commit = self.rev or self.dirtyRev or "";
          };
        in
        {
          inherit ilonasin;
          default = ilonasin;
        }
      );

      homeManagerModules = {
        ilonasin = import ./nix/home-manager.nix { inherit self; };
        default = self.homeManagerModules.ilonasin;
      };
    };
}
