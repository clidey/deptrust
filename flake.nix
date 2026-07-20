{
  description = "deptrust — local package vulnerability checker and MCP server for AI agents";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }: let
    version = "0.11.0";

    assets = {
      "x86_64-linux" = {
        file = "deptrust_v${version}_linux_amd64.tar.gz";
        sha256 = "sha256-Sb4XcXLBej5uGphOQgjzxYttCtcHwfAPxR9xHi9rX0U=";
      };
      "aarch64-linux" = {
        file = "deptrust_v${version}_linux_arm64.tar.gz";
        sha256 = "sha256-jjR1H7F5RDO6IuniRmydQJX3ciCy445fhdg1n1kgkK8=";
      };
      "x86_64-darwin" = {
        file = "deptrust_v${version}_darwin_amd64.tar.gz";
        sha256 = "sha256-fXr+3FKGoD5whlSVTuQSE0Fnp35mMJi7ed5mrDyJcPw=";
      };
      "aarch64-darwin" = {
        file = "deptrust_v${version}_darwin_arm64.tar.gz";
        sha256 = "sha256-onIPbz/NrwwlWBlWw0EfyonxlZQntSRldYJ9o1jtS44=";
      };
    };

    systems = builtins.attrNames assets;
    forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);

    deptrustFor = system: let
      pkgs = nixpkgs.legacyPackages.${system};
      asset = assets.${system};
    in pkgs.stdenvNoCC.mkDerivation {
      pname = "deptrust";
      inherit version;

      src = pkgs.fetchurl {
        url = "https://github.com/clidey/deptrust/releases/download/v${version}/${asset.file}";
        sha256 = asset.sha256;
      };

      dontConfigure = true;
      dontBuild = true;
      dontPatchELF = true;

      installPhase = ''
        runHook preInstall
        mkdir -p $out/bin
        install -Dm755 deptrust $out/bin/deptrust
        runHook postInstall
      '';

      meta = with pkgs.lib; {
        description = "Local package vulnerability checker and MCP server for AI agents";
        homepage = "https://github.com/clidey/deptrust";
        downloadPage = "https://github.com/clidey/deptrust/releases";
        license = licenses.mit;
        mainProgram = "deptrust";
        platforms = systems;
        sourceProvenance = [ sourceTypes.binaryNativeCode ];
      };
    };
  in {
    packages = forAllSystems (system: rec {
      deptrust = deptrustFor system;
      default = deptrust;
    });

    apps = forAllSystems (system: let
      deptrustPkg = deptrustFor system;
    in rec {
      deptrust = {
        type = "app";
        program = "${deptrustPkg}/bin/deptrust";
        meta = deptrustPkg.meta;
      };
      default = deptrust;
    });
  };
}
