// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest/fake"
	clienttesting "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/provider"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var (
	codec     = scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
	resources = map[string]string{
		"deployment": `
kind: Deployment
apiVersion: apps/v1
metadata:
  name: foo
  namespace: default
  uid: dep-uid
  generation: 1
spec:
  replicas: 1
`,
		"secret": `
kind: Secret
apiVersion: v1
metadata:
  name: secret
  namespace: default
  uid: secret-uid
  generation: 1
type: Opaque
spec:
  foo: bar
`,
	}
)

func Unstructured(t *testing.T, manifest string, mutators ...mutator) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	err := runtime.DecodeInto(codec, []byte(manifest), u)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	for _, m := range mutators {
		m.Mutate(u)
	}
	return u
}

type mutator interface {
	Mutate(u *unstructured.Unstructured)
}

func addOwningInv(t *testing.T, inv string) mutator {
	return owningInvMutator{
		t:   t,
		inv: inv,
	}
}

type owningInvMutator struct {
	t   *testing.T
	inv string
}

func (a owningInvMutator) Mutate(u *unstructured.Unstructured) {
	annos, found, err := unstructured.NestedStringMap(u.Object, "metadata", "annotations")
	if !assert.NoError(a.t, err) {
		a.t.FailNow()
	}
	if !found {
		annos = make(map[string]string)
	}
	annos["config.k8s.io/owning-inventory"] = a.inv
	err = unstructured.SetNestedStringMap(u.Object, annos, "metadata", "annotations")
	if !assert.NoError(a.t, err) {
		a.t.FailNow()
	}
}

type resourceInfo struct {
	resource *unstructured.Unstructured
	exists   bool
}

type inventoryInfo struct {
	name      string
	namespace string
	id        string
	list      []object.ObjMetadata
}

func (i inventoryInfo) toWrapped() inventory.InventoryInfo {
	inv := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      i.name,
				"namespace": i.namespace,
				"labels": map[string]interface{}{
					common.InventoryLabel: i.id,
				},
			},
		},
	}
	return inventory.WrapInventoryInfoObj(inv)
}

