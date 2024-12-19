// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package stress

import (
	"testing"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func TestStress(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Stress Test Suite")
}
