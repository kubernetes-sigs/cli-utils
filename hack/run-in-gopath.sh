#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
# SPDX-License-Identifier: Apache-2.0

set -o errexit -o nounset -o pipefail -o posix

PKG_PATH="sigs.k8s.io/cli-utils"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"

# Make a new temporary GOPATH directory
export GOPATH=$(mktemp -d -t cli-utils-gopath.XXXXXXXXXX)
# Clean up on exit (modcache has read-only files, so clean that first)
trap "go clean -modcache && rm '${GOPATH}/src/${PKG_PATH}' && rm -rf '${GOPATH}'" EXIT

# Make sure we can read, write, and delete
chmod a+rw "${GOPATH}"

# Use a temporary cache
export GOCACHE="${GOPATH}/cache"

# Create a symlink for the local repo in the GOPATH
mkdir -p "${GOPATH}/src/${PKG_PATH}"
rm -r "${GOPATH}/src/${PKG_PATH}"
ln -s "${REPO_ROOT}" "${GOPATH}/src/${PKG_PATH}"

# Make sure our own Go binaries are in PATH.
export PATH="${GOPATH}/bin:${PATH}"

# Set GOROOT so binaries that parse code can work properly.
export GOROOT=$(go env GOROOT)

# Unset GOBIN in case it already exists in the current session.
unset GOBIN

# enter the GOPATH before executing the command
cd "${GOPATH}/src/${PKG_PATH}"

# Run the user-provided command.
"${@}"

# exit the GOPATH before deleting it
cd "${REPO_ROOT}"