func TestApplier(t *testing.T) {
	testCases := map[string]struct {
		namespace        string
		resources        []*unstructured.Unstructured
		invInfo          inventoryInfo
		clusterObjs      []*unstructured.Unstructured
		handlers         []handler
		reconcileTimeout time.Duration
		prune            bool
		inventoryPolicy  inventory.InventoryPolicy
		statusEvents     []pollevent.Event
		expectedEvents   []testutil.ExpEvent
	}{
		"initial apply without status or prune": {
			namespace: "default",
			resources: []*unstructured.Unstructured{
				Unstructured(t, resources["deployment"]),
			},
			invInfo: inventoryInfo{
				name:      "abc-123",
				namespace: "default",
				id:        "test",
			},
			clusterObjs:      []*unstructured.Unstructured{},
			reconcileTimeout: time.Duration(0),
			prune:            false,
			inventoryPolicy:  inventory.InventoryPolicyMustMatch,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.InitType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ApplyType,
				},
				{
					EventType: event.ActionGroupType,
				},
			},
		},
		"first apply multiple resources with status and prune": {
			namespace: "default",
			resources: []*unstructured.Unstructured{
				Unstructured(t, resources["deployment"]),
				Unstructured(t, resources["secret"]),
			},
			invInfo: inventoryInfo{
				name:      "inv-123",
				namespace: "default",
				id:        "test",
			},
			clusterObjs:      []*unstructured.Unstructured{},
			reconcileTimeout: time.Minute,
			prune:            true,
			inventoryPolicy:  inventory.InventoryPolicyMustMatch,
			statusEvents: []pollevent.Event{
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["deployment"]),
						Status:     status.InProgressStatus,
						Resource:   Unstructured(t, resources["deployment"]),
					},
				},
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["deployment"]),
						Status:     status.CurrentStatus,
						Resource:   Unstructured(t, resources["deployment"]),
					},
				},
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["secret"]),
						Status:     status.CurrentStatus,
						Resource:   Unstructured(t, resources["secret"]),
					},
				},
			},
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.InitType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ApplyType,
				},
				{
					EventType: event.ApplyType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.StatusType,
				},
				{
					EventType: event.StatusType,
				},
				{
					EventType: event.StatusType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
			},
		},
		"apply multiple existing resources with status and prune": {
			namespace: "default",
			resources: []*unstructured.Unstructured{
				Unstructured(t, resources["deployment"]),
				Unstructured(t, resources["secret"]),
			},
			invInfo: inventoryInfo{
				name:      "inv-123",
				namespace: "default",
				id:        "test",
				list: []object.ObjMetadata{
					object.UnstructuredToObjMeta(
						Unstructured(t, resources["deployment"]),
					),
				},
			},
			clusterObjs: []*unstructured.Unstructured{
				Unstructured(t, resources["deployment"]),
			},
			reconcileTimeout: time.Minute,
			prune:            true,
			inventoryPolicy:  inventory.AdoptIfNoInventory,
			statusEvents: []pollevent.Event{
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["deployment"]),
						Status:     status.CurrentStatus,
						Resource:   Unstructured(t, resources["deployment"]),
					},
				},
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["secret"]),
						Status:     status.CurrentStatus,
						Resource:   Unstructured(t, resources["secret"]),
					},
				},
			},
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.InitType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ApplyType,
				},
				{
					EventType: event.ApplyType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.StatusType,
				},
				{
					EventType: event.StatusType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
			},
		},
		"apply no resources and prune all existing": {
			namespace: "default",
			resources: []*unstructured.Unstructured{},
			invInfo: inventoryInfo{
				name:      "inv-123",
				namespace: "default",
				id:        "test",
				list: []object.ObjMetadata{
					object.UnstructuredToObjMeta(
						Unstructured(t, resources["deployment"]),
					),
					object.UnstructuredToObjMeta(
						Unstructured(t, resources["secret"]),
					),
				},
			},
			clusterObjs: []*unstructured.Unstructured{
				Unstructured(t, resources["deployment"], addOwningInv(t, "test")),
				Unstructured(t, resources["secret"], addOwningInv(t, "test")),
			},
			reconcileTimeout: time.Minute,
			prune:            true,
			inventoryPolicy:  inventory.InventoryPolicyMustMatch,
			statusEvents:     []pollevent.Event{},
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.InitType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
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
				{
					EventType: event.ActionGroupType,
				},
			},
		},
		"apply resource with existing object belonging to different inventory": {
			namespace: "default",
			resources: []*unstructured.Unstructured{
				Unstructured(t, resources["deployment"]),
			},
			invInfo: inventoryInfo{
				name:      "abc-123",
				namespace: "default",
				id:        "test",
			},
			clusterObjs: []*unstructured.Unstructured{
				Unstructured(t, resources["deployment"], addOwningInv(t, "unmatched")),
			},
			reconcileTimeout: time.Minute,
			prune:            true,
			inventoryPolicy:  inventory.InventoryPolicyMustMatch,
			statusEvents: []pollevent.Event{
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["deployment"]),
						Status:     status.InProgressStatus,
					},
				},
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["deployment"]),
						Status:     status.CurrentStatus,
					},
				},
			},
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.InitType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ApplyType,
					ApplyEvent: &testutil.ExpApplyEvent{
						Error: inventory.NewInventoryOverlapError(fmt.Errorf("")),
					},
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
			},
		},
		"resources belonging to a different inventory should not be pruned": {
			namespace: "default",
			resources: []*unstructured.Unstructured{},
			invInfo: inventoryInfo{
				name:      "abc-123",
				namespace: "default",
				id:        "test",
				list: []object.ObjMetadata{
					object.UnstructuredToObjMeta(
						Unstructured(t, resources["deployment"]),
					),
				},
			},
			clusterObjs: []*unstructured.Unstructured{
				Unstructured(t, resources["deployment"], addOwningInv(t, "unmatched")),
			},
			reconcileTimeout: 0,
			prune:            true,
			inventoryPolicy:  inventory.InventoryPolicyMustMatch,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.InitType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.PruneSkipped,
					},
				},
				{
					EventType: event.ActionGroupType,
				},
			},
		},
		"prune with inventory object annotation matched": {
			namespace: "default",
			resources: []*unstructured.Unstructured{},
			invInfo: inventoryInfo{
				name:      "abc-123",
				namespace: "default",
				id:        "test",
				list: []object.ObjMetadata{
					object.UnstructuredToObjMeta(
						Unstructured(t, resources["deployment"]),
					),
				},
			},
			clusterObjs: []*unstructured.Unstructured{
				Unstructured(t, resources["deployment"], addOwningInv(t, "test")),
			},
			reconcileTimeout: 0,
			prune:            true,
			inventoryPolicy:  inventory.InventoryPolicyMustMatch,
			expectedEvents: []testutil.ExpEvent{
				{
					EventType: event.InitType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.ActionGroupType,
				},
				{
					EventType: event.PruneType,
					PruneEvent: &testutil.ExpPruneEvent{
						Operation: event.Pruned,
					},
				},
				{
					EventType: event.ActionGroupType,
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace(tc.namespace)
			defer tf.Cleanup()

			mapper, err := tf.ToRESTMapper()
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			objMap := make(map[object.ObjMetadata]resourceInfo)
			for _, r := range tc.resources {
				objMeta := object.UnstructuredToObjMeta(r)
				objMap[objMeta] = resourceInfo{
					resource: r,
					exists:   false,
				}
			}
			for _, r := range tc.clusterObjs {
				objMeta := object.UnstructuredToObjMeta(r)
				objMap[objMeta] = resourceInfo{
					resource: r,
					exists:   true,
				}
			}
			var objs []resourceInfo
			for _, obj := range objMap {
				objs = append(objs, obj)
			}

			handlers := append([]handler{
				&nsHandler{},
				&inventoryObjectHandler{
					inventoryName:      tc.invInfo.name,
					inventoryNamespace: tc.invInfo.namespace,
					inventoryID:        tc.invInfo.id,
					inventoryList:      tc.invInfo.list,
				},
			}, &genericHandler{
				resources: objs,
				mapper:    mapper,
			})

			tf.UnstructuredClient = newFakeRESTClient(t, handlers)
			tf.FakeDynamicClient = fakeDynamicClient(t, mapper, objs...)

			cf := provider.NewProvider(tf)
			applier := NewApplier(cf)
			applier.infoHelper = &fakeInfoHelper{
				factory: tf,
			}

			err = applier.Initialize()
			if !assert.NoError(t, err) {
				return
			}
			// TODO(mortent): This is not great, but at least this keeps the
			// ugliness in the test code until we can find a way to wire it
			// up so to avoid it.
			applier.invClient.(*inventory.ClusterInventoryClient).InfoHelper = applier.infoHelper

			poller := &fakePoller{
				events: tc.statusEvents,
				start:  make(chan struct{}),
			}
			applier.StatusPoller = poller

			ctx := context.Background()
			eventChannel := applier.Run(ctx, tc.invInfo.toWrapped(), tc.resources, Options{
				ReconcileTimeout: tc.reconcileTimeout,
				EmitStatusEvents: true,
				NoPrune:          !tc.prune,
				InventoryPolicy:  tc.inventoryPolicy,
			})

			var events []event.Event
			timer := time.NewTimer(30 * time.Second)

		loop:
			for {
				select {
				case e, ok := <-eventChannel:
					if !ok {
						break loop
					}
					if e.Type == event.ActionGroupType &&
						e.ActionGroupEvent.Action == event.ApplyAction &&
						e.ActionGroupEvent.Type == event.Finished {
						close(poller.start)
					}
					events = append(events, e)
				case <-timer.C:
					t.Errorf("timeout")
					break loop
				}
			}

			err = testutil.VerifyEvents(tc.expectedEvents, events)
			assert.NoError(t, err)
		})
	}
}

