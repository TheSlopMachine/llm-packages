{
  description = "TheSlopMachine packages";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      # Nix system to release platform. The release records live in releases/.
      platforms = {
        "x86_64-linux" = "linux_amd64";
        "aarch64-linux" = "linux_arm64";
        "i686-linux" = "linux_386";
        "armv7l-linux" = "linux_arm";
        "x86_64-darwin" = "darwin_amd64";
        "aarch64-darwin" = "darwin_arm64";
      };

      releases = map
        (file: builtins.fromJSON (builtins.readFile (./releases + "/${file}")))
        (builtins.attrNames (builtins.readDir ./releases));

      packagesFor = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          platform = platforms.${system};

          build = release:
            let
              asset = release.assets.${platform};
            in
            pkgs.stdenv.mkDerivation {
              pname = release.name;
              version = release.version;
              src = pkgs.fetchurl {
                url = asset.url;
                sha256 = asset.sha256;
              };
              sourceRoot = ".";
              nativeBuildInputs = [ pkgs.unzip ]
                ++ pkgs.lib.optionals pkgs.stdenv.hostPlatform.isLinux [ pkgs.autoPatchelfHook ];
              dontConfigure = true;
              dontBuild = true;
              installPhase = ''
                runHook preInstall
                install -Dm755 ${release.binary} $out/bin/${release.binary}
                runHook postInstall
              '';
              meta = {
                description = release.description;
                homepage = release.homepage;
                license = pkgs.lib.getLicenseFromSpdxId release.license;
                mainProgram = release.binary;
                platforms = builtins.attrNames platforms;
                sourceProvenance = [ pkgs.lib.sourceTypes.binaryNativeCode ];
              };
            };

          available = builtins.filter (release: release.assets ? ${platform}) releases;
        in
        builtins.listToAttrs (map (release: { name = release.name; value = build release; }) available);
    in
    {
      packages = nixpkgs.lib.genAttrs (builtins.attrNames platforms) packagesFor;
    };
}
