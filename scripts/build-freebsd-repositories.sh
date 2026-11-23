#!/bin/sh
# Builds one signed pkg repository per ABI from the trees that
# scripts/stage-freebsd-packages.sh staged. Runs on FreeBSD.
#
# Usage: scripts/build-freebsd-repositories.sh <work-directory> <site-directory> <rsa-private-key>
set -eu

work=${1:?usage: build-freebsd-repositories.sh <work-directory> <site-directory> <rsa-private-key>}
site=${2:?usage: build-freebsd-repositories.sh <work-directory> <site-directory> <rsa-private-key>}
key=${3:?usage: build-freebsd-repositories.sh <work-directory> <site-directory> <rsa-private-key>}

built=0
for abi_directory in "$work"/FreeBSD:*; do
	[ -d "$abi_directory" ] || continue
	abi=$(basename "$abi_directory")
	repository="$site/$abi"
	mkdir -p "$repository/All"
	for package_directory in "$abi_directory"/*/; do
		pkg create \
			-M "$package_directory/+MANIFEST" \
			-p "$package_directory/plist" \
			-r "$package_directory/stage" \
			-o "$repository/All"
	done
	for package in "$repository"/All/*.pkg; do
		package_abi=$(pkg query -F "$package" '%q')
		if [ "$package_abi" != "$abi" ]; then
			echo "$package has ABI $package_abi, expected $abi" >&2
			exit 1
		fi
	done
	pkg repo "$repository" "$key"
	built=$((built + 1))
done

if [ "$built" -eq 0 ]; then
	echo "no staged ABI directories in $work" >&2
	exit 1
fi
