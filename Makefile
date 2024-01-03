# Copyright 2019 The Kubernetes Authors.
# SPDX-License-Identifier: Apache-2.0

GOPATH := $(shell go env GOPATH)
MYGOBIN := $(shell go env GOPATH)/bin
SHELL := /bin/bash
export PATH := $(MYGOBIN):$(PATH)

.PHONY: all
all: generate license fix vet fmt test lint tidy

"$(MYGOBIN)/stringer":
	go install golang.org/x/tools/cmd/stringer@v0.12.0

"$(MYGOBIN)/addlicense":
	go install github.com/google/addlicense@v1.0.0

"$(MYGOBIN)/golangci-lint":
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.53.3

"$(MYGOBIN)/deepcopy-gen":
	go install k8s.io/code-generator/cmd/deepcopy-gen@v0.25.2

"$(MYGOBIN)/ginkgo":
	go install github.com/onsi/ginkgo/v2/ginkgo@v2.2.0

"$(MYGOBIN)/mdrip":
	go install github.com/monopole/mdrip@v1.0.2

"$(MYGOBIN)/kind":
	go install sigs.k8s.io/kind@v0.16.0

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
generate: "$(MYGOBIN)/stringer" generate-deepcopy
	go generate ./...

.PHONY: license
license: "$(MYGOBIN)/addlicense"
	"$(MYGOBIN)/addlicense" -v -y 2021 -c "The Kubernetes Authors." -f LICENSE_TEMPLATE .

.PHONY: verify-license
verify-license: "$(MYGOBIN)/addlicense"
	"$(MYGOBIN)/addlicense" -check .

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: lint
lint: "$(MYGOBIN)/golangci-lint"
	"$(MYGOBIN)/golangci-lint" run ./...

.PHONY: test
test:
	go test -race -cover ./cmd/... ./pkg/...

.PHONY: test-e2e
test-e2e: "$(MYGOBIN)/ginkgo" "$(MYGOBIN)/kind"
	kind delete cluster --name=cli-utils-e2e && kind create cluster --name=cli-utils-e2e --wait 5m
	"$(MYGOBIN)/ginkgo" -v ./test/e2e/... -- -v 3

.PHONY: test-e2e-focus
test-e2e-focus: "$(MYGOBIN)/ginkgo" "$(MYGOBIN)/kind"
	kind delete cluster --name=cli-utils-e2e && kind create cluster --name=cli-utils-e2e --wait 5m
	"$(MYGOBIN)"/ginkgo -v -focus ".*$(FOCUS).*" ./test/e2e/... -- -v 5

.PHONY: test-stress
test-stress: "$(MYGOBIN)/ginkgo" "$(MYGOBIN)/kind"
	kind delete cluster --name=cli-utils-e2e && kind create cluster --name=cli-utils-e2e --wait 5m \
		--config=./test/stress/kind-cluster.yaml
	kubectl wait nodes --for=condition=ready --all --timeout=5m
	"$(MYGOBIN)/ginkgo" -v ./test/stress/... -- -v 3

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
test-examples-e2e-kapply: "$(MYGOBIN)/mdrip" "$(MYGOBIN)/kind"
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
