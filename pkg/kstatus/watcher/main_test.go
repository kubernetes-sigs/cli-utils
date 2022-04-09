// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"os"
	"testing"

	"github.com/onsi/gomega/format"
	"k8s.io/klog/v2"
)

// TestMain executes the tests for this package, with optional logging.
// To see all logs, use:
// go test sigs.k8s.io/cli-utils/pkg/kstatus/watcher -v -args -v=5
func TestMain(m *testing.M) {
	// increase from 4000 to handle long event lists
	format.MaxLength = 10000
	klog.InitFlags(nil)
	os.Exit(m.Run())
}
