// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	// This is here rather than in the libraries because of
	// https://github.com/kubernetes-sigs/kustomize/issues/2060
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/cli-utils/cmd/common"
)

func main() {
	if err := common.NewKapplyCommand(nil).Execute(); err != nil {
		os.Exit(1)
	}
}
