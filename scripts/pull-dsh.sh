#!/usr/bin/env bash
# Pull the DeepSeek Harness source this shell bundles, at the release tag
# whose npm build scripts/prepare-runtime.sh installs.
#
#   scripts/pull-dsh.sh [tag]     (default: dsh-v0.2.2-alpha.3)
#
# A shallow checkout into third_party/deepseek-harness, which Git ignores:
# the app runs the npm build, not this tree.
set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:-dsh-v0.2.2-alpha.3}"
DIR=third_party/deepseek-harness
URL=https://github.com/deepseek-ai/deepseek-harness.git

mkdir -p third_party
if [ -d "$DIR/.git" ]; then
  git -C "$DIR" fetch --depth 1 origin "refs/tags/$TAG"
  git -C "$DIR" checkout -q --detach FETCH_HEAD
else
  git clone --depth 1 --branch "$TAG" "$URL" "$DIR"
fi

echo "dsh source: $DIR at $TAG ($(git -C "$DIR" rev-parse --short HEAD))"
