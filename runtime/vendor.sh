#!/usr/bin/env bash
# Fetch the pinned solid-js release and extract its ESM build into dist/.
#
# This runs at development time only, never at build or run time, and needs
# nothing but curl and tar. It deliberately does not use npm: the point of
# embedding is that consumers need no Node toolchain, and that guarantee is
# weaker if maintaining the embed requires one.
set -euo pipefail

VERSION="${SOLID_VERSION:-1.9.14}"
DIST="$(cd "$(dirname "$0")" && pwd)/dist"

echo "vendoring solid-js ${VERSION}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

curl -fsSL "https://registry.npmjs.org/solid-js/-/solid-js-${VERSION}.tgz" \
  -o "$work/solid.tgz"
tar -xzf "$work/solid.tgz" -C "$work"

rm -rf "$DIST"
mkdir -p "$DIST"

# The ESM builds are what a bundler should consume; the CJS ones would drag in
# require() and defeat tree shaking.
cp "$work/package/dist/solid.js"       "$DIST/solid.js"
cp "$work/package/web/dist/web.js"     "$DIST/web.js"
cp "$work/package/store/dist/store.js" "$DIST/store.js"
cp "$work/package/html/dist/html.js"   "$DIST/html.js" 2>/dev/null || true
cp "$work/package/h/dist/h.js"         "$DIST/h.js"     2>/dev/null || true
cp "$work/package/LICENSE"             "$DIST/LICENSE"

# Keep the recorded version and the fetched one from drifting apart.
sed -i.bak "s/^const Version = .*/const Version = \"${VERSION}\"/" \
  "$(dirname "$DIST")/runtime.go" && rm -f "$(dirname "$DIST")/runtime.go.bak"

echo "vendored:"
ls -la "$DIST"
