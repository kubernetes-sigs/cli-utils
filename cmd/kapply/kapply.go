// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"sigs.k8s.io/cli-utils/pkg/apply"

	// This is here rather than in the libraries because of
	// https://github.com/kubernetes-sigs/kustomize/issues/2060
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	//os.Setenv(commandutil.EnableAlphaCommmandsEnvName, "true")
	if err := apply.GetCommand(nil).Execute(); err != nil {
		os.Exit(1)
	}
}
