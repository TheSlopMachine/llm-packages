# AGENTS.md

Rules for AI agents in `llm-packages/`.

## Layout

| Path | Owner | Holds |
|---|---|---|
| `apps.json` | human | Apps to publish |
| `cmd/generate` | human | Resolves releases and renders metadata |
| `releases/` | generated | One release record per app. Every manager reads it or is rendered from it |
| `bucket/` | generated | Scoop manifests |
| `Formula/` | generated | Homebrew formulas |
| `flake.nix` | human | Nix packages read from `releases/` |
| `flake.lock` | CI | Created by the `update` workflow |
| `scripts/` | human | APT, RPM, APK and FreeBSD repository builds for GitHub Pages |

Never edit generated files by hand. Run `go run ./cmd/generate`.

Platform tables live where each format needs them: `render.go` (Scoop, Homebrew), `flake.nix` (Nix), `linux_targets` and `freebsd_targets` in `scripts/`.

## Rules

- Fail closed. A missing archive, checksum or key stops the run with an error.
- Never commit private keys. CI reads them from the `GPG_PRIVATE_KEY` and `RSA_PRIVATE_KEY` secrets.
- Use full words in names. Write plain, short sentences in docs and logs.
- `README.md` holds install commands and the publishing contract. Keep one copy of each fact.

## Verify

- `go vet ./...`, `go test ./...`, `gofmt -l .`
- `bash -n` on every changed script.
- Repository builds run only in CI. Read the `update` workflow logs.
