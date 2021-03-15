// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package duration

import "time"

var startTime time.Time

// SetStartTime sets the start time to now.
func SetStartTime(t time.Time) {
	startTime = t
}

// GetDuration computes the duration from  the start time
// to current time.
func GetDuration(t time.Time) *time.Duration {
	d := t.Sub(startTime)
	return &d
}
