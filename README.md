# llm-packages

Package repositories for TheSlopMachine apps. Every package follows the latest release of its app.

Apps: `llm-router`.

## Install

### Scoop (Windows)

```powershell
scoop bucket add theslopmachine https://github.com/TheSlopMachine/llm-packages
scoop install theslopmachine/llm-router
```

### Homebrew (macOS, Linux)

```sh
brew tap theslopmachine/packages https://github.com/TheSlopMachine/llm-packages
brew install theslopmachine/packages/llm-router
```

### Nix (macOS, Linux)

```sh
nix profile install github:TheSlopMachine/llm-packages#llm-router
```

### APT (Debian 12+, Ubuntu 22.04+)

```sh
sudo curl -fsSLo /etc/apt/sources.list.d/theslopmachine.sources https://theslopmachine.github.io/llm-packages/apt/theslopmachine.sources
sudo apt update && sudo apt install llm-router
```

### DNF (Fedora, RHEL)

```sh
sudo curl -fsSLo /etc/yum.repos.d/theslopmachine.repo https://theslopmachine.github.io/llm-packages/rpm/theslopmachine.repo
sudo dnf install llm-router
```

### Zypper (openSUSE)

```sh
sudo zypper addrepo https://theslopmachine.github.io/llm-packages/rpm/theslopmachine.repo
sudo zypper install llm-router
```

### APK (Alpine)

Run as root:

```sh
wget -qO /etc/apk/keys/theslopmachine.rsa.pub https://theslopmachine.github.io/llm-packages/keys/theslopmachine.rsa.pub
echo https://theslopmachine.github.io/llm-packages/apk >> /etc/apk/repositories
apk update && apk add llm-router
```

### pkg (FreeBSD 14, 15)

Run as root:

```sh
mkdir -p /usr/local/etc/pkg/repos && cd /usr/local/etc/pkg/repos
fetch https://theslopmachine.github.io/llm-packages/freebsd/theslopmachine.conf https://theslopmachine.github.io/llm-packages/keys/theslopmachine.rsa.pub
pkg update && pkg install llm-router
```

## Run llm-router as a service

APT, DNF, Zypper, APK and pkg packages install a service. It does not start on its own. Set the dashboard password, then start the service and enable it at boot:

```sh
sudo llm-router set-dashboard-password
sudo llm-router daemon enable --now
```

`sudo llm-router daemon start|stop|restart|status` controls the service through systemd, OpenRC or FreeBSD rc.d.

The service reads `/etc/llm-router/.env` (`/usr/local/etc/llm-router/.env` on FreeBSD). Change settings with `sudo llm-router config`, then run `sudo llm-router daemon restart`. Back up the configuration directory with the data: provider keys cannot be decrypted without `credentials.key`.

Upgrades restart a running service.

With Homebrew, run `llm-router set-dashboard-password` and `brew services start llm-router`.

### NixOS

```nix
{
  inputs.llm-packages.url = "github:TheSlopMachine/llm-packages";

  outputs = { nixpkgs, llm-packages, ... }: {
    nixosConfigurations.host = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        llm-packages.nixosModules.llm-router
        {
          services.llm-router.enable = true;
          services.llm-router.settings.HOST = "0.0.0.0";
        }
      ];
    };
  };
}
```

Set the password with `sudo llm-router set-dashboard-password`. Settings go into `services.llm-router.settings`: NixOS rewrites `/etc/llm-router/.env` on every switch.

## Publish an app

The `update` workflow publishes every app in `apps.json`. It runs on a `release` dispatch, every 15 minutes, and on a manual start.

An app qualifies when its latest GitHub release has:

- a tag `vX.Y` or `vX.Y.Z`;
- one archive per platform named `<binary>_<goos>_<goarch>.zip` with the binary at the archive root;
- `checksums.txt` with `<sha256>  <path>/<archive>` lines.

Every package places `../share/<app>/package-manager` next to the binary. It holds the package manager name. An app with a self-update reads it and refuses to update itself.

A server app adds `service` to its `apps.json` entry:

```json
"service": {
    "arguments": ["serve"],
    "configuration_file": ".env",
    "configuration": {"DB_PATH": "{state}/app.sqlite3"}
}
```

Packages run `<binary> <arguments>` as the user `<app>` and install `configuration` as `<configuration directory>/<configuration_file>`, owned by that user. `{configuration}`, `{state}` and `{cache}` expand to `/etc/<app>`, `/var/lib/<app>` and `/var/cache/<app>` on Linux, and to `/usr/local/etc/<app>`, `/var/db/<app>` and `/var/cache/<app>` on FreeBSD.

Add the app to `apps.json`. Then add this step after the release step of the app's workflow:

```yaml
- name: Update package repositories
  env:
    GH_TOKEN: ${{ secrets.PACKAGES_DISPATCH_TOKEN }}
  run: gh api repos/TheSlopMachine/llm-packages/dispatches -f event_type=release
```

`PACKAGES_DISPATCH_TOKEN` is a fine-grained token for `llm-packages` with the "Contents: read and write" permission.

The signing keys are created once with `scripts/create-signing-keys.sh`. Back them up offline: users trust them when they add a repository.
