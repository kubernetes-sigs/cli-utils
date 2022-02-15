# Copyright 2019 The Kubernetes Authors.
# SPDX-License-Identifier: Apache-2.0

.PHONY: generate license fix vet fmt test lint tidy openapi

GOPATH := $(shell go env GOPATH)
MYGOBIN := $(shell go env GOPATH)/bin
SHELL := /bin/bash
export PATH := $(MYGOBIN):$(PATH)

all: generate license fix vet fmt test lint tidy

# The following target intended for reference by a file in
# https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/cli-utils
.PHONY: prow-presubmit-check
prow-presubmit-check: \
	test lint verify-license

.PHONY: prow-presubmit-check-e2e
prow-presubmit-check-e2e: \
	test-e2e verify-kapply-e2e

fix:
	go fix ./...

fmt:
	go fmt ./...

install-stringer:
	(which $(GOPATH)/bin/stringer || go install golang.org/x/tools/cmd/stringer@v0.1.5)

install-addlicense:
	(which $(GOPATH)/bin/addlicense || go install github.com/google/addlicense@v1.0.0)

install-lint:
	(which $(GOPATH)/bin/golangci-lint || go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.40.1)

install-deepcopy-gen:
	(which $(GOPATH)/bin/deepcopy-gen || go install k8s.io/code-generator/cmd/deepcopy-gen@v0.23.3)

generate-deepcopy: install-deepcopy-gen
	hack/run-in-gopath.sh deepcopy-gen --input-dirs ./pkg/apis/... -O zz_generated.deepcopy --go-header-file ./LICENSE_TEMPLATE_GO

generate: install-stringer generate-deepcopy
	go generate ./...

license: install-addlicense
	$(GOPATH)/bin/addlicense -v -y 2021 -c "The Kubernetes Authors." -f LICENSE_TEMPLATE .

verify-license: install-addlicense
	$(GOPATH)/bin/addlicense  -check .

tidy:
	go mod tidy

lint: install-lint
	$(GOPATH)/bin/golangci-lint run ./...

test:
	go test -race -cover ./cmd/... ./pkg/...

test-e2e: $(MYGOBIN)/ginkgo $(MYGOBIN)/kind
	kind delete cluster --name=cli-utils-e2e && kind create cluster --name=cli-utils-e2e
	$(GOPATH)/bin/ginkgo ./test/e2e/...

vet:
	go vet ./...

build:
	go build -o bin/kapply sigs.k8s.io/cli-utils/cmd;
	mv bin/kapply $(MYGOBIN)

build-with-race-detector:
	go build -race -o bin/kapply sigs.k8s.io/cli-utils/cmd;
	mv bin/kapply $(MYGOBIN)

.PHONY: verify-kapply-e2e
verify-kapply-e2e: test-examples-e2e-kapply

$(MYGOBIN)/ginkgo:
	go install github.com/onsi/ginkgo/ginkgo@v1.16.2

$(MYGOBIN)/mdrip:
	go install github.com/monopole/mdrip@v1.0.2

.PHONY:
test-examples-e2e-kapply: $(MYGOBIN)/mdrip $(MYGOBIN)/kind
	( \
		set -e; \
		/bin/rm -f bin/kapply; \
		/bin/rm -f $(MYGOBIN)/kapply; \
		echo "Installing kapply from ."; \
		make build-with-race-detector; \
		./hack/testExamplesE2EAgainstKapply.sh .; \
	)

$(MYGOBIN)/kind:
	go install sigs.k8s.io/kind@v0.11.0

.PHONY: nuke
nuke: clean
	sudo rm -rf $(shell go env GOPATH)/pkg/mod/sigs.k8s.io
