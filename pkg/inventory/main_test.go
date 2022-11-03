// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"os"
	"testing"

	"k8s.io/klog/v2"
)

// TestMain executes the tests for this package, with optional logging.
// To see all logs, use:
// go test sigs.k8s.io/cli-utils/pkg/inventory -v -args -v=5
func TestMain(m *testing.M) {
	klog.InitFlags(nil)
	os.Exit(m.Run())
}
