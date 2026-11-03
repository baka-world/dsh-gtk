#!/usr/bin/env bash
# Bundle the dsh runtime, a Node interpreter and npm into resources/runtime.
#
#   scripts/prepare-runtime.sh [--version 0.2.2-alpha.3] [--node-version 24.11.0] [--no-node]
#
# `mygo build` copies resources/ into the app, so the bundle is what makes
# the packaged app run without a global dsh or node installation. The bundled
# npm lets `dsh-go runtime install` update dsh without the app.
set -euo pipefail

cd "$(dirname "$0")/.."

DSH_VERSION="${DSH_VERSION:-0.2.2-alpha.3}"
NODE_VERSION="${NODE_VERSION:-24.11.0}"
DEST="${DSH_RUNTIME_DIR:-resources/runtime}"
WITH_NODE=1

usage() {
  sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version) DSH_VERSION="$2"; shift 2 ;;
    --node-version) NODE_VERSION="$2"; shift 2 ;;
    --no-node) WITH_NODE=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "prepare-runtime: unknown argument: $1" >&2; exit 2 ;;
  esac
done

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "prepare-runtime: unsupported OS $(uname -s); bundle node manually as $DEST/node" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=x64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "prepare-runtime: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

command -v npm >/dev/null || { echo "prepare-runtime: npm not found" >&2; exit 1; }
command -v curl >/dev/null || { echo "prepare-runtime: curl not found" >&2; exit 1; }
command -v tar >/dev/null || { echo "prepare-runtime: tar not found" >&2; exit 1; }

mkdir -p "$DEST"

# 1. The dsh tree.
if [ -f "$DEST/node_modules/@deepseek-ai/dsh/lib/bin.js" ] \
  && [ "$(cat "$DEST/.dsh-version" 2>/dev/null || true)" = "$DSH_VERSION" ]; then
  echo "dsh $DSH_VERSION is already bundled in $DEST"
else
  echo "installing @deepseek-ai/dsh@$DSH_VERSION into $DEST (this takes a few minutes)"
  printf '{\n  "name": "dsh-go-runtime",\n  "private": true\n}\n' > "$DEST/package.json"
  rm -rf "$DEST/node_modules"
  npm install --prefix "$DEST" --omit=dev --no-audit --no-fund "@deepseek-ai/dsh@$DSH_VERSION"
  printf '%s\n' "$DSH_VERSION" > "$DEST/.dsh-version"
fi

# 2. Node and npm from the official distribution. npm installs runtime
#    updates; it runs on the bundled node.
need_node=0
if [ "$WITH_NODE" = 1 ] && [ ! -x "$DEST/node" ]; then
  need_node=1
fi
need_npm=0
if [ ! -f "$DEST/npm/bin/npm-cli.js" ]; then
  need_npm=1
fi

if [ "$need_node" = 1 ] || [ "$need_npm" = 1 ]; then
  name="node-v$NODE_VERSION-$os-$arch"
  echo "downloading node v$NODE_VERSION ($os-$arch)"
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  curl -fsSL "https://nodejs.org/dist/v$NODE_VERSION/$name.tar.xz" -o "$tmp/$name.tar.xz"
  curl -fsSL "https://nodejs.org/dist/v$NODE_VERSION/SHASUMS256.txt" -o "$tmp/SHASUMS256.txt"
  expected=$(awk -v f="$name.tar.xz" '$2 == f { print $1 }' "$tmp/SHASUMS256.txt")
  [ -n "$expected" ] || { echo "prepare-runtime: no checksum for $name.tar.xz" >&2; exit 1; }
  if command -v sha256sum >/dev/null; then
    actual=$(sha256sum "$tmp/$name.tar.xz" | awk '{print $1}')
  else
    actual=$(shasum -a 256 "$tmp/$name.tar.xz" | awk '{print $1}')
  fi
  [ "$actual" = "$expected" ] || { echo "prepare-runtime: checksum mismatch for $name.tar.xz" >&2; exit 1; }
  tar -xJf "$tmp/$name.tar.xz" -C "$tmp"
  if [ "$need_node" = 1 ]; then
    cp "$tmp/$name/bin/node" "$DEST/node"
    chmod +x "$DEST/node"
    cp "$tmp/$name/LICENSE" "$DEST/NODE-LICENSE" 2>/dev/null || true
  fi
  if [ "$need_npm" = 1 ]; then
    rm -rf "$DEST/npm"
    cp -R "$tmp/$name/lib/node_modules/npm" "$DEST/npm"
  fi
fi

# 3. The manifest the app reads for versions.
node_version=""
if [ -x "$DEST/node" ]; then
  node_version=$("$DEST/node" --version | sed 's/^v//')
fi
dsh_version=$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' \
  "$DEST/node_modules/@deepseek-ai/dsh/package.json" 2>/dev/null | head -1)
[ -n "$dsh_version" ] || dsh_version="$DSH_VERSION"
cat > "$DEST/runtime.json" <<EOF
{
  "dsh": "$dsh_version",
  "node": "$node_version",
  "source": "bundle",
  "installedAt": "$(date -u +%FT%TZ)"
}
EOF

echo
echo "runtime: $DEST ($(du -sh "$DEST" | cut -f1))"
if [ -x "$DEST/node" ]; then
  echo "node:    $("$DEST/node" --version)"
else
  echo "node:    not bundled; the app falls back to node on PATH"
fi
if [ -f "$DEST/npm/bin/npm-cli.js" ]; then
  echo "npm:     $("$DEST/node" "$DEST/npm/bin/npm-cli.js" --version 2>/dev/null || echo bundled)"
else
  echo "npm:     not bundled; runtime installs need npm on PATH"
fi
echo "dsh:     $("$DEST/node" "$DEST/node_modules/@deepseek-ai/dsh/lib/bin.js" --version 2>/dev/null \
  || node "$DEST/node_modules/@deepseek-ai/dsh/lib/bin.js" --version 2>/dev/null || echo "unknown")"
