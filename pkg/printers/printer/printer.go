// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package printer

import (
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
)

type Printer interface {
	Print(ch <-chan event.Event, previewStrategy common.DryRunStrategy, printStatus bool) error
}

type DryRunStringer interface {
	String(strategy common.DryRunStrategy) string
}

type PreviewStringer struct{}

func (p PreviewStringer) String(strategy common.DryRunStrategy) string {
	switch {
	case strategy.ClientDryRun():
		return " (preview)"
	case strategy.ServerDryRun():
		return " (preview-server)"
	default:
		return ""
	}
}
