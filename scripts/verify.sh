#!/usr/bin/env bash

#  Copyright 2019 The Kubernetes Authors.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

set -e

source $(dirname ${BASH_SOURCE})/common.sh

header_text "running go vet"

go vet ./internal/... ./pkg/... ./cmd/...

header_text "populating vendor for gometalinter.v2"

go mod vendor

header_text "running gometalinter.v2"

gometalinter.v2 -e $(go env GOROOT) -e vendor/ -e _gen.go --disable-all \
    --deadline 15m \
    --enable=misspell \
    --enable=structcheck \
    --enable=golint \
    --enable=deadcode \
    --enable=errcheck \
    --enable=varcheck \
    --enable=goconst \
    --enable=goimports \
    --enable=gocyclo \
    --cyclo-over=7 \
    --line-length=120 \
    --enable=lll \
    --enable=nakedret \
    --enable=unparam \
    --enable=ineffassign \
    --enable=interfacer \
    --dupl-threshold=400 \
    --enable=dupl \
    --enable=misspell \
    ./pkg/... ./internal/... ./cmd/...