var namespace = "test-namespace"

var inventoryObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "test-inventory-obj",
			"namespace": namespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: "test-app-label",
			},
		},
	},
}

var localInv = inventory.WrapInventoryInfoObj(inventoryObj)

var obj1 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "obj1",
			"namespace": namespace,
		},
	},
}

var obj2 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      "obj2",
			"namespace": namespace,
		},
	},
}

var obj3 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "obj3",
			"namespace": "different-namespace",
		},
	},
}

var clusterScopedObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata": map[string]interface{}{
			"name": "cluster-scoped-1",
		},
	},
}

func createNamespace(ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": ns,
			},
		},
	}
}

func TestInventoryNamespaceInSet(t *testing.T) {
	inventoryNamespace := createNamespace(namespace)

	tests := map[string]struct {
		inv       inventory.InventoryInfo
		objects   []*unstructured.Unstructured
		namespace *unstructured.Unstructured
	}{
		"Nil inventory object, no resources returns nil namespace": {
			inv:       nil,
			objects:   []*unstructured.Unstructured{},
			namespace: nil,
		},
		"Inventory object, but no resources returns nil namespace": {
			inv:       localInv,
			objects:   []*unstructured.Unstructured{},
			namespace: nil,
		},
		"Inventory object, resources with no namespace returns nil namespace": {
			inv:       localInv,
			objects:   []*unstructured.Unstructured{obj1, obj2},
			namespace: nil,
		},
		"Inventory object, different namespace returns nil namespace": {
			inv:       localInv,
			objects:   []*unstructured.Unstructured{createNamespace("foo")},
			namespace: nil,
		},
		"Inventory object, inventory namespace returns inventory namespace": {
			inv:       localInv,
			objects:   []*unstructured.Unstructured{obj1, inventoryNamespace, obj3},
			namespace: inventoryNamespace,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actualNamespace := inventoryNamespaceInSet(tc.inv, tc.objects)
			if tc.namespace != actualNamespace {
				t.Fatalf("expected namespace (%v), got (%v)", tc.namespace, actualNamespace)
			}
		})
	}
}

