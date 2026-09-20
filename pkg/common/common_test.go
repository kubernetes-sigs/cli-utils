// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func TestRandomStr(t *testing.T) {
	re := regexp.MustCompile(`^[0-9]{8}$`)
	for i := 0; i < 100; i++ {
		assert.Regexp(t, re, RandomStr())
	}
}

func TestNoDeletion(t *testing.T) {
	tests := map[string]struct {
		key      string
		value    string
		expected bool
	}{
		"lifecycle deletion annotation with prevent value": {
			key:      LifecycleDeleteAnnotation,
			value:    PreventDeletion,
			expected: true,
		},
		"on-remove annotation with keep value": {
			key:      OnRemoveAnnotation,
			value:    OnRemoveKeep,
			expected: true,
		},
		"lifecycle deletion annotation with wrong value": {
			key:      LifecycleDeleteAnnotation,
			value:    "something-else",
			expected: false,
		},
		"unrelated key": {
			key:      "unrelated.key",
			value:    PreventDeletion,
			expected: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, NoDeletion(tc.key, tc.value))
		})
	}
}

func TestDryRunStrategy(t *testing.T) {
	tests := map[string]struct {
		strategy              DryRunStrategy
		clientDryRun          bool
		serverDryRun          bool
		clientOrServerDryRun  bool
		cmdutilDryRunStrategy cmdutil.DryRunStrategy
	}{
		"none": {
			strategy:              DryRunNone,
			clientDryRun:          false,
			serverDryRun:          false,
			clientOrServerDryRun:  false,
			cmdutilDryRunStrategy: cmdutil.DryRunNone,
		},
		"client": {
			strategy:              DryRunClient,
			clientDryRun:          true,
			serverDryRun:          false,
			clientOrServerDryRun:  true,
			cmdutilDryRunStrategy: cmdutil.DryRunClient,
		},
		"server": {
			strategy:              DryRunServer,
			clientDryRun:          false,
			serverDryRun:          true,
			clientOrServerDryRun:  true,
			cmdutilDryRunStrategy: cmdutil.DryRunServer,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.clientDryRun, tc.strategy.ClientDryRun())
			assert.Equal(t, tc.serverDryRun, tc.strategy.ServerDryRun())
			assert.Equal(t, tc.clientOrServerDryRun, tc.strategy.ClientOrServerDryRun())
			assert.Equal(t, tc.cmdutilDryRunStrategy, tc.strategy.Strategy())
		})
	}
}
