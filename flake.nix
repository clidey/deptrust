{
  description = "deptrust — local package vulnerability checker and MCP server for AI agents";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }: let
    version = "0.9.0";

    assets = {
      "x86_64-linux" = {
        file = "deptrust_v${version}_linux_amd64.tar.gz";
        sha256 = "sha256-qL/N68QDrzGB7wLsy0X3sk4bpgzKXexNKZQ/bEvZ0RA=";
      };
      "aarch64-linux" = {
        file = "deptrust_v${version}_linux_arm64.tar.gz";
        sha256 = "sha256-g6f5GH37PLP0A1zWb5mzlSVGk963ZkJztXkW0ZU8+iY=";
      };
      "x86_64-darwin" = {
        file = "deptrust_v${version}_darwin_amd64.tar.gz";
        sha256 = "sha256-vRch3/CkvkgKEB8sqi2WK4MlL/+ucDI9DgA9hqVi4VU=";
      };
      "aarch64-darwin" = {
        file = "deptrust_v${version}_darwin_arm64.tar.gz";
        sha256 = "sha256-yvSrmO55FFTyQAFDTyFAri4Xcp8Z/dUY7UYXtF6wjag=";
      };
    };

    systems = builtins.attrNames assets;
    forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);

    deptrustFor = system: let
      pkgs = nixpkgs.legacyPackages.${system};
      asset = assets.${system};
    in pkgs.stdenv.mkDerivation {
      pname = "deptrust";
      inherit version;

      src = pkgs.fetchurl {
        url = "https://github.com/clidey/deptrust/releases/download/v${version}/${asset.file}";
        sha256 = asset.sha256;
      };

      nativeBuildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.autoPatchelfHook ];
      buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.stdenv.cc.cc.lib ];

      dontConfigure = true;
      dontBuild = true;

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
    in {
      deptrust = {
        type = "app";
        program = "${deptrustPkg}/bin/deptrust";
      };
      default = {
        type = "app";
        program = "${deptrustPkg}/bin/deptrust";
      };
    });
  };
}
