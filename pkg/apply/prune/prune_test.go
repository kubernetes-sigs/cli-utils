// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var testNamespace = "test-inventory-namespace"
var inventoryObjName = "test-inventory-obj"
var podName = "pod-1"
var pdbName = "pdb"

var testInventoryLabel = "test-app-label"

var inventoryObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      inventoryObjName,
			"namespace": testNamespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: testInventoryLabel,
			},
		},
	},
}

var namespace = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": testNamespace,
			"uid":  "uid-namespace",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

var pod = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      podName,
			"namespace": testNamespace,
			"uid":       "pod-uid",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

var pdb = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "policy/v1beta1",
		"kind":       "PodDisruptionBudget",
		"metadata": map[string]interface{}{
			"name":      pdbName,
			"namespace": testNamespace,
			"uid":       "uid2",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

var pdbDeleteFailure = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "policy/v1beta1",
		"kind":       "PodDisruptionBudget",
		"metadata": map[string]interface{}{
			"name":      pdbName + "delete-failure",
			"namespace": testNamespace,
			"uid":       "uid2",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

// Returns a inventory object with the inventory set from
// the passed "children".
func createInventoryInfo(children ...*unstructured.Unstructured) inventory.InventoryInfo {
	inventoryObjCopy := inventoryObj.DeepCopy()
	wrappedInv := inventory.WrapInventoryObj(inventoryObjCopy)
	objs := object.UnstructuredsToObjMetasOrDie(children)
	if err := wrappedInv.Store(objs); err != nil {
		return nil
	}
	obj, err := wrappedInv.GetObject()
	if err != nil {
		return nil
	}
	return inventory.WrapInventoryInfoObj(obj)
}

// preventDelete object contains the "on-remove:keep" lifecycle directive.
var preventDelete = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "test-prevent-delete",
			"namespace": testNamespace,
			"annotations": map[string]interface{}{
				common.OnRemoveAnnotation: common.OnRemoveKeep,
			},
			"uid": "prevent-delete",
		},
	},
}

// Options with different dry-run values.
var (
	defaultOptions = Options{
		DryRunStrategy:    common.DryRunNone,
		PropagationPolicy: metav1.DeletePropagationBackground,
	}
	defaultOptionsDestroy = Options{
		DryRunStrategy:    common.DryRunNone,
		PropagationPolicy: metav1.DeletePropagationBackground,
		Destroy:           true,
	}
	clientDryRunOptions = Options{
		DryRunStrategy:    common.DryRunClient,
		PropagationPolicy: metav1.DeletePropagationBackground,
	}
)