func TestReadAndPrepareObjects(t *testing.T) {
	testCases := map[string]struct {
		// local inventory input into applier.prepareObjects
		inventory inventory.InventoryInfo
		// locally read resources input into applier.prepareObjects
		resources []*unstructured.Unstructured
		// objects already stored in the cluster inventory
		clusterObjs []*unstructured.Unstructured
		// expected returned local inventory object
		localInv inventory.InventoryInfo
		// expected returned local objects to apply (in order)
		localObjs []*unstructured.Unstructured
		// expected calculated prune objects
		pruneObjs []*unstructured.Unstructured
		// expected error
		isError bool
	}{
		"no inventory object": {
			resources: []*unstructured.Unstructured{obj1},
			isError:   true,
		},
		"multiple inventory objects": {
			inventory: localInv,
			resources: []*unstructured.Unstructured{inventoryObj},
			isError:   true,
		},
		"only inventory object": {
			inventory: localInv,
			localInv:  localInv,
			isError:   false,
		},
		"only inventory object, prune one object": {
			inventory:   localInv,
			clusterObjs: []*unstructured.Unstructured{obj1},
			localInv:    localInv,
			pruneObjs:   []*unstructured.Unstructured{obj1},
			isError:     false,
		},
		"inventory object already at the beginning": {
			inventory: localInv,
			resources: []*unstructured.Unstructured{obj1, clusterScopedObj},
			localInv:  localInv,
			localObjs: []*unstructured.Unstructured{obj1, clusterScopedObj},
			isError:   false,
		},
		"inventory object already at the beginning, prune one": {
			inventory:   localInv,
			resources:   []*unstructured.Unstructured{obj1, clusterScopedObj},
			clusterObjs: []*unstructured.Unstructured{obj2},
			localInv:    localInv,
			localObjs:   []*unstructured.Unstructured{obj1, clusterScopedObj},
			pruneObjs:   []*unstructured.Unstructured{obj2},
			isError:     false,
		},
		"inventory object not at the beginning": {
			inventory:   localInv,
			resources:   []*unstructured.Unstructured{obj1, obj2, clusterScopedObj},
			clusterObjs: []*unstructured.Unstructured{obj2},
			localInv:    localInv,
			localObjs:   []*unstructured.Unstructured{obj1, obj2, clusterScopedObj},
			pruneObjs:   []*unstructured.Unstructured{},
			isError:     false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Set up objects already stored in cluster inventory.
			clusterObjs := object.UnstructuredsToObjMetas(tc.clusterObjs)
			fakeInvClient := inventory.NewFakeInventoryClient(clusterObjs)
			// Create applier with fake inventory client, and call prepareObjects
			applier := &Applier{invClient: fakeInvClient}
			resourceObjs, err := applier.prepareObjects(tc.inventory, tc.resources)
			if !tc.isError && err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error, but received none")
				}
				return
			}
			// Validate the returned ResourceObjs
			expected := tc.localInv
			actual := resourceObjs.LocalInv
			if expected.Namespace() != actual.Namespace() ||
				expected.Name() != actual.Name() ||
				expected.ID() != actual.ID() {
				t.Errorf("expected local inventory (%v), got (%v)",
					tc.localInv, resourceObjs.LocalInv)
			}
			if !objSetsEqual(tc.localObjs, resourceObjs.Resources) {
				t.Errorf("expected local infos (%v), got (%v)",
					tc.localObjs, resourceObjs.Resources)
			}
			if len(tc.pruneObjs) != len(resourceObjs.PruneIds) {
				t.Errorf("expected prune ids (%v), got (%v)",
					tc.pruneObjs, resourceObjs.PruneIds)
			}
		})
	}
}

