// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
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

// Tests that the correct "applied" objects are sent
// to the TaskContext correctly, since these are the
// applied objects added to the final inventory.
func TestApplyTask_BasicAppliedObjects(t *testing.T) {
	testCases := map[string]struct {
		applied []resourceInfo
	}{
		"apply single namespaced resource": {
			applied: []resourceInfo{
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
		"apply multiple clusterscoped resources": {
			applied: []resourceInfo{
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

			objs := toUnstructureds(tc.applied)

			oldAO := applyOptionsFactoryFunc
			applyOptionsFactoryFunc = func(chan event.Event, common.ServerSideOptions, common.DryRunStrategy, util.Factory) (applyOptions, dynamic.Interface, error) {
				return &fakeApplyOptions{}, nil, nil
			}
			defer func() { applyOptionsFactoryFunc = oldAO }()

			restMapper := testutil.NewFakeRESTMapper(schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			}, schema.GroupVersionKind{
				Group:   "anothercustom.io",
				Version: "v2",
				Kind:    "AnotherCustom",
			})

			applyTask := &ApplyTask{
				Objects:    objs,
				Mapper:     restMapper,
				InfoHelper: &fakeInfoHelper{},
				InvInfo:    &fakeInventoryInfo{},
			}

			getClusterObj = func(d dynamic.Interface, info *resource.Info) (*unstructured.Unstructured, error) {
				return objs[0], nil
			}
			applyTask.Start(taskContext)
			<-taskContext.TaskChannel()

			// The applied resources should be stored in the TaskContext
			// for the final inventory.
			expected, err := object.UnstructuredsToObjMetas(objs)
			require.NoError(t, err)

			actual := taskContext.AppliedResources()
			if !object.SetEquals(expected, actual) {
				t.Errorf("expected (%s) inventory resources, got (%s)", expected, actual)
			}
		})
	}
}

// Checks the inventory stored in the task context applied
// resources is correct, given a retrieval error and
// a specific previous inventory. Also, an apply failure
// for an object in the previous inventory should remain
// in the inventory, while an apply failure that is not
// in the previous inventory (creation) should not be
// in the final inventory.
func TestApplyTask_ApplyFailuresAndInventory(t *testing.T) {
	resInfo := resourceInfo{
		group:      "apps",
		apiVersion: "apps/v1",
		kind:       "Deployment",
		name:       "foo",
		namespace:  "default",
		uid:        types.UID("my-uid"),
		generation: int64(42),
	}
	resID, _ := object.CreateObjMetadata("default", "foo",
		schema.GroupKind{Group: "apps", Kind: "Deployment"})
	applyFailInfo := resourceInfo{
		group:      "apps",
		apiVersion: "apps/v1",
		kind:       "Deployment",
		name:       "failure",
		namespace:  "default",
		uid:        types.UID("my-uid"),
		generation: int64(42),
	}
	applyFailID, _ := object.CreateObjMetadata("default", "failure",
		schema.GroupKind{Group: "apps", Kind: "Deployment"})

	testCases := map[string]struct {
		applied       []resourceInfo
		prevInventory []object.ObjMetadata
		expected      []object.ObjMetadata
		err           error
	}{
		"not found error with successful apply is in final inventory": {
			applied:       []resourceInfo{resInfo},
			prevInventory: []object.ObjMetadata{},
			expected:      []object.ObjMetadata{resID},
			err:           apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "pod"}, "fake"),
		},
		"unknown error, but in previous inventory: object is in final inventory": {
			applied:       []resourceInfo{resInfo},
			prevInventory: []object.ObjMetadata{resID},
			expected:      []object.ObjMetadata{resID},
			err:           apierrors.NewUnauthorized("not authorized"),
		},
		"unknown error, not in previous inventory: object is NOT in final inventory": {
			applied:       []resourceInfo{resInfo},
			prevInventory: []object.ObjMetadata{},
			expected:      []object.ObjMetadata{},
			err:           apierrors.NewUnauthorized("not authorized"),
		},
		"apply failure, in previous inventory: object is in final inventory": {
			applied:       []resourceInfo{applyFailInfo},
			prevInventory: []object.ObjMetadata{applyFailID},
			expected:      []object.ObjMetadata{applyFailID},
			err:           nil,
		},
		"apply failure, not in previous inventory: object is NOT in final inventory": {
			applied:       []resourceInfo{applyFailInfo},
			prevInventory: []object.ObjMetadata{},
			expected:      []object.ObjMetadata{},
			err:           nil,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			eventChannel := make(chan event.Event)
			taskContext := taskrunner.NewTaskContext(eventChannel)

			objs := toUnstructureds(tc.applied)

			oldAO := applyOptionsFactoryFunc
			applyOptionsFactoryFunc = func(chan event.Event, common.ServerSideOptions, common.DryRunStrategy, util.Factory) (applyOptions, dynamic.Interface, error) {
				return &fakeApplyOptions{}, nil, nil
			}
			defer func() { applyOptionsFactoryFunc = oldAO }()

			restMapper := testutil.NewFakeRESTMapper(schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			})

			prevInv := map[object.ObjMetadata]bool{}
			for _, id := range tc.prevInventory {
				prevInv[id] = true
			}
			applyTask := &ApplyTask{
				Objects:       objs,
				PrevInventory: prevInv,
				Mapper:        restMapper,
				InfoHelper:    &fakeInfoHelper{},
				InvInfo:       &fakeInventoryInfo{},
			}

			getClusterObj = func(d dynamic.Interface, info *resource.Info) (*unstructured.Unstructured, error) {
				return objs[0], nil
			}
			if tc.err != nil {
				getClusterObj = func(d dynamic.Interface, info *resource.Info) (*unstructured.Unstructured, error) {
					return nil, tc.err
				}
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

			// The applied resources should be stored in the TaskContext
			// for the final inventory.
			actual := taskContext.AppliedResources()
			if !object.SetEquals(tc.expected, actual) {
				t.Errorf("expected (%s) inventory resources, got (%s)", tc.expected, actual)
			}
		})
	}
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

			objs := toUnstructureds(tc.rss)

			oldAO := applyOptionsFactoryFunc
			applyOptionsFactoryFunc = func(chan event.Event, common.ServerSideOptions, common.DryRunStrategy, util.Factory) (applyOptions, dynamic.Interface, error) {
				return &fakeApplyOptions{}, nil, nil
			}
			defer func() { applyOptionsFactoryFunc = oldAO }()
			applyTask := &ApplyTask{
				Objects:    objs,
				InfoHelper: &fakeInfoHelper{},
				InvInfo:    &fakeInventoryInfo{},
			}

			getClusterObj = func(d dynamic.Interface, info *resource.Info) (*unstructured.Unstructured, error) {
				return objs[0], nil
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
		objs            []*unstructured.Unstructured
		crds            []*unstructured.Unstructured
		expectedObjects []object.ObjMetadata
		expectedEvents  []event.Event
	}{
		"dry run with no CRDs or CRs": {
			objs: []*unstructured.Unstructured{
				toUnstructured(map[string]interface{}{
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
			crds: []*unstructured.Unstructured{
				toUnstructured(map[string]interface{}{
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
			objs: []*unstructured.Unstructured{
				toUnstructured(map[string]interface{}{
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
			crds: []*unstructured.Unstructured{
				toUnstructured(map[string]interface{}{
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
			objs: []*unstructured.Unstructured{
				toUnstructured(map[string]interface{}{
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
				applyOptionsFactoryFunc = func(chan event.Event, common.ServerSideOptions, common.DryRunStrategy, util.Factory) (applyOptions, dynamic.Interface, error) {
					return ao, nil, nil
				}
				defer func() { applyOptionsFactoryFunc = oldAO }()
				getClusterObj = func(d dynamic.Interface, info *resource.Info) (*unstructured.Unstructured, error) {
					return addOwningInventory(tc.objs[0], "id"), nil
				}

				applyTask := &ApplyTask{
					Objects:        tc.objs,
					InfoHelper:     &fakeInfoHelper{},
					Mapper:         restMapper,
					DryRunStrategy: drs,
					CRDs:           tc.crds,
					InvInfo:        &fakeInventoryInfo{},
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

func TestApplyTaskWithError(t *testing.T) {
	testCases := map[string]struct {
		objs            []*unstructured.Unstructured
		crds            []*unstructured.Unstructured
		expectedObjects []object.ObjMetadata
		expectedEvents  []event.Event
	}{
		"some resources have apply error": {
			crds: []*unstructured.Unstructured{
				toUnstructured(map[string]interface{}{
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
			objs: []*unstructured.Unstructured{
				toUnstructured(map[string]interface{}{
					"apiVersion": "anothercustom.io/v2",
					"kind":       "AnotherCustom",
					"metadata": map[string]interface{}{
						"name":      "bar",
						"namespace": "barbar",
					},
				}),
				toUnstructured(map[string]interface{}{
					"apiVersion": "anothercustom.io/v2",
					"kind":       "AnotherCustom",
					"metadata": map[string]interface{}{
						"name":      "bar-with-failure",
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
			expectedEvents: []event.Event{
				{
					Type: event.ApplyType,
					ApplyEvent: event.ApplyEvent{
						Error: fmt.Errorf("expected apply error"),
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		drs := common.DryRunNone
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
			applyOptionsFactoryFunc = func(chan event.Event, common.ServerSideOptions, common.DryRunStrategy, util.Factory) (applyOptions, dynamic.Interface, error) {
				return ao, nil, nil
			}
			defer func() { applyOptionsFactoryFunc = oldAO }()

			getClusterObj = func(d dynamic.Interface, info *resource.Info) (*unstructured.Unstructured, error) {
				return addOwningInventory(tc.objs[0], "id"), nil
			}
			applyTask := &ApplyTask{
				Objects:        tc.objs,
				InfoHelper:     &fakeInfoHelper{},
				Mapper:         restMapper,
				DryRunStrategy: drs,
				CRDs:           tc.crds,
				InvInfo:        &fakeInventoryInfo{},
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

			assert.Equal(t, len(tc.expectedObjects), len(ao.passedObjects))
			for i, obj := range ao.passedObjects {
				actual, err := object.InfoToObjMeta(obj)
				if err != nil {
					continue
				}
				assert.Equal(t, tc.expectedObjects[i], actual)
			}

			assert.Equal(t, len(tc.expectedEvents), len(events))
			for i, e := range events {
				assert.Equal(t, tc.expectedEvents[i].Type, e.Type)
				assert.Equal(t, tc.expectedEvents[i].ApplyEvent.Error.Error(), e.ApplyEvent.Error.Error())
			}
		})
	}
}

var deployment = toUnstructured(map[string]interface{}{
	"apiVersion": "apps/v1",
	"kind":       "Deployment",
	"metadata": map[string]interface{}{
		"name":      "deploy",
		"namespace": "default",
		"uid":       "uid-deployment",
	},
})

var deploymentObjMetadata = []object.ObjMetadata{
	{
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
		Name:      "deploy",
		Namespace: "default",
	},
}

func TestApplyTaskWithDifferentInventoryAnnotation(t *testing.T) {
	testCases := map[string]struct {
		obj             *unstructured.Unstructured
		clusterObj      *unstructured.Unstructured
		policy          inventory.InventoryPolicy
		expectedObjects []object.ObjMetadata
		expectedEvents  []event.Event
	}{
		"InventoryPolicyMustMatch with object doesn't exist on cluster - Can Apply": {
			obj:             deployment,
			clusterObj:      nil,
			policy:          inventory.InventoryPolicyMustMatch,
			expectedObjects: deploymentObjMetadata,
			expectedEvents:  []event.Event{},
		},
		"InventoryPolicyMustMatch with object annotation is empty - Can't Apply": {
			obj:             deployment,
			clusterObj:      removeOwningInventory(deployment),
			policy:          inventory.InventoryPolicyMustMatch,
			expectedObjects: nil,
			expectedEvents: []event.Event{
				{
					Type: event.ApplyType,
					ApplyEvent: event.ApplyEvent{
						Error: inventory.NewNeedAdoptionError(
							fmt.Errorf("can't adopt an object without the annotation config.k8s.io/owning-inventory")),
					},
				},
			},
		},
		"InventoryPolicyMustMatch with object annotation doesn't match - Can't Apply": {
			obj:             deployment,
			clusterObj:      addOwningInventory(deployment, "unmatchd"),
			policy:          inventory.InventoryPolicyMustMatch,
			expectedObjects: nil,
			expectedEvents: []event.Event{
				{
					Type: event.ApplyType,
					ApplyEvent: event.ApplyEvent{
						Error: inventory.NewInventoryOverlapError(
							fmt.Errorf("can't apply the resource since its annotation config.k8s.io/owning-inventory is a different inventory object")),
					},
				},
			},
		},
		"InventoryPolicyMustMatch with object annotation matches - Can Apply": {
			obj:             deployment,
			clusterObj:      addOwningInventory(deployment, "id"),
			policy:          inventory.InventoryPolicyMustMatch,
			expectedObjects: deploymentObjMetadata,
			expectedEvents:  nil,
		},
		"AdoptIfNoInventory with object doesn't exist on cluster - Can Apply": {
			obj:             deployment,
			clusterObj:      nil,
			policy:          inventory.AdoptIfNoInventory,
			expectedObjects: deploymentObjMetadata,
			expectedEvents:  []event.Event{},
		},
		"AdoptIfNoInventory with object annotation is empty - Can Apply": {
			obj:             deployment,
			clusterObj:      removeOwningInventory(deployment),
			policy:          inventory.AdoptIfNoInventory,
			expectedObjects: deploymentObjMetadata,
			expectedEvents:  []event.Event{},
		},
		"AdoptIfNoInventory with object annotation doesn't match - Can't Apply": {
			obj:             deployment,
			clusterObj:      addOwningInventory(deployment, "notmatch"),
			policy:          inventory.AdoptIfNoInventory,
			expectedObjects: nil,
			expectedEvents: []event.Event{
				{
					Type: event.ApplyType,
					ApplyEvent: event.ApplyEvent{
						Error: inventory.NewInventoryOverlapError(
							fmt.Errorf("can't apply the resource since its annotation config.k8s.io/owning-inventory is a different inventory object")),
					},
				},
			},
		},
		"AdoptIfNoInventory with object annotation matches - Can Apply": {
			obj:             deployment,
			clusterObj:      addOwningInventory(deployment, "id"),
			policy:          inventory.AdoptIfNoInventory,
			expectedObjects: deploymentObjMetadata,
			expectedEvents:  []event.Event{},
		},
		"AdoptAll with object doesn't exist on cluster - Can Apply": {
			obj:             deployment,
			clusterObj:      nil,
			policy:          inventory.AdoptAll,
			expectedObjects: deploymentObjMetadata,
			expectedEvents:  []event.Event{},
		},
	}

	for tn, tc := range testCases {
		drs := common.DryRunNone
		t.Run(tn, func(t *testing.T) {
			eventChannel := make(chan event.Event)
			taskContext := taskrunner.NewTaskContext(eventChannel)

			restMapper := testutil.NewFakeRESTMapper(schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			})

			ao := &fakeApplyOptions{}
			oldAO := applyOptionsFactoryFunc
			applyOptionsFactoryFunc = func(chan event.Event, common.ServerSideOptions, common.DryRunStrategy, util.Factory) (applyOptions, dynamic.Interface, error) {
				return ao, nil, nil
			}
			defer func() { applyOptionsFactoryFunc = oldAO }()

			getClusterObj = func(d dynamic.Interface, info *resource.Info) (*unstructured.Unstructured, error) {
				return tc.clusterObj, nil
			}
			applyTask := &ApplyTask{
				Objects:         []*unstructured.Unstructured{tc.obj},
				InfoHelper:      &fakeInfoHelper{},
				Mapper:          restMapper,
				DryRunStrategy:  drs,
				InvInfo:         &fakeInventoryInfo{},
				InventoryPolicy: tc.policy,
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

			assert.Equal(t, len(tc.expectedObjects), len(ao.passedObjects))
			for i, obj := range ao.passedObjects {
				actual, err := object.InfoToObjMeta(obj)
				if err != nil {
					continue
				}
				assert.Equal(t, tc.expectedObjects[i], actual)
			}

			assert.Equal(t, len(tc.expectedEvents), len(events))
			for i, e := range events {
				assert.Equal(t, tc.expectedEvents[i].Type, e.Type)
				assert.Equal(t, tc.expectedEvents[i].ApplyEvent.Error.Error(), e.ApplyEvent.Error.Error())
			}
			actualUids := taskContext.AppliedResourceUIDs()
			assert.Equal(t, len(actualUids), 1)
		})
	}
}

func toUnstructured(obj map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: obj,
	}
}

func toUnstructureds(rss []resourceInfo) []*unstructured.Unstructured {
	var objs []*unstructured.Unstructured

	for _, rs := range rss {
		objs = append(objs, &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": rs.apiVersion,
				"kind":       rs.kind,
				"metadata": map[string]interface{}{
					"name":       rs.name,
					"namespace":  rs.namespace,
					"uid":        string(rs.uid),
					"generation": rs.generation,
					"annotations": map[string]interface{}{
						"config.k8s.io/owning-inventory": "id",
					},
				},
			},
		})
	}
	return objs
}

type fakeApplyOptions struct {
	objects       []*resource.Info
	passedObjects []*resource.Info
}

func (f *fakeApplyOptions) Run() error {
	var err error
	for _, obj := range f.objects {
		if strings.Contains(obj.Name, "failure") {
			err = fmt.Errorf("expected apply error")
		} else {
			f.passedObjects = append(f.passedObjects, obj)
		}
	}
	return err
}

func (f *fakeApplyOptions) SetObjects(objects []*resource.Info) {
	f.objects = objects
}

type fakeInfoHelper struct{}

func (f *fakeInfoHelper) UpdateInfo(*resource.Info) error {
	return nil
}

func (f *fakeInfoHelper) BuildInfos(objs []*unstructured.Unstructured) ([]*resource.Info, error) {
	return object.UnstructuredsToInfos(objs)
}

func (f *fakeInfoHelper) BuildInfo(obj *unstructured.Unstructured) (*resource.Info, error) {
	return object.UnstructuredToInfo(obj)
}

type fakeInventoryInfo struct{}

func (fi *fakeInventoryInfo) Name() string {
	return "name"
}

func (fi *fakeInventoryInfo) Namespace() string {
	return "namespace"
}

func (fi *fakeInventoryInfo) ID() string {
	return "id"
}

func (fi *fakeInventoryInfo) Strategy() inventory.InventoryStrategy {
	return inventory.NameStrategy
}

func addOwningInventory(obj *unstructured.Unstructured, id string) *unstructured.Unstructured {
	if obj == nil {
		return nil
	}
	newObj := obj.DeepCopy()
	annotations := newObj.GetAnnotations()
	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}

	annotations["config.k8s.io/owning-inventory"] = id
	newObj.SetAnnotations(annotations)
	return newObj
}

func removeOwningInventory(obj *unstructured.Unstructured) *unstructured.Unstructured {
	if obj == nil {
		return nil
	}
	newObj := obj.DeepCopy()
	annotations := newObj.GetAnnotations()
	if len(annotations) == 0 {
		return newObj
	}
	delete(annotations, "config.k8s.io/owning-inventory")
	newObj.SetAnnotations(annotations)
	return newObj
}
