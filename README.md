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

## Publish an app

The `update` workflow publishes every app in `apps.json`. It runs on a `release` dispatch, every 15 minutes, and on a manual start.

An app qualifies when its latest GitHub release has:

- a tag `vX.Y` or `vX.Y.Z`;
- one archive per platform named `<binary>_<goos>_<goarch>.zip` with the binary at the archive root;
- `checksums.txt` with `<sha256>  <path>/<archive>` lines.

Add the app to `apps.json`. Then add this step after the release step of the app's workflow:

```yaml
- name: Update package repositories
  env:
    GH_TOKEN: ${{ secrets.PACKAGES_DISPATCH_TOKEN }}
  run: gh api repos/TheSlopMachine/llm-packages/dispatches -f event_type=release
```

`PACKAGES_DISPATCH_TOKEN` is a fine-grained token for `llm-packages` with the "Contents: read and write" permission.

The signing keys are created once with `scripts/create-signing-keys.sh`. Back them up offline: users trust them when they add a repository.
