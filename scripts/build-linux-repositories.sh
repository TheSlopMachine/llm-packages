#!/usr/bin/env bash
# Builds the signed APT, RPM and APK repositories for every record in releases/.
#
# Usage: scripts/build-linux-repositories.sh <site-directory>
# Environment:
#   SITE_URL              public URL of the site, without a trailing slash
#   GPG_PRIVATE_KEY_FILE  armored OpenPGP secret key without a passphrase (APT, RPM)
#   RSA_PRIVATE_KEY_FILE  PEM RSA private key (APK)
# Tools: jq, curl, unzip, sha256sum, file, nfpm, apt-ftparchive, createrepo_c, gpg, openssl, docker.
set -euo pipefail

site=${1:?usage: build-linux-repositories.sh <site-directory>}
: "${SITE_URL:?SITE_URL is not set}"
: "${GPG_PRIVATE_KEY_FILE:?GPG_PRIVATE_KEY_FILE is not set}"
: "${RSA_PRIVATE_KEY_FILE:?RSA_PRIVATE_KEY_FILE is not set}"

readonly key_name=theslopmachine
readonly maintainer="TheSlopMachine <TheSlopMachine@users.noreply.github.com>"
readonly apt_architectures="amd64 i386 arm64 armhf"
# Package revision of every deb, rpm and apk. Raise it to republish the same app version.
readonly package_release=1

# Release platform, nfpm architecture, APK architecture directory.
readonly linux_targets=(
	"linux_amd64 amd64 x86_64"
	"linux_386 386 x86"
	"linux_arm64 arm64 aarch64"
	"linux_arm arm7 armv7"
)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

export GNUPGHOME="$work/gnupg"
mkdir -m 700 "$GNUPGHOME"
gpg --batch --import "$GPG_PRIVATE_KEY_FILE"
fingerprint=$(gpg --batch --with-colons --list-secret-keys | awk -F: '$1 == "fpr" { print $10; exit }')
if [[ -z $fingerprint ]]; then
	echo "no secret key in $GPG_PRIVATE_KEY_FILE" >&2
	exit 1
fi
sign() {
	gpg --batch --yes --local-user "$fingerprint" "$@"
}

mkdir -p "$site/keys" "$site/apt/pool/main" "$site/rpm" "$site/apk"
gpg --batch --armor --export "$fingerprint" >"$site/keys/$key_name.asc"
openssl rsa -in "$RSA_PRIVATE_KEY_FILE" -pubout -out "$site/keys/$key_name.rsa.pub"

# abuild-sign names the signature after the key file, so the key needs this exact name.
mkdir -p "$work/apk-keys"
install -m 600 "$RSA_PRIVATE_KEY_FILE" "$work/apk-keys/$key_name.rsa"

