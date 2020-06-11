// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestResetRESTMapperTask(t *testing.T) {
	testCases := map[string]struct {
		toRESTMapper       func() (meta.RESTMapper, *fakeCachedDiscoveryClient)
		expectErr          bool
		expectedErrMessage string
	}{
		"correct wrapped RESTMapper": {
			toRESTMapper: func() (meta.RESTMapper, *fakeCachedDiscoveryClient) {
				discoveryClient := &fakeCachedDiscoveryClient{}
				ddRESTMapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
				return restmapper.NewShortcutExpander(ddRESTMapper, discoveryClient), discoveryClient
			},
			expectErr: false,
		},
		"incorrect wrapped RESTMapper": {
			toRESTMapper: func() (meta.RESTMapper, *fakeCachedDiscoveryClient) {
				return testutil.NewFakeRESTMapper(), nil
			},
			expectErr:          true,
			expectedErrMessage: "unexpected RESTMapper type",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			eventChannel := make(chan event.Event)
			defer close(eventChannel)
			taskContext := taskrunner.NewTaskContext(eventChannel)

			mapper, discoveryClient := tc.toRESTMapper()

			resetRESTMapperTask := &ResetRESTMapperTask{
				Mapper: mapper,
			}

			resetRESTMapperTask.Start(taskContext)

			result := <-taskContext.TaskChannel()

			if tc.expectErr {
				assert.Error(t, result.Err)
				assert.Contains(t, result.Err.Error(), tc.expectedErrMessage)
				return
			}

			assert.True(t, discoveryClient.invalidated)
		})
	}
}

type fakeCachedDiscoveryClient struct {
	discovery.DiscoveryInterface
	invalidated bool
}

func (d *fakeCachedDiscoveryClient) Fresh() bool {
	return true
}

func (d *fakeCachedDiscoveryClient) Invalidate() {
	d.invalidated = true
}
