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
              # Apps read the package-manager marker to refuse a self-update.
              installPhase = ''
                runHook preInstall
                install -Dm755 ${release.binary} $out/bin/${release.binary}
                mkdir -p $out/share/${release.name}
                echo Nix >$out/share/${release.name}/package-manager
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

      # Apps with a service get a NixOS module: services.<app>.enable runs the app as a system service.
      serviceReleases = builtins.filter (release: release ? service) releases;

      moduleFor = release: { config, lib, pkgs, ... }:
        let
          name = release.name;
          service = release.service;
          cfg = config.services.${name};
        in
        {
          options.services.${name} = {
            enable = lib.mkEnableOption release.description;
            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.stdenv.hostPlatform.system}.${name};
              defaultText = lib.literalExpression "llm-packages.packages.\${system}.${name}";
              description = "The package to run.";
            };
            settings = lib.mkOption {
              type = lib.types.attrsOf lib.types.str;
              default = { };
              example = { HOST = "0.0.0.0"; };
              description = "Lines added to /etc/${name}/${service.configuration_file}. They win over the package defaults.";
            };
          };

          config = lib.mkIf cfg.enable {
            users.users.${name} = {
              isSystemUser = true;
              group = name;
              home = "/var/lib/${name}";
            };
            users.groups.${name} = { };
            environment.systemPackages = [ cfg.package ];
            # NixOS does not link arbitrary /share entries into the system profile by default.
            # Expose this app's package-manager marker at the path used by the binary.
            environment.pathsToLink = [ "/share/${name}" ];
            # The same file as in the Linux packages. The owner lets `sudo ${name}` act as the service user.
            environment.etc."${name}/${service.configuration_file}" = {
              text = builtins.readFile (./services + "/${name}/linux/configuration")
                + lib.concatStrings (lib.mapAttrsToList (key: value: "${key}=${value}\n") cfg.settings);
              mode = "0640";
              user = name;
              group = name;
            };
            # The service writes into its configuration directory, for example a generated key.
            systemd.tmpfiles.rules = [ "d /etc/${name} 0750 ${name} ${name} -" ];
            systemd.services.${name} = {
              description = release.description;
              wantedBy = [ "multi-user.target" ];
              wants = [ "network-online.target" ];
              after = [ "network-online.target" ];
              serviceConfig = {
                ExecStart = builtins.concatStringsSep " " ([ (lib.getExe cfg.package) ] ++ service.arguments);
                User = name;
                Group = name;
                StateDirectory = name;
                StateDirectoryMode = "0750";
                CacheDirectory = name;
                CacheDirectoryMode = "0750";
                Restart = "on-failure";
                RestartSec = 5;
                NoNewPrivileges = true;
                PrivateTmp = true;
              };
            };
          };
        };

      modules = builtins.listToAttrs (map (release: { name = release.name; value = moduleFor release; }) serviceReleases);

      # Boots a NixOS machine with each service enabled. Needs KVM, so CI runs it on code changes only.
      checksFor = system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          check = release: pkgs.testers.runNixOSTest {
            name = "${release.name}-service";
            nodes.machine = {
              imports = [ (moduleFor release) ];
              services.${release.name}.enable = true;
            };
            testScript = ''
              machine.wait_for_unit("multi-user.target")
              machine.succeed("test -x /run/current-system/sw/bin/${release.binary}")
              machine.succeed("grep -qx Nix /run/current-system/sw/share/${release.name}/package-manager")
              machine.succeed("test \"$(stat -c %U:%a /etc/${release.name})\" = ${release.name}:750")
              machine.succeed("test \"$(stat -c %U:%a /etc/${release.name}/${release.service.configuration_file})\" = ${release.name}:640")
              machine.succeed("test \"$(stat -c %U /var/lib/${release.name})\" = ${release.name}")
            '';
          };
        in
        builtins.listToAttrs (map (release: { name = "${release.name}-service"; value = check release; })
          (builtins.filter (release: release.assets ? ${platforms.${system}}) serviceReleases));
    in
    {
      packages = nixpkgs.lib.genAttrs (builtins.attrNames platforms) packagesFor;
      nixosModules = modules // {
        default = { imports = builtins.attrValues modules; };
      };
      checks.x86_64-linux = checksFor "x86_64-linux";
    };
}
