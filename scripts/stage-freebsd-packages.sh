#!/usr/bin/env bash
# Stages FreeBSD package trees for every record in releases/. Runs on Linux.
# scripts/build-freebsd-repositories.sh turns the trees into signed repositories inside FreeBSD.
#
# Usage: scripts/stage-freebsd-packages.sh <work-directory> <site-directory>
# Environment: SITE_URL  public URL of the site, without a trailing slash
# Tools: jq, curl, unzip, sha256sum.
set -euo pipefail

work=${1:?usage: stage-freebsd-packages.sh <work-directory> <site-directory>}
site=${2:?usage: stage-freebsd-packages.sh <work-directory> <site-directory>}
: "${SITE_URL:?SITE_URL is not set}"

readonly key_name=theslopmachine
readonly maintainer="TheSlopMachine@users.noreply.github.com"

# pkg ABI, release platform. Every supported FreeBSD major gets its own repository.
readonly freebsd_targets=(
	"FreeBSD:14:amd64 freebsd_amd64"
	"FreeBSD:14:aarch64 freebsd_arm64"
	"FreeBSD:14:i386 freebsd_386"
	"FreeBSD:15:amd64 freebsd_amd64"
	"FreeBSD:15:aarch64 freebsd_arm64"
)

shopt -s nullglob
records=(releases/*.json)
if ((${#records[@]} == 0)); then
	echo "no release records in releases/" >&2
	exit 1
fi

mkdir -p "$work/archives" "$site"
for record in "${records[@]}"; do
	name=$(jq -r .name "$record")
	binary=$(jq -r .binary "$record")
	version=$(jq -r .version "$record")
	for target in "${freebsd_targets[@]}"; do
		read -r abi platform <<<"$target"
		url=$(jq -r --arg platform "$platform" '.assets[$platform].url // empty' "$record")
		if [[ -z $url ]]; then
			echo "$name $version: no $platform archive"
			continue
		fi
		sha256=$(jq -r --arg platform "$platform" '.assets[$platform].sha256' "$record")

		archive="$work/archives/$name-$platform.zip"
		if [[ ! -f $archive ]]; then
			curl -fsSL --retry 3 -o "$archive" "$url"
			echo "$sha256  $archive" | sha256sum --check --quiet -
		fi

		directory="$work/$abi/$name"
		stage="$directory/stage/usr/local"
		mkdir -p "$stage/bin" "$stage/share/$name"
		unzip -q -o "$archive" "$binary" -d "$stage/bin"
		chmod 755 "$stage/bin/$binary"
		# Apps read the marker to refuse a self-update.
		printf 'pkg\n' >"$stage/share/$name/package-manager"
		{
			printf '@owner root\n@group wheel\n@mode 0755\nbin/%s\n' "$binary"
			printf '@mode 0644\nshare/%s/package-manager\n' "$name"
		} >"$directory/plist"

		services="services/$name/freebsd"
		post_install=""
		if [[ $(jq '.service != null' "$record") == true ]]; then
			rc_name=${name//-/_}
			configuration_file=$(jq -r .service.configuration_file "$record")
			mkdir -p "$stage/etc/rc.d" "$stage/etc/$name"
			install -m 755 "$services/$rc_name" "$stage/etc/rc.d/$rc_name"
			install -m 640 "$services/configuration" "$stage/etc/$name/$configuration_file"
			# @config keeps local edits of the configuration across upgrades.
			printf '@mode 0755\netc/rc.d/%s\n@mode 0640\n@config etc/%s/%s\n' "$rc_name" "$name" "$configuration_file" >>"$directory/plist"
			post_install="$services/post-install.sh"
		fi

		jq -n \
			--slurpfile release "$record" \
			--arg abi "$abi" \
			--arg maintainer "$maintainer" \
			--rawfile post_install "${post_install:-/dev/null}" \
			'$release[0] as $release | {
				name: $release.name,
				origin: ("theslopmachine/" + $release.name),
				version: $release.version,
				comment: $release.description,
				desc: $release.description,
				www: $release.homepage,
				maintainer: $maintainer,
				prefix: "/usr/local",
				abi: $abi,
				licenselogic: "single",
				licenses: [$release.license],
				categories: ["sysutils"]
			} + (if $post_install == "" then {} else {scripts: {"post-install": $post_install}} end)' >"$directory/+MANIFEST"
	done
done
rm -rf "$work/archives"

cat >"$site/$key_name.conf" <<EOF
$key_name: {
  url: "$SITE_URL/freebsd/\${ABI}",
  signature_type: "pubkey",
  pubkey: "/usr/local/etc/pkg/repos/$key_name.rsa.pub",
  enabled: yes
}
EOF
