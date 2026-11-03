#!/usr/bin/env bash
# Build an RPM from the tree `mygo build` wrote, with rpmbuild.
#
#   scripts/make-rpm.sh [--app-dir DIR] [--arch amd64|arm64]
#
# Needs rpmbuild: `dnf install rpm-build` on Fedora, `apt-get install rpm` on
# Debian and Ubuntu. The package installs the app and its bundled runtime
# under /opt/<slug>, the command in /usr/bin, and the usual desktop entry and
# icons; it is not a distribution package.
set -euo pipefail

cd "$(dirname "$0")/.."
root=$PWD
app_dir=""
arch=""
rpmbuild_bin="${RPMBUILD:-rpmbuild}"

usage() {
	sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
	case "$1" in
		--app-dir) app_dir="$2"; shift 2 ;;
		--arch) arch="$2"; shift 2 ;;
		-h|--help) usage; exit 0 ;;
		*) echo "make-rpm: unknown argument: $1" >&2; exit 2 ;;
	esac
done

# mygo.json holds the strings the package metadata comes from.
field() {
	sed -n "s/^[[:space:]]*\"$1\":[[:space:]]*\"\(.*\)\".*/\1/p" mygo.json | head -1
}

name="$(field slug)"
[ -n "$name" ] || name=$(basename "$root")
version="$(field version)"
summary="$(field comment)"
[ -n "$summary" ] || summary="$(field name)"
license=$(sed -n 's/^[[:space:]]*"license":[[:space:]]*"\(.*\)".*/\1/p' package.json 2>/dev/null | head -1)
[ -n "$license" ] || license="LicenseRef-custom"

if [ -z "$arch" ]; then
	arch="$(go env GOARCH)"
fi
case "$arch" in
	amd64) rpm_arch=x86_64 ;;
	arm64) rpm_arch=aarch64 ;;
	*) echo "make-rpm: unsupported architecture $arch" >&2; exit 2 ;;
esac
[ -n "$app_dir" ] || app_dir="dist/linux-$arch"

if [ ! -x "$app_dir/$name" ]; then
	echo "make-rpm: $app_dir/$name is missing; run 'make build' first" >&2
	exit 1
fi
if ! command -v "$rpmbuild_bin" >/dev/null 2>&1; then
	echo "make-rpm: rpmbuild is missing; install rpm-build (Fedora) or rpm (Debian/Ubuntu)" >&2
	exit 1
fi

top=$root/dist/.rpmbuild
rm -rf "$top"
mkdir -p "$top"/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS,tmp}

# Our own desktop entry: MyGo's has Type, Name, Exec, Icon and Categories
# only, and the shell groups a window with its entry by StartupWMClass.
desktop=$top/SPECS/$name.desktop
sed \
	-e "s|@NAME@|$(field name)|g" \
	-e "s|@SUMMARY@|$summary|g" \
	-e "s|@BINARY@|$name|g" \
	-e "s|@SLUG@|$name|g" \
	packaging/desktop/dsh-gtk.desktop.in >"$desktop"

spec=$top/SPECS/$name.spec
sed \
	-e "s|@NAME@|$name|g" \
	-e "s|@VERSION@|$version|g" \
	-e "s|@SUMMARY@|$summary|g" \
	-e "s|@LICENSE@|$license|g" \
	-e "s|@DESCRIPTION@|$(field name) shows DeepSeek Harness in a native window.|g" \
	-e "s|@APPDIR@|$root/$app_dir|g" \
	-e "s|@BINARY@|$name|g" \
	-e "s|@DESKTOP@|$desktop|g" \
	-e "s|@ICON256@|$root/$app_dir/$name-256x256.png|g" \
	-e "s|@ICON512@|$root/$app_dir/$name-512x512.png|g" \
	-e "s|@LICENSEFILE@|$root/LICENSE|g" \
	packaging/rpm/dsh-gtk.spec.in >"$spec"

# rpmbuild scripts through %{_tmppath}, /var/tmp by default: keep them in the
# build tree, where a container may add a read-only /var/tmp. The file check
# is part of rpm-build; a trimmed installation may not have it, and the
# package lists its files explicitly.
export TMPDIR="$top/tmp"
defines=(--define "_topdir $top" --define "_tmppath $top/tmp")
if [ ! -x /usr/lib/rpm/check-files ]; then
	defines+=(--define "__check_files %{nil}")
fi
"$rpmbuild_bin" -bb "${defines[@]}" --target "$rpm_arch" "$spec"

rpm=$(find "$top/RPMS/$rpm_arch" -name '*.rpm' -type f | head -1)
if [ -z "$rpm" ]; then
	echo "make-rpm: rpmbuild wrote no package" >&2
	exit 1
fi
mv "$rpm" "$app_dir/"
echo "$app_dir/$(basename "$rpm")"
