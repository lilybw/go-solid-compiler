#!/usr/bin/env bash
# Fetch the pinned solid-js release and extract its client ESM builds.
#
# Runs at development time only. It needs curl and tar, deliberately not npm:
# the point of embedding is that consumers need no Node toolchain, and that
# guarantee is weaker if maintaining the embed requires one.
#
# Only the client builds are taken. The server build imports seroval, and the
# storage build imports node:async_hooks; the client builds import nothing but
# each other, which is what makes embedding them self-contained.
set -euo pipefail

VERSION="${SOLID_VERSION:-1.9.14}"
DIST="$(cd "$(dirname "$0")" && pwd)/dist"

echo "vendoring solid-js ${VERSION}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

curl -fsSL "https://registry.npmjs.org/solid-js/-/solid-js-${VERSION}.tgz" -o "$work/solid.tgz"
tar -xzf "$work/solid.tgz" -C "$work"
pkg="$work/package"

rm -rf "$DIST"
mkdir -p "$DIST"

# Production and development builds of each entry point. The development ones
# carry the DEV warnings and named ownership that make Solid's runtime errors
# legible, so both are shipped and selected at bundle time.
copy() { # <source> <dest> <required>
    if [ -f "$pkg/$1" ]; then
        cp "$pkg/$1" "$DIST/$2"
    elif [ "$3" = "required" ]; then
        echo "missing required file: $1" >&2
        exit 1
    fi
}

copy dist/solid.js          solid.js        required
copy dist/dev.js            solid.dev.js    required
copy web/dist/web.js        web.js          required
copy web/dist/dev.js        web.dev.js      required
copy store/dist/store.js    store.js        required
copy store/dist/dev.js      store.dev.js    optional
copy html/dist/html.js      html.js         optional
copy h/dist/h.js            h.js            optional
copy LICENSE                LICENSE         required

# A client build that reaches outside the embedded set would break at bundle
# time with an unresolvable import, so catch it here instead.
if grep -ohE "from[[:space:]]*['\"][^'\"./][^'\"]*['\"]" "$DIST"/*.js \
     | tr -d "\"'" | sed 's/from[[:space:]]*//' | grep -v '^solid-js' | sort -u | grep .; then
    echo "error: a vendored file imports something outside solid-js (shown above)" >&2
    exit 1
fi

sed -i.bak "s/^const Version = .*/const Version = \"${VERSION}\"/" \
    "$(dirname "$DIST")/runtime.go" && rm -f "$(dirname "$DIST")/runtime.go.bak"

echo "vendored:"
ls -la "$DIST"
