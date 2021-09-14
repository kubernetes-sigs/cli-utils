// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"os"
	"testing"

	"k8s.io/klog/v2"
)

// TestMain executes the tests for this package.
// Adds support for parsing logging flags. Example:
// go test sigs.k8s.io/cli-utils/pkg/apply/taskrunner -v -args -v=5
func TestMain(m *testing.M) {
	klog.InitFlags(nil)
	os.Exit(m.Run())
}
