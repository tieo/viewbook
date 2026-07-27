{
  description = "viewbook; a model of an app's views, read and edited in one place";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAll = f: nixpkgs.lib.genAttrs systems (s: f (import nixpkgs { system = s; }));
    in {
      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [ go gopls gotools nodejs pnpm ];
        };
      });

      packages = forAll (pkgs: rec {
        viewbook = pkgs.buildGoModule {
          pname = "viewbook";
          version = "0.1.0";
          src = ./.;
          # The interface is committed built, and go:embed carries it into the
          # binary, so installing this needs no node and no npm step.
          vendorHash = null;
          subPackages = [ "cmd/viewbook" ];
          meta = {
            description = "a model of an app's views: what each screen is for, what it has to do, and how it renders today";
            mainProgram = "viewbook";
            platforms = systems;
            license = nixpkgs.lib.licenses.mit;
          };
        };
        default = viewbook;
      });

      apps = forAll (pkgs: rec {
        viewbook = {
          type = "app";
          program = "${self.packages.${pkgs.system}.viewbook}/bin/viewbook";
        };
        default = viewbook;
      });
    };
}
