// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package printer

import "sigs.k8s.io/cli-utils/pkg/apply/event"

type Printer interface {
	Print(ch <-chan event.Event, preview bool)
}