func toJSONBytes(t *testing.T, obj runtime.Object) []byte {
	objBytes, err := runtime.Encode(unstructured.NewJSONFallbackEncoder(codec), obj)
	if !assert.NoError(t, err) {
		t.Fatal(err)
	}
	return objBytes
}

// newFakeRESTClient creates a new client that uses a set of handlers to
// determine how to handle requests. For every request it will iterate through
// the handlers until it can find one that knows how to handle the request.
// This is to keep the main structure of the fake client manageable while still
// allowing different behavior for different testcases.
func newFakeRESTClient(t *testing.T, handlers []handler) *fake.RESTClient {
	return &fake.RESTClient{
		NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			for _, h := range handlers {
				resp, handled, err := h.handle(t, req)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
					return nil, nil
				}
				if handled {
					return resp, nil
				}
			}
			t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
			return nil, nil
		}),
	}
}

func toIdentifier(t *testing.T, manifest string) object.ObjMetadata {
	obj := Unstructured(t, manifest)

	accessor, err := meta.Accessor(obj)
	if err != nil {
		t.Fatal(err)
	}
	return object.ObjMetadata{
		GroupKind: obj.GetObjectKind().GroupVersionKind().GroupKind(),
		Name:      accessor.GetName(),
		Namespace: "default",
	}
}

// The handler interface allows different testcases to provide
// special handling of requests. It also allows a single handler
// to keep state between a set of related requests instead of keeping
// a single large event handler.
type handler interface {
	handle(t *testing.T, req *http.Request) (*http.Response, bool, error)
}

// genericHandler provides a simple handler for resources that can
// be fetched and updated. It will simply return the given resource
// when asked for and accept patch requests.
type genericHandler struct {
	resources []resourceInfo
	mapper    meta.RESTMapper
}