shopt -s nullglob
records=(releases/*.json)
if ((${#records[@]} == 0)); then
	echo "no release records in releases/" >&2
	exit 1
fi

for record in "${records[@]}"; do
	name=$(jq -r .name "$record")
	binary=$(jq -r .binary "$record")
	version=$(jq -r .version "$record")
	for target in "${linux_targets[@]}"; do
		read -r platform nfpm_architecture apk_architecture <<<"$target"
		url=$(jq -r --arg platform "$platform" '.assets[$platform].url // empty' "$record")
		if [[ -z $url ]]; then
			echo "$name $version: no $platform archive"
			continue
		fi
		sha256=$(jq -r --arg platform "$platform" '.assets[$platform].sha256' "$record")

		directory="$work/$name/$platform"
		mkdir -p "$directory/root"
		curl -fsSL --retry 3 -o "$directory/archive.zip" "$url"
		echo "$sha256  $directory/archive.zip" | sha256sum --check --quiet -
		unzip -q "$directory/archive.zip" -d "$directory/root"
		binary_path="$directory/root/$binary"
		if [[ ! -f $binary_path ]]; then
			echo "$url does not contain $binary" >&2
			exit 1
		fi

		# A binary linked against glibc runs on Alpine only through gcompat.
		apk_dependency=""
		if file -b "$binary_path" | grep -q 'dynamically linked'; then
			apk_dependency=gcompat
		fi

		jq -n \
			--slurpfile release "$record" \
			--arg architecture "$nfpm_architecture" \
			--arg maintainer "$maintainer" \
			--arg source "$binary_path" \
			--arg gpg_key "$GPG_PRIVATE_KEY_FILE" \
			--arg rsa_key "$RSA_PRIVATE_KEY_FILE" \
			--arg key_name "$key_name" \
			--arg apk_dependency "$apk_dependency" \
			--arg package_release "$package_release" \
			'$release[0] as $release | {
				name: $release.name,
				arch: $architecture,
				platform: "linux",
				version: $release.version,
				version_schema: "none",
				release: $package_release,
				maintainer: $maintainer,
				description: $release.description,
				homepage: $release.homepage,
				license: $release.license,
				contents: [{src: $source, dst: ("/usr/bin/" + $release.binary), file_info: {mode: 493}}],
				rpm: {signature: {key_file: $gpg_key}},
				apk: {signature: {key_file: $rsa_key, key_name: $key_name}}
			} + (if $apk_dependency == "" then {} else {overrides: {apk: {depends: [$apk_dependency]}}} end)' \
			>"$directory/nfpm.yaml"

		mkdir -p "$site/apt/pool/main/$name" "$site/apk/$apk_architecture"
		nfpm package --config "$directory/nfpm.yaml" --packager deb --target "$site/apt/pool/main/$name/"
		nfpm package --config "$directory/nfpm.yaml" --packager rpm --target "$site/rpm/"
		# apk fetches "<name>-<version>.apk", so the default nfpm file name does not resolve.
		nfpm package --config "$directory/nfpm.yaml" --packager apk \
			--target "$site/apk/$apk_architecture/$name-$version-r$package_release.apk"
		rm -rf "$directory"
	done
done

# APT: one suite "stable" with one component "main".
(
	cd "$site/apt"
	for architecture in $apt_architectures; do
		directory="dists/stable/main/binary-$architecture"
		mkdir -p "$directory"
		apt-ftparchive --arch "$architecture" packages pool >"$directory/Packages"
		gzip -9 --no-name --keep "$directory/Packages"
	done
	apt-ftparchive \
		-o APT::FTPArchive::Release::Origin=TheSlopMachine \
		-o APT::FTPArchive::Release::Label=TheSlopMachine \
		-o APT::FTPArchive::Release::Suite=stable \
		-o APT::FTPArchive::Release::Codename=stable \
		-o "APT::FTPArchive::Release::Architectures=$apt_architectures" \
		-o APT::FTPArchive::Release::Components=main \
		release dists/stable >"$work/Release"
	mv "$work/Release" dists/stable/Release
	sign --clearsign --output dists/stable/InRelease dists/stable/Release
	sign --armor --detach-sign --output dists/stable/Release.gpg dists/stable/Release
)
{
	printf 'Types: deb\nURIs: %s/apt\nSuites: stable\nComponents: main\nSigned-By:\n' "$SITE_URL"
	sed -e 's/^$/./' -e 's/^/ /' "$site/keys/$key_name.asc"
} >"$site/apt/$key_name.sources"

# RPM: one repository for every architecture. dnf and zypper pick by architecture.
createrepo_c "$site/rpm"
sign --armor --detach-sign --output "$site/rpm/repodata/repomd.xml.asc" "$site/rpm/repodata/repomd.xml"
cat >"$site/rpm/$key_name.repo" <<EOF
[$key_name]
name=TheSlopMachine
type=rpm-md
baseurl=$SITE_URL/rpm
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=$SITE_URL/keys/$key_name.asc
EOF

# APK: one index per architecture directory, built with Alpine's own tools.
docker run --rm \
	--volume "$(realpath "$site/apk"):/repository" \
	--volume "$work/apk-keys:/keys:ro" \
	alpine:3.22 sh -euc '
		apk add --no-cache abuild
		for directory in /repository/*/; do
			cd "$directory"
			apk index --allow-untrusted --output APKINDEX.tar.gz *.apk
			abuild-sign -k /keys/'"$key_name"'.rsa APKINDEX.tar.gz
		done
		chown -R '"$(id -u):$(id -g)"' /repository
	'
