# Development and test environment for go-solid-compiler.
#
# This image exists so that "it works on my machine" stops being a variable.
# The project spans two toolchains — Go for the compiler, Node for the
# reference implementation the differential harness compares against — and
# pins an exact TypeScript compiler fork. Reproducing that by hand on Windows,
# macOS, and Linux is exactly the kind of setup that silently drifts.
#
# The same image runs locally and in CI, so a green pipeline and a green
# checkout mean the same thing.

ARG GO_VERSION=1.27

FROM golang:${GO_VERSION}-bookworm AS base

# Node is needed only by the differential harness, to run babel-preset-solid
# and record the reference output. It is deliberately absent from the runtime
# story — the whole point of the project is that building a Solid app needs no
# Node — but the reference implementation is the definition of correct, so it
# stays available for verification.
ARG NODE_MAJOR=22
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl gnupg git \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
        | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
        > /etc/apt/sources.list.d/nodesource.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# A non-root user whose ids can be matched to the host's. On Linux a container
# running as root leaves root-owned files in the bind-mounted source tree,
# which is a genuinely irritating way to lose an afternoon. Docker Desktop on
# macOS and Windows remaps ownership itself, so the defaults are harmless there.
ARG UID=1000
ARG GID=1000
RUN if ! getent group ${GID} >/dev/null; then groupadd -g ${GID} dev; fi \
    && if ! getent passwd ${UID} >/dev/null; then \
         useradd -m -u ${UID} -g ${GID} -s /bin/bash dev; \
       fi \
    && mkdir -p /go/pkg/mod /go/cache /workspace/harness/babel/node_modules \
    && chown -R ${UID}:${GID} /go /workspace

# Every path above is a mount target for a named volume, and each one must
# exist in the image with the right ownership.
#
# Docker seeds a fresh named volume from whatever the image has at that exact
# path, ownership included. If the directory is missing, the volume is created
# owned by root, and this image runs as a non-root user — so the failure is a
# permission error on first write:
#
#   go: writing stat cache: mkdir /go/pkg/mod/cache: permission denied
#   npm ERR! EACCES  .../node_modules
#
# Note the exact paths: the volume mounts at /go/pkg/mod, not /go/pkg, so
# creating the parent is not enough. docker-compose.yml lists the mount targets;
# they must match this line.
#
# Changing ownership here only affects volumes created afterwards. An existing
# volume keeps its original ownership, so after editing this line run
# `docker compose down -v` to discard the old ones.

ENV GOPATH=/go \
    GOMODCACHE=/go/pkg/mod \
    GOCACHE=/go/cache \
    GOFLAGS=-buildvcs=false \
    CGO_ENABLED=1 \
    npm_config_cache=/go/cache/npm

USER ${UID}:${GID}
WORKDIR /workspace

# Sanity-check the toolchains at build time rather than on first use, so a bad
# version argument fails the build instead of a confusing test run.
RUN go version && node --version && npm --version

CMD ["bash"]
