#!/bin/sh
# bootstrap.sh — verify toolchains and install JS dependencies.
#
# Checks that required toolchains (pnpm, go, cargo) are present, then runs
# `pnpm install` to hydrate the workspace.
set -eu

log() { printf '[bootstrap] %s\n' "$*"; }
fail() { printf '[bootstrap] ERROR: %s\n' "$*" >&2; exit 1; }

need() {
    cmd=$1
    hint=$2
    if ! command -v "$cmd" >/dev/null 2>&1; then
        fail "'$cmd' not found in PATH — $hint"
    fi
    log "found $cmd: $(command -v "$cmd")"
}

need pnpm "install from https://pnpm.io (or corepack enable)"
need go   "install Go 1.23+ from https://go.dev/dl/"
need cargo "install Rust stable via https://rustup.rs"

log "running pnpm install"
pnpm install

log "bootstrap complete"