func TestPrune(t *testing.T) {
	tests := map[string]struct {
		pruneObjs      []*unstructured.Unstructured
		pruneFilters   []filter.ValidationFilter
		options        Options
		expectedEvents []testutil.ExpEvent
	}{
		"No pruned objects; no prune/delete events": {
			pruneObjs:      []*unstructured.Unstructured{},
			options:        defaultOptions,
			expectedEvents: []testutil.ExpEvent{},
		},
		"One successfully pruned object": {
			pruneObjs: []*unstructured.Unstructured{pod},
			options:   defaultOptions,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.Pruned,
					},
				},
			},
		},
		"Multiple successfully pruned object": {
			pruneObjs: []*unstructured.Unstructured{pod, pdb, namespace},
			options:   defaultOptions,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.Pruned,
					},
				},
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.Pruned,
					},
				},
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.Pruned,
					},
				},
			},
		},
		"One successfully deleted object": {
			pruneObjs: []*unstructured.Unstructured{pod},
			options:   defaultOptionsDestroy,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.DeleteType,
					DeleteEvent: &testutil.ExpDeleteEvent{
						Operation: event.Deleted,
					},
				},
			},
		},
		"Multiple successfully deleted objects": {
			pruneObjs: []*unstructured.Unstructured{pod, pdb, namespace},
			options:   defaultOptionsDestroy,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.DeleteType,
					DeleteEvent: &testutil.ExpDeleteEvent{
						Operation: event.Deleted,
					},
				},
				{
					EventType: event.DeleteType,
					DeleteEvent: &testutil.ExpDeleteEvent{
						Operation: event.Deleted,
					},
				},
				{
					EventType: event.DeleteType,
					DeleteEvent: &testutil.ExpDeleteEvent{
						Operation: event.Deleted,
					},
				},
			},
		},
		"Client dry run still pruned event": {
			pruneObjs: []*unstructured.Unstructured{pod},
			options:   clientDryRunOptions,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.Pruned,
					},
				},
			},
		},
		"Server dry run still deleted event": {
			pruneObjs: []*unstructured.Unstructured{pod},
			options: Options{
				DryRunStrategy:    common.DryRunServer,
				PropagationPolicy: metav1.DeletePropagationBackground,
				Destroy:           true,
			},
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.DeleteType,
					DeleteEvent: &testutil.ExpDeleteEvent{
						Operation: event.Deleted,
					},
				},
			},
		},
		"UID match means prune skipped": {
			pruneObjs: []*unstructured.Unstructured{pod},
			pruneFilters: []filter.ValidationFilter{
				filter.CurrentUIDFilter{
					// Add pod UID to set of current UIDs
					CurrentUIDs: sets.NewString("pod-uid"),
				},
			},
			options: defaultOptions,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.PruneSkipped,
					},
				},
			},
		},
		"UID match for only one object one pruned, one skipped": {
			pruneObjs: []*unstructured.Unstructured{pod, pdb},
			pruneFilters: []filter.ValidationFilter{
				filter.CurrentUIDFilter{
					// Add pod UID to set of current UIDs
					CurrentUIDs: sets.NewString("pod-uid"),
				},
			},
			options: defaultOptions,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.PruneSkipped,
					},
				},
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.Pruned,
					},
				},
			},
		},
		"Prevent delete annotation equals prune skipped": {
			pruneObjs:    []*unstructured.Unstructured{preventDelete},
			pruneFilters: []filter.ValidationFilter{filter.PreventRemoveFilter{}},
			options:      defaultOptions,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.PruneSkipped,
					},
				},
			},
		},
		"Prevent delete annotation equals delete skipped": {
			pruneObjs:    []*unstructured.Unstructured{preventDelete},
			pruneFilters: []filter.ValidationFilter{filter.PreventRemoveFilter{}},
			options:      defaultOptionsDestroy,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.DeleteType,
					DeleteEvent: &testutil.ExpDeleteEvent{
						Operation: event.DeleteSkipped,
					},
				},
			},
		},
		"Prevent delete annotation, one skipped, one pruned": {
			pruneObjs:    []*unstructured.Unstructured{preventDelete, pod},
			pruneFilters: []filter.ValidationFilter{filter.PreventRemoveFilter{}},
			options:      defaultOptions,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.PruneSkipped,
					},
				},
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.Pruned,
					},
				},
			},
		},
		"Namespace prune skipped": {
			pruneObjs: []*unstructured.Unstructured{namespace},
			pruneFilters: []filter.ValidationFilter{
				filter.LocalNamespacesFilter{
					LocalNamespaces: sets.NewString(namespace.GetName()),
				},
			},
			options: defaultOptions,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.PruneSkipped,
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Set up the fake dynamic client to recognize all objects, and the RESTMapper.
			objs := make([]runtime.Object, 0, len(tc.pruneObjs))
			for _, obj := range tc.pruneObjs {
				objs = append(objs, obj)
			}
			pruneIds, err := object.UnstructuredsToObjMetas(tc.pruneObjs)
			require.NoError(t, err)

			po := PruneOptions{
				InvClient: inventory.NewFakeInventoryClient(pruneIds),
				Client:    fake.NewSimpleDynamicClient(scheme.Scheme, objs...),
				Mapper: testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...),
			}
			// The event channel can not block; make sure its bigger than all
			// the events that can be put on it.
			eventChannel := make(chan event.Event, len(tc.pruneObjs)+1)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := taskrunner.NewTaskContext(eventChannel, resourceCache)
			err = func() error {
				defer close(eventChannel)
				// Run the prune and validate.
				return po.Prune(tc.pruneObjs, tc.pruneFilters, taskContext, "test-0", tc.options)
			}()

			if err != nil {
				t.Fatalf("Unexpected error during Prune(): %#v", err)
			}
			var actualEvents []event.Event
			for e := range eventChannel {
				actualEvents = append(actualEvents, e)
			}
			// Validate the expected/actual events
			err = testutil.VerifyEvents(tc.expectedEvents, actualEvents)
			assert.NoError(t, err)
		})
	}
}

