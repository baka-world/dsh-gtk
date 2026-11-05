#!/usr/bin/env bash
# Build the Linux packages on this machine and attach them to the release, for
# when CI cannot run (the Release workflow needs GitHub Actions).
#
#   scripts/release-local.sh [--publish] [--version x.y.z]
#
# It takes the same steps the workflow takes for Linux: `mygo build`, then
# scripts/make-rpm.sh, then uploads the deb, rpm, tarball and install.sh to the
# release of the tag. The arm64 packages are not built here: the bundled runtime
# is platform specific and there is no cross build, so CI still has to fill
# them in.
#
# Needs rpmbuild (dnf install rpm-build) and gh, authenticated for the repo.
set -euo pipefail

cd "$(dirname "$0")/.."

publish=0
version=""
while [ $# -gt 0 ]; do
	case "$1" in
		--publish) publish=1; shift ;;
		--version) version="$2"; shift 2 ;;
		-h|--help) sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
		*) echo "release-local: unknown argument: $1" >&2; exit 2 ;;
	esac
done

if [ -z "$version" ]; then
	version="$(sed -n 's/^[[:space:]]*"version":[[:space:]]*"\(.*\)".*/\1/p' mygo.json | head -1)"
fi
[ -n "$version" ] || { echo "release-local: mygo.json has no version" >&2; exit 1; }
tag="v$version"
app="dist/linux-amd64"

command -v gh >/dev/null 2>&1 || { echo "release-local: gh is missing" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "release-local: go is missing" >&2; exit 1; }

echo "==> mygo build $version"
go tool mygo build

echo "==> rpm"
scripts/make-rpm.sh

echo "==> release $tag"
if ! gh release view "$tag" >/dev/null 2>&1; then
	gh release create "$tag" --draft --verify-tag --title "$tag" --generate-notes
fi
gh release upload "$tag" "$app"/*.deb "$app"/*.rpm "$app"/*.tar.gz "$app/install.sh" --clobber

if [ "$publish" = 1 ]; then
	gh release edit "$tag" --draft=false
	echo "published: $(gh release view "$tag" --json url --jq .url)"
else
	echo "draft:     $(gh release view "$tag" --json url --jq .url)"
	echo "publish:   gh release edit $tag --draft=false"
fi
