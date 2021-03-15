// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package duration

import (
	"testing"
	"time"
)

func TestSetStartTime(t *testing.T) {
	start := time.Date(2021, 03, 12, 00, 10, 20, 0, time.UTC)
	SetStartTime(start)
	if startTime != start {
		t.Errorf("expected %v equal to %v", startTime, start)
	}
}

func TestGetDuration(t *testing.T) {
	start := time.Date(2021, 03, 12, 00, 10, 20, 0, time.UTC)
	SetStartTime(start)
	end := time.Date(2021, 03, 13, 00, 10, 20, 0, time.UTC)
	d := GetDuration(end)
	if *d != 24*time.Hour {
		t.Errorf("expected %v but got %v", 24*time.Hour, *d)
	}
}