// failureNamespaceClient wrappers around a namespaceClient with the overwriting to Get and Delete functions.
type failureNamespaceClient struct {
	dynamic.ResourceInterface
}

var _ dynamic.ResourceInterface = &failureNamespaceClient{}

func (c *failureNamespaceClient) Delete(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
	if strings.Contains(name, "delete-failure") {
		return fmt.Errorf("expected delete error")
	}
	return nil
}

func (c *failureNamespaceClient) Get(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	if strings.Contains(name, "get-failure") {
		return nil, fmt.Errorf("expected get error")
	}
	return pdb, nil
}

func TestPruneWithErrors(t *testing.T) {
	tests := map[string]struct {
		pruneObjs      []*unstructured.Unstructured
		destroy        bool
		expectedEvents []testutil.ExpEvent
	}{
		"Prune delete failure": {
			pruneObjs: []*unstructured.Unstructured{pdbDeleteFailure},
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Identifier: object.UnstructuredToObjMetaOrDie(pdbDeleteFailure),
						Error:      fmt.Errorf("expected delete error"),
					},
				},
			},
		},
		"Destroy delete failure": {
			pruneObjs: []*unstructured.Unstructured{pdbDeleteFailure},
			destroy:   true,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.DeleteType,
					DeleteEvent: &testutil.ExpDeleteEvent{
						Identifier: object.UnstructuredToObjMetaOrDie(pdbDeleteFailure),
						Error:      fmt.Errorf("expected delete error"),
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pruneIds, err := object.UnstructuredsToObjMetas(tc.pruneObjs)
			require.NoError(t, err)
			po := PruneOptions{
				InvClient: inventory.NewFakeInventoryClient(pruneIds),
				// Set up the fake dynamic client to recognize all objects, and the RESTMapper.
				Client: &fakeDynamicClient{
					resourceInterface: &failureNamespaceClient{},
				},
				Mapper: testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...),
			}
			// The event channel can not block; make sure its bigger than all
			// the events that can be put on it.
			eventChannel := make(chan event.Event, len(tc.pruneObjs))
			resourceCache := cache.NewResourceCacheMap()
			taskContext := taskrunner.NewTaskContext(eventChannel, resourceCache)
			err = func() error {
				defer close(eventChannel)
				var opts Options
				if tc.destroy {
					opts = defaultOptionsDestroy
				} else {
					opts = defaultOptions
				}
				// Run the prune and validate.
				return po.Prune(tc.pruneObjs, []filter.ValidationFilter{}, taskContext, "test-0", opts)
			}()
			if err != nil {
				t.Fatalf("Unexpected error during Prune(): %#v", err)
			}
			var actualEvents []event.Event
			for e := range eventChannel {
				actualEvents = append(actualEvents, e)
			}
			err = testutil.VerifyEvents(tc.expectedEvents, actualEvents)
			assert.NoError(t, err)
		})
	}
}

