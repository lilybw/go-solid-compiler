#!/usr/bin/env bash
# Run a command inside the dev image.
#
# CI uses this rather than `docker compose run` because the workflow builds the
# image with buildx (for layer caching) and needs the module cache to live in a
# path the actions/cache step can see. Locally, prefer:
#
#   docker compose run --rm check
#
# Usage: ./ci-run.sh <go-version> <command...>
set -euo pipefail

GO_VERSION="${1:?usage: ci-run.sh <go-version> <command...>}"
shift

mkdir -p .cache/gomod .cache/gobuild

docker run --rm \
  --volume "$PWD:/workspace" \
  --volume "$PWD/.cache/gomod:/go/pkg/mod" \
  --volume "$PWD/.cache/gobuild:/go/cache" \
  --workdir /workspace \
  --user "$(id -u):$(id -g)" \
  --env GOTOOLCHAIN=local \
  --env GOFLAGS=-buildvcs=false \
  --env GOMODCACHE=/go/pkg/mod \
  --env GOCACHE=/go/cache \
  --env npm_config_cache=/go/cache/npm \
  "go-solid-compiler-dev:${GO_VERSION}" \
  bash -euo pipefail -c "$*"
