# Copyright 2019 The Kubernetes Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Makefile for kapply CLI.

MYGOBIN := $(shell go env GOPATH)/bin
SHELL := /bin/bash
export PATH := $(MYGOBIN):$(PATH)

.PHONY: verify-kapply-e2e
verify-kapply-e2e: test-examples-e2e-kapply

$(MYGOBIN)/mdrip:
	go install github.com/monopole/mdrip

.PHONY:
test-examples-e2e-kapply: $(MYGOBIN)/mdrip $(MYGOBIN)/kind
	( \
		set -e; \
		/bin/rm -f $(MYGOBIN)/kapply; \
		echo "Installing kapply from ."; \
		cd cmd/kapply/; go install .; cd ../..; \
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
