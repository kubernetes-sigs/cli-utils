// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package actuation

import runtime "k8s.io/apimachinery/pkg/runtime"

// GroupName is the group name use in this package
const GroupName = "cli-utils.kubernetes.io"

var (
	SchemeBuilder runtime.SchemeBuilder
	// localSchemeBuilder required for generated conversion code to compile.
	localSchemeBuilder = &SchemeBuilder
)
