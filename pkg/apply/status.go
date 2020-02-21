// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"time"

	"github.com/spf13/cobra"
)

func NewStatusOptions() *StatusOptions {
	return &StatusOptions{
		wait:    false,
		period:  2 * time.Second,
		Timeout: time.Minute,
	}
}

type StatusOptions struct {
	wait    bool
	period  time.Duration
	Timeout time.Duration
}

func (s *StatusOptions) AddFlags(c *cobra.Command) {
	c.Flags().BoolVar(&s.wait, "wait-for-reconcile", s.wait, "Wait for all applied resources to reach the Current status.")
	c.Flags().DurationVar(&s.period, "wait-polling-period", s.period, "Polling period for resource statuses.")
	c.Flags().DurationVar(&s.Timeout, "wait-timeout", s.Timeout, "Timeout threshold for waiting for all resources to reach the Current status.")
}
