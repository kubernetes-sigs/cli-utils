// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package polling

import (
	"context"
	"testing"
	"time"

	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestStatusPoller_Poll_validateFailuresCloseChannel(t *testing.T) {
	poller := StatusPoller{}
	statusChannel := poller.Poll(context.Background(), []object.ObjMetadata{}, Options{
		DesiredStatus: status.UnknownStatus,
	})

	timer := time.NewTimer(3 * time.Second)

	var e event.Event

	for {
		select {
		case msg, ok := <-statusChannel:
			if !ok {
				if want, got := event.ErrorEvent, e.EventType; want != got {
					t.Errorf("expected event type %s, but got %s",
						want.String(), got.String())
				}
				return
			}
			e = msg
		case <-timer.C:
			t.Errorf("expected channel to close, but it didn't")
			return
		}
	}
}
