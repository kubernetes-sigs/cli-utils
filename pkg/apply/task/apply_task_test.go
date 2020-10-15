// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"sync"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

type resourceInfo struct {
	group      string
	apiVersion string
	kind       string
	name       string
	namespace  string
	uid        types.UID
	generation int64
}

func TestApplyTask_FetchGeneration(t *testing.T) {
	testCases := map[string]struct {
		rss []resourceInfo
	}{
		"single namespaced resource": {
			rss: []resourceInfo{
				{
					group:      "apps",
					apiVersion: "apps/v1",
					kind:       "Deployment",
					name:       "foo",
					namespace:  "default",
					uid:        types.UID("my-uid"),
					generation: int64(42),
				},
			},
		},
		"multiple clusterscoped resources": {
			rss: []resourceInfo{
				{
					group:      "custom.io",
					apiVersion: "custom.io/v1beta1",
					kind:       "Custom",
					name:       "bar",
					uid:        types.UID("uid-1"),
					generation: int64(32),
				},
				{
					group:      "custom2.io",
					apiVersion: "custom2.io/v1",
					kind:       "Custom2",
					name:       "foo",
					uid:        types.UID("uid-2"),
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

			infos := toInfos(tc.rss)

			oldAO := applyOptionsFactoryFunc
			applyOptionsFactoryFunc = func(chan event.Event, common.DryRunStrategy, util.Factory) (applyOptions, error) {
				return &fakeApplyOptions{}, nil
			}
			defer func() { applyOptionsFactoryFunc = oldAO }()

			applyTask := &ApplyTask{
				Objects:    infos,
				InfoHelper: &fakeInfoHelper{},
			}

			applyTask.Start(taskContext)

			<-taskContext.TaskChannel()

			for _, info := range tc.rss {
				id := object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: info.group,
						Kind:  info.kind,
					},
					Name:      info.name,
					Namespace: info.namespace,
				}
				uid, _ := taskContext.ResourceUID(id)
				assert.Equal(t, info.uid, uid)

				gen, _ := taskContext.ResourceGeneration(id)
				assert.Equal(t, info.generation, gen)
			}
		})
	}
}

func TestApplyTask_DryRun(t *testing.T) {
	testCases := map[string]struct {
		infos           []*resource.Info
		crds            []*resource.Info
		expectedObjects []object.ObjMetadata
		expectedEvents  []event.Event
	}{
		"dry run with no CRDs or CRs": {
			infos: []*resource.Info{
				toInfo(map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "default",
					},
				}),
			},
			expectedObjects: []object.ObjMetadata{
				{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Name:      "foo",
					Namespace: "default",
				},
			},
			expectedEvents: []event.Event{},
		},
		"dry run with CRD and CR": {
			crds: []*resource.Info{
				toInfo(map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name": "foo",
					},
					"spec": map[string]interface{}{
						"group": "custom.io",
						"names": map[string]interface{}{
							"kind": "Custom",
						},
						"versions": []interface{}{
							map[string]interface{}{
								"name": "v1alpha1",
							},
						},
					},
				}),
			},
			infos: []*resource.Info{
				toInfo(map[string]interface{}{
					"apiVersion": "custom.io/v1alpha1",
					"kind":       "Custom",
					"metadata": map[string]interface{}{
						"name": "bar",
					},
				}),
			},
			expectedObjects: []object.ObjMetadata{},
			expectedEvents: []event.Event{
				{
					Type: event.ApplyType,
				},
			},
		},
		"dry run with CRD and CR and CRD already installed": {
			crds: []*resource.Info{
				toInfo(map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name": "foo",
					},
					"spec": map[string]interface{}{
						"group": "anothercustom.io",
						"names": map[string]interface{}{
							"kind": "AnotherCustom",
						},
						"versions": []interface{}{
							map[string]interface{}{
								"name": "v2",
							},
						},
					},
				}),
			},
			infos: []*resource.Info{
				toInfo(map[string]interface{}{
					"apiVersion": "anothercustom.io/v2",
					"kind":       "AnotherCustom",
					"metadata": map[string]interface{}{
						"name":      "bar",
						"namespace": "barbar",
					},
				}),
			},
			expectedObjects: []object.ObjMetadata{
				{
					GroupKind: schema.GroupKind{
						Group: "anothercustom.io",
						Kind:  "AnotherCustom",
					},
					Name:      "bar",
					Namespace: "barbar",
				},
			},
			expectedEvents: []event.Event{},
		},
	}

	for tn, tc := range testCases {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(tn, func(t *testing.T) {
				eventChannel := make(chan event.Event)
				taskContext := taskrunner.NewTaskContext(eventChannel)

				restMapper := testutil.NewFakeRESTMapper(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				}, schema.GroupVersionKind{
					Group:   "anothercustom.io",
					Version: "v2",
					Kind:    "AnotherCustom",
				})

				ao := &fakeApplyOptions{}
				oldAO := applyOptionsFactoryFunc
				applyOptionsFactoryFunc = func(chan event.Event, common.DryRunStrategy, util.Factory) (applyOptions, error) {
					return ao, nil
				}
				defer func() { applyOptionsFactoryFunc = oldAO }()

				applyTask := &ApplyTask{
					Objects:        tc.infos,
					InfoHelper:     &fakeInfoHelper{},
					Mapper:         restMapper,
					DryRunStrategy: drs,
					CRDs:           tc.crds,
				}

				var events []event.Event
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					for msg := range eventChannel {
						events = append(events, msg)
					}
				}()

				applyTask.Start(taskContext)
				<-taskContext.TaskChannel()
				close(eventChannel)
				wg.Wait()

				assert.Equal(t, len(tc.expectedObjects), len(ao.objects))
				for i, obj := range ao.objects {
					actual, err := object.InfoToObjMeta(obj)
					if err != nil {
						continue
					}
					assert.Equal(t, tc.expectedObjects[i], actual)
				}

				assert.Equal(t, len(tc.expectedEvents), len(events))
				for i, e := range events {
					assert.Equal(t, tc.expectedEvents[i].Type, e.Type)
				}
			})
		}
	}
}

func toInfo(obj map[string]interface{}) *resource.Info {
	return &resource.Info{
		Object: &unstructured.Unstructured{
			Object: obj,
		},
	}
}

func toInfos(rss []resourceInfo) []*resource.Info {
	var infos []*resource.Info

	for _, rs := range rss {
		infos = append(infos, &resource.Info{
			Name:      rs.name,
			Namespace: rs.namespace,
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": rs.apiVersion,
					"kind":       rs.kind,
					"metadata": map[string]interface{}{
						"name":       rs.name,
						"namespace":  rs.namespace,
						"uid":        string(rs.uid),
						"generation": rs.generation,
					},
				},
			},
		})
	}
	return infos
}

type fakeApplyOptions struct {
	objects []*resource.Info
}

func (f *fakeApplyOptions) Run() error {
	return nil
}

func (f *fakeApplyOptions) SetObjects(objects []*resource.Info) {
	f.objects = objects
}

type fakeInfoHelper struct{}

func (f *fakeInfoHelper) UpdateInfos([]*resource.Info) error {
	return nil
}