func (g *genericHandler) handle(t *testing.T, req *http.Request) (*http.Response, bool, error) {
	for _, r := range g.resources {
		gvk := r.resource.GroupVersionKind()
		mapping, err := g.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return nil, false, err
		}
		var allPath string
		if mapping.Scope == meta.RESTScopeNamespace {
			allPath = fmt.Sprintf("/namespaces/%s/%s", r.resource.GetNamespace(), mapping.Resource.Resource)
		} else {
			allPath = fmt.Sprintf("/%s", mapping.Resource.Resource)
		}
		singlePath := allPath + "/" + r.resource.GetName()

		if req.URL.Path == singlePath && req.Method == http.MethodGet {
			if r.exists {
				bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, r.resource)))
				return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
			}
			return &http.Response{StatusCode: http.StatusNotFound, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.StringBody("")}, true, nil
		}

		if req.URL.Path == singlePath && req.Method == http.MethodPatch {
			bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, r.resource)))
			return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
		}

		if req.URL.Path == allPath && req.Method == http.MethodPost {
			bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, r.resource)))
			return &http.Response{StatusCode: http.StatusCreated, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
		}
	}
	return nil, false, nil
}

// inventoryObjectHandler knows how to handle requests on the inventory objects.
// It knows how to handle creation, list and get requests for inventory objects.
type inventoryObjectHandler struct {
	inventoryName      string
	inventoryNamespace string
	inventoryID        string
	inventoryList      []object.ObjMetadata
	inventoryObj       *v1.ConfigMap
}

var (
	cmPathRegex     = regexp.MustCompile(`^/namespaces/([^/]+)/configmaps$`)
	invObjPathRegex = regexp.MustCompile(`^/namespaces/([^/]+)/configmaps/[a-zA-Z]+-[a-z0-9]+$`)
)

func (i *inventoryObjectHandler) handle(t *testing.T, req *http.Request) (*http.Response, bool, error) {
	if (req.Method == "POST" && cmPathRegex.Match([]byte(req.URL.Path))) ||
		(req.Method == "PUT" && invObjPathRegex.Match([]byte(req.URL.Path))) {
		b, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, false, err
		}
		cm := v1.ConfigMap{}
		err = runtime.DecodeInto(codec, b, &cm)
		if err != nil {
			return nil, false, err
		}
		if cm.Name == i.inventoryName && cm.Namespace == i.inventoryNamespace {
			i.inventoryObj = &cm
			var inventoryList []object.ObjMetadata
			for s := range cm.Data {
				objMeta, err := object.ParseObjMetadata(s)
				if err != nil {
					return nil, false, err
				}
				inventoryList = append(inventoryList, objMeta)
			}
			i.inventoryList = inventoryList

			bodyRC := ioutil.NopCloser(bytes.NewReader(b))
			var statusCode int
			if req.Method == "POST" {
				statusCode = http.StatusCreated
			} else {
				statusCode = http.StatusOK
			}
			return &http.Response{StatusCode: statusCode, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
		}
		return nil, false, nil
	}

	if req.Method == http.MethodGet && cmPathRegex.Match([]byte(req.URL.Path)) {
		cmList := v1.ConfigMapList{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "List",
			},
			Items: []v1.ConfigMap{},
		}
		if len(i.inventoryList) > 0 {
			cmList.Items = append(cmList.Items, i.currentInvObj())
		}
		bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, &cmList)))
		return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
	}

	if req.Method == http.MethodGet && invObjPathRegex.Match([]byte(req.URL.Path)) {
		if len(i.inventoryList) == 0 {
			return &http.Response{StatusCode: http.StatusNotFound, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.StringBody("")}, true, nil
		}
		invObj := i.currentInvObj()
		bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, &invObj)))
		return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
	}
	return nil, false, nil
}

func (i *inventoryObjectHandler) currentInvObj() v1.ConfigMap {
	inv := make(map[string]string)
	for _, objMeta := range i.inventoryList {
		inv[objMeta.String()] = ""
	}
	return v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.inventoryName,
			Namespace: i.inventoryNamespace,
			Labels: map[string]string{
				common.InventoryLabel: i.inventoryID,
			},
		},
		Data: inv,
	}
}