func TestGetPruneObjs(t *testing.T) {
	tests := map[string]struct {
		localObjs     []*unstructured.Unstructured
		prevInventory []*unstructured.Unstructured
		expectedObjs  []*unstructured.Unstructured
	}{
		"no local objects, no inventory equals no prune objs": {
			localObjs:     []*unstructured.Unstructured{},
			prevInventory: []*unstructured.Unstructured{},
			expectedObjs:  []*unstructured.Unstructured{},
		},
		"local objects, no inventory equals no prune objs": {
			localObjs:     []*unstructured.Unstructured{pod, pdb, namespace},
			prevInventory: []*unstructured.Unstructured{},
			expectedObjs:  []*unstructured.Unstructured{},
		},
		"no local objects, with inventory equals all prune objs": {
			localObjs:     []*unstructured.Unstructured{},
			prevInventory: []*unstructured.Unstructured{pod, pdb, namespace},
			expectedObjs:  []*unstructured.Unstructured{pod, pdb, namespace},
		},
		"set difference equals one prune object": {
			localObjs:     []*unstructured.Unstructured{pod, pdb},
			prevInventory: []*unstructured.Unstructured{pdb, namespace},
			expectedObjs:  []*unstructured.Unstructured{namespace},
		},
		"local and inventory the same equals no prune objects": {
			localObjs:     []*unstructured.Unstructured{pod, pdb},
			prevInventory: []*unstructured.Unstructured{pod, pdb},
			expectedObjs:  []*unstructured.Unstructured{},
		},
		"two prune objects": {
			localObjs:     []*unstructured.Unstructured{pdb},
			prevInventory: []*unstructured.Unstructured{pod, pdb, namespace},
			expectedObjs:  []*unstructured.Unstructured{pod, namespace},
		},
		"local objs, inventory disjoint means inventory is pruned": {
			localObjs:     []*unstructured.Unstructured{pdb},
			prevInventory: []*unstructured.Unstructured{pod, namespace},
			expectedObjs:  []*unstructured.Unstructured{pod, namespace},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			objs := make([]runtime.Object, 0, len(tc.prevInventory))
			for _, obj := range tc.prevInventory {
				objs = append(objs, obj)
			}
			po := PruneOptions{
				InvClient: inventory.NewFakeInventoryClient(object.UnstructuredsToObjMetasOrDie(tc.prevInventory)),
				Client:    fake.NewSimpleDynamicClient(scheme.Scheme, objs...),
				Mapper: testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...),
			}
			currentInventory := createInventoryInfo(tc.prevInventory...)
			actualObjs, err := po.GetPruneObjs(currentInventory, tc.localObjs, Options{})
			if err != nil {
				t.Fatalf("unexpected error %s returned", err)
			}
			if len(tc.expectedObjs) != len(actualObjs) {
				t.Fatalf("expected %d prune objs, got %d", len(tc.expectedObjs), len(actualObjs))
			}
			actualIds, err := object.UnstructuredsToObjMetas(actualObjs)
			require.NoError(t, err)

			expectedIds, err := object.UnstructuredsToObjMetas(tc.expectedObjs)
			require.NoError(t, err)

			if !object.SetEquals(expectedIds, actualIds) {
				t.Errorf("expected prune objects (%v), got (%v)", expectedIds, actualIds)
			}
		})
	}
}

type optionsCaptureNamespaceClient struct {
	dynamic.ResourceInterface
	options metav1.DeleteOptions
}

var _ dynamic.ResourceInterface = &optionsCaptureNamespaceClient{}

func (c *optionsCaptureNamespaceClient) Delete(_ context.Context, _ string, options metav1.DeleteOptions, _ ...string) error {
	c.options = options
	return nil
}

func TestPrune_PropagationPolicy(t *testing.T) {
	testCases := map[string]struct {
		propagationPolicy metav1.DeletionPropagation
	}{
		"background propagation policy": {
			propagationPolicy: metav1.DeletePropagationBackground,
		},
		"foreground propagation policy": {
			propagationPolicy: metav1.DeletePropagationForeground,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			captureClient := &optionsCaptureNamespaceClient{}
			po := PruneOptions{
				InvClient: inventory.NewFakeInventoryClient([]object.ObjMetadata{}),
				Client: &fakeDynamicClient{
					resourceInterface: captureClient,
				},
				Mapper: testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...),
			}

			eventChannel := make(chan event.Event, 1)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := taskrunner.NewTaskContext(eventChannel, resourceCache)
			err := po.Prune([]*unstructured.Unstructured{pdb}, []filter.ValidationFilter{}, taskContext, "test-0", Options{
				PropagationPolicy: tc.propagationPolicy,
			})
			assert.NoError(t, err)
			require.NotNil(t, captureClient.options.PropagationPolicy)
			assert.Equal(t, tc.propagationPolicy, *captureClient.options.PropagationPolicy)
		})
	}
}

type fakeDynamicClient struct {
	resourceInterface dynamic.ResourceInterface
}

var _ dynamic.Interface = &fakeDynamicClient{}

func (c *fakeDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &fakeDynamicResourceClient{
		resourceInterface:              c.resourceInterface,
		NamespaceableResourceInterface: fake.NewSimpleDynamicClient(scheme.Scheme).Resource(resource),
	}
}

type fakeDynamicResourceClient struct {
	dynamic.NamespaceableResourceInterface
	resourceInterface dynamic.ResourceInterface
}

func (c *fakeDynamicResourceClient) Namespace(ns string) dynamic.ResourceInterface {
	return c.resourceInterface
}
