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

fix:
	go fix ./...

fmt:
	go fmt ./...

generate:
	go generate ./...

license:
	(which $(GOPATH)/bin/addlicense || go get github.com/google/addlicense)
	$(GOPATH)/bin/addlicense  -y 2020 -c "The Kubernetes Authors." -f LICENSE_TEMPLATE .

verify-license:
	(which $(GOPATH)/bin/addlicense || go get github.com/google/addlicense)
	$(GOPATH)/bin/addlicense  -check .

tidy:
	go mod tidy

lint:
	(which $(GOPATH)/bin/golangci-lint || go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.19.1)
	$(GOPATH)/bin/golangci-lint run ./...

test:
	go test -cover ./...

vet:
	go vet ./...

build:
	go build -o bin/kapply sigs.k8s.io/cli-utils/cmd;
	mv bin/kapply $(MYGOBIN)

.PHONY: verify-kapply-e2e
verify-kapply-e2e: test-examples-e2e-kapply

$(MYGOBIN)/mdrip:
	go install github.com/monopole/mdrip

.PHONY:
test-examples-e2e-kapply: $(MYGOBIN)/mdrip $(MYGOBIN)/kind
	( \
		set -e; \
		/bin/rm -f bin/kapply; \
		/bin/rm -f $(MYGOBIN)/kapply; \
		echo "Installing kapply from ."; \
		make build; \
		./hack/testExamplesE2EAgainstKapply.sh .; \
	)

$(MYGOBIN)/kind:
	( \
        set -e; \
        d=$(shell mktemp -d); cd $$d; \
        wget -O ./kind https://github.com/kubernetes-sigs/kind/releases/download/v0.7.0/kind-$(shell uname)-amd64; \
        chmod +x ./kind; \
        mv ./kind $(MYGOBIN); \
        rm -rf $$d; \
	)

.PHONY: nuke
nuke: clean
	sudo rm -rf $(shell go env GOPATH)/pkg/mod/sigs.k8s.io