// nsHandler can handle requests for a namespace. It will behave as if
// every requested namespace exists. It simply fetches the name of the requested
// namespace from the url and creates a new namespace type with the provided
// name for the response.
type nsHandler struct{}

var (
	nsPathRegex = regexp.MustCompile(`/api/v1/namespaces/([^/]+)`)
)

func (n *nsHandler) handle(t *testing.T, req *http.Request) (*http.Response, bool, error) {
	match := nsPathRegex.FindStringSubmatch(req.URL.Path)
	if req.Method == http.MethodGet && match != nil {
		nsName := match[1]
		ns := v1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
			},
		}
		bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, &ns)))
		return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
	}
	return nil, false, nil
}

type fakePoller struct {
	start  chan struct{}
	events []pollevent.Event
}

func (f *fakePoller) Poll(ctx context.Context, _ []object.ObjMetadata, _ polling.Options) <-chan pollevent.Event {
	eventChannel := make(chan pollevent.Event)
	go func() {
		defer close(eventChannel)
		<-f.start
		for _, f := range f.events {
			eventChannel <- f
		}
		<-ctx.Done()
	}()
	return eventChannel
}

type fakeInfoHelper struct {
	factory *cmdtesting.TestFactory
}

// TODO(mortent): This has too much code in common with the
// infoHelper implementation. We need to find a better way to structure
// this.
func (f *fakeInfoHelper) UpdateInfo(info *resource.Info) error {
	mapper, err := f.factory.ToRESTMapper()
	if err != nil {
		return err
	}
	gvk := info.Object.GetObjectKind().GroupVersionKind()
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}
	info.Mapping = mapping

	c, err := f.getClient(gvk.GroupVersion())
	if err != nil {
		return err
	}
	info.Client = c
	return nil
}

func (f *fakeInfoHelper) BuildInfo(obj *unstructured.Unstructured) (*resource.Info, error) {
	info := &resource.Info{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Source:    "unstructured",
		Object:    obj,
	}
	err := f.UpdateInfo(info)
	return info, err
}

func (f *fakeInfoHelper) getClient(gv schema.GroupVersion) (resource.RESTClient, error) {
	if f.factory.UnstructuredClientForMappingFunc != nil {
		return f.factory.UnstructuredClientForMappingFunc(gv)
	}
	if f.factory.UnstructuredClient != nil {
		return f.factory.UnstructuredClient, nil
	}
	return f.factory.Client, nil
}

// infoSetEquals returns true if the set of Infos in setA equals the
// set of Infos in setB (ordering does not matter); false otherwise.
func objSetsEqual(setA []*unstructured.Unstructured, setB []*unstructured.Unstructured) bool {
	if len(setA) != len(setB) {
		return false
	}
	mapA := map[string]bool{}
	objMetasA := object.UnstructuredsToObjMetas(setA)
	for _, objMetaA := range objMetasA {
		mapA[objMetaA.String()] = true
	}
	objMetasB := object.UnstructuredsToObjMetas(setB)
	for _, objMetaB := range objMetasB {
		if _, ok := mapA[objMetaB.String()]; !ok {
			return false
		}
	}
	return true
}

// fakeDynamicClient returns a fake dynamic client.
func fakeDynamicClient(t *testing.T, mapper meta.RESTMapper, objs ...resourceInfo) *dynamicfake.FakeDynamicClient {
	fakeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	for i := range objs {
		obj := objs[i]
		gvk := obj.resource.GroupVersionKind()
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if !assert.NoError(t, err) {
			t.FailNow()
		}
		r := mapping.Resource.Resource
		fakeClient.PrependReactor("get", r, func(clienttesting.Action) (bool, runtime.Object, error) {
			if obj.exists {
				return true, obj.resource, nil
			}
			return false, nil, nil
		})
		fakeClient.PrependReactor("delete", r, func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, nil
		})
	}
	return fakeClient
}
