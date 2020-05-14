// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type resourceInfo struct {
	group      string
	apiVersion string
	kind       string
	name       string
	namespace  string
	generation int64
}

func TestApplyTask_FetchGeneration(t *testing.T) {
	testCases := map[string]struct {
		infos []resourceInfo
	}{
		"single namespaced resource": {
			infos: []resourceInfo{
				{
					group:      "apps",
					apiVersion: "apps/v1",
					kind:       "Deployment",
					name:       "foo",
					namespace:  "default",
					generation: int64(42),
				},
			},
		},
		"multiple clusterscoped resources": {
			infos: []resourceInfo{
				{
					group:      "custom.io",
					apiVersion: "custom.io/v1beta1",
					kind:       "Custom",
					name:       "bar",
					generation: int64(32),
				},
				{
					group:      "custom2.io",
					apiVersion: "custom2.io/v1",
					kind:       "Custom2",
					name:       "foo",
					generation: int64(1),
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			eventChannel := make(chan event.Event)
			defer close(eventChannel)
			taskContext := taskrunner.NewTaskContext(eventChannel)

			var infos []*resource.Info

			for _, info := range tc.infos {
				infos = append(infos, &resource.Info{
					Object: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": info.apiVersion,
							"kind":       info.kind,
							"metadata": map[string]interface{}{
								"name":       info.name,
								"namespace":  info.namespace,
								"generation": info.generation,
							},
						},
					},
				})
			}

			applyOptions := &fakeApplyOptions{}

			applyTask := &ApplyTask{
				ApplyOptions: applyOptions,
				Objects:      infos,
				InfoHelper:   &fakeInfoHelper{},
			}

			applyTask.Start(taskContext)

			<-taskContext.TaskChannel()

			for _, info := range tc.infos {
				id := object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: info.group,
						Kind:  info.kind,
					},
					Name:      info.name,
					Namespace: info.namespace,
				}
				gen := taskContext.ResourceGeneration(id)
				assert.Equal(t, info.generation, gen)
			}
		})
	}
}

type fakeApplyOptions struct{}

func (f *fakeApplyOptions) Run() error {
	return nil
}

func (f *fakeApplyOptions) SetObjects([]*resource.Info) {}

type fakeInfoHelper struct{}

func (f *fakeInfoHelper) UpdateInfos(infos []*resource.Info) error {
	return nil
}

func (f *fakeInfoHelper) ResetRESTMapper() error {
	return nil
}
