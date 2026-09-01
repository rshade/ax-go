# CI job image for ax-go's self-hosted runner. Published to public GHCR by
# .github/workflows/build-ci-image.yml on every main push that touches this
# file or mise.toml — the runner VM pulls it anonymously and never holds
# registry credentials. Tags: latest + mise-<first 12 of sha256(mise.toml)>,
# so a checkout can tell whether the image predates its pins.
#
# A stale image degrades to a slower job, not a red one: workflows run
# `mise install`, which downloads whatever a newer mise.toml added.
FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git make build-essential jq unzip zip zstd xz-utils \
    gnupg \
    && rm -rf /var/lib/apt/lists/*

ENV MISE_VERSION=v2026.8.15
RUN curl -fsSL https://mise.run | MISE_INSTALL_PATH=/usr/local/bin/mise sh

# Everything mise needs lives outside $HOME and is wired through env: the
# Actions runner overrides HOME to /github/home inside job containers, so a
# home-relative data dir or trust state silently vanishes at job time. /__w
# is the runner's workspace mount, trusted so the checked-out mise.toml
# resolves without a prompt. specify-cli is a dev-time spec tool no workflow
# invokes; disabling it keeps python/pipx out of the image.
ENV MISE_DATA_DIR=/opt/mise \
    MISE_GLOBAL_CONFIG_FILE=/opt/ci/mise.toml \
    MISE_DISABLE_TOOLS=pipx:specify-cli \
    MISE_TRUSTED_CONFIG_PATHS=/__w:/opt/ci \
    PATH=/opt/mise/shims:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

COPY mise.toml /opt/ci/mise.toml
RUN cd /opt/ci && mise trust mise.toml && mise install && mise reshim

# gh: .github/apidiff-comment.sh upserts the API-diff PR comment with gh + jq.
# Installed after the mise layer so pin-bump rebuilds reuse the toolchain
# cache.
RUN mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        -o /etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# The runner owns the workspace as uid 1000; the container runs as root. Git
# refuses that mismatch as "dubious ownership", which surfaces as go's
# "error obtaining VCS status" and would break go-apidiff's in-place base
# checkout. System-level (not global) so it survives the HOME override.
RUN git config --system --add safe.directory '*'
