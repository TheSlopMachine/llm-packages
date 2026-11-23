#!/usr/bin/env bash
# Creates the repository signing keys once and stores them as GitHub Actions secrets.
#
# Usage: scripts/create-signing-keys.sh <empty-backup-directory>
# Environment: REPOSITORY  GitHub repository for the secrets (default TheSlopMachine/llm-packages)
# Tools: gpg, openssl, gh (signed in with access to REPOSITORY).
#
# Users trust these keys when they add the repositories. Replacing them breaks every
# installation, so keep the backup directory offline.
set -euo pipefail

backup=${1:?usage: create-signing-keys.sh <empty-backup-directory>}
repository=${REPOSITORY:-TheSlopMachine/llm-packages}

mkdir -p "$backup"
if [[ -n $(ls -A "$backup") ]]; then
	echo "$backup is not empty" >&2
	exit 1
fi

GNUPGHOME=$(mktemp -d)
export GNUPGHOME
trap 'rm -rf "$GNUPGHOME"' EXIT

gpg --batch --pinentry-mode loopback --passphrase '' \
	--quick-generate-key "TheSlopMachine packages <TheSlopMachine@users.noreply.github.com>" rsa4096 sign never
gpg --batch --armor --export-secret-keys >"$backup/gpg-private.asc"
openssl genrsa -traditional -out "$backup/rsa-private.pem" 4096

gh secret set GPG_PRIVATE_KEY --repo "$repository" <"$backup/gpg-private.asc"
gh secret set RSA_PRIVATE_KEY --repo "$repository" <"$backup/rsa-private.pem"
echo "Stored GPG_PRIVATE_KEY and RSA_PRIVATE_KEY in $repository. Backup: $backup"
