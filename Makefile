# Copyright 2019 The Kubernetes Authors.
# SPDX-License-Identifier: Apache-2.0

GOPATH := $(shell go env GOPATH)
MYGOBIN := $(shell go env GOPATH)/bin
# Tools are versioned in hack/tools/go.mod and run with $(GO_TOOL) <tool>.
# The path is absolute and exported, so go:generate directives and scripts can use it too.
export TOOLS_MODFILE := $(CURDIR)/hack/tools/go.mod
GO_TOOL := go tool -modfile=$(TOOLS_MODFILE)
SHELL := /bin/bash
export PATH := $(MYGOBIN):$(PATH)
# Use the Ginkgo CLI version from go.mod, so it always matches the library.
GINKGO := go run github.com/onsi/ginkgo/v2/ginkgo

.PHONY: all
all: generate license fix vet fmt test lint tidy

"$(MYGOBIN)/deepcopy-gen":
	go install k8s.io/code-generator/cmd/deepcopy-gen@v0.25.2

# The following target intended for reference by a file in
# https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/cli-utils
.PHONY: prow-presubmit-check
prow-presubmit-check: \
	test lint verify-license

.PHONY: prow-presubmit-check-e2e
prow-presubmit-check-e2e: \
	install-column-apt test-e2e verify-kapply-e2e

.PHONY: prow-presubmit-check-stress
prow-presubmit-check-stress: \
	test-stress

.PHONY: fix
fix:
	go fix ./...

.PHONY: fmt
fmt:
	go fmt ./...

# Install column (required by verify-kapply-e2e)
# Update is included because the kubekins-e2e container build strips out the package cache.
# In newer versions of debian, column is in the bsdextrautils package,
# but in buster (used by kubekins-e2e) it's in bsdmainutils.
.PHONY: install-column-apt
install-column-apt:
	apt-get update
	apt-get install -y bsdmainutils

.PHONY: generate-deepcopy
generate-deepcopy: "$(MYGOBIN)/deepcopy-gen"
	hack/run-in-gopath.sh deepcopy-gen --input-dirs ./pkg/apis/... -O zz_generated.deepcopy --go-header-file ./LICENSE_TEMPLATE_GO

.PHONY: generate
generate: generate-deepcopy
	go generate ./...

.PHONY: license
license:
	$(GO_TOOL) addlicense -v -y 2021 -c "The Kubernetes Authors." -f LICENSE_TEMPLATE .

.PHONY: verify-license
verify-license:
	$(GO_TOOL) addlicense -check .

.PHONY: tidy
tidy:
	go mod tidy
	go -C hack/tools mod tidy

.PHONY: lint
lint:
	$(GO_TOOL) golangci-lint run ./...

.PHONY: test
test:
	go test -race -cover ./cmd/... ./pkg/...

.PHONY: test-e2e
test-e2e:
	$(GO_TOOL) kind delete cluster --name=cli-utils-e2e && $(GO_TOOL) kind create cluster --name=cli-utils-e2e --wait 5m
	$(GINKGO) -v ./test/e2e/... -- -v 3

.PHONY: test-e2e-focus
test-e2e-focus:
	$(GO_TOOL) kind delete cluster --name=cli-utils-e2e && $(GO_TOOL) kind create cluster --name=cli-utils-e2e --wait 5m
	$(GINKGO) -v -focus ".*$(FOCUS).*" ./test/e2e/... -- -v 5

.PHONY: test-stress
test-stress:
	$(GO_TOOL) kind delete cluster --name=cli-utils-e2e && $(GO_TOOL) kind create cluster --name=cli-utils-e2e --wait 5m \
		--config=./test/stress/kind-cluster.yaml
	kubectl wait nodes --for=condition=ready --all --timeout=5m
	$(GINKGO) -v ./test/stress/... -- -v 3

.PHONY: vet
vet:
	go vet ./...

.PHONY: build
build:
	go build -o bin/kapply sigs.k8s.io/cli-utils/cmd;
	mv bin/kapply "$(MYGOBIN)"

.PHONY: build-with-race-detector
build-with-race-detector:
	go build -race -o bin/kapply sigs.k8s.io/cli-utils/cmd;
	mv bin/kapply "$(MYGOBIN)"

.PHONY: verify-kapply-e2e
verify-kapply-e2e: test-examples-e2e-kapply

.PHONY: test-examples-e2e-kapply
test-examples-e2e-kapply:
	( \
		set -e; \
		/bin/rm -f bin/kapply; \
		/bin/rm -f "$(MYGOBIN)/kapply"; \
		echo "Installing kapply from ."; \
		make build-with-race-detector; \
		./hack/testExamplesE2EAgainstKapply.sh .; \
	)

.PHONY: nuke
nuke:
	sudo rm -rf "$(GOPATH)/pkg/mod/sigs.k8s.io"
