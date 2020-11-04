// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest/fake"
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
)

var (
	codec     = scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
	resources = map[string]resourceInfo{
		"inventoryObject": {
			manifest: `
  kind: ConfigMap
  apiVersion: v1
  metadata:
    labels:
      cli-utils.sigs.k8s.io/inventory-id: test
    name: foo
    namespace: default
`,
		},
		"deployment": {
			manifest: `
  kind: Deployment
  apiVersion: apps/v1
  metadata:
    name: foo
    namespace: default
  spec:
    replicas: 1
`,
			basePath:    "/namespaces/%s/deployments",
			factoryFunc: func() runtime.Object { return &appsv1.Deployment{} },
		},
	}
)

// resourceStatus contains information about a specific resource, such
// as the manifest yaml and the URL path for this resource in the
// client. It also contains a factory function for creating a new
// resource of the given type.
type resourceInfo struct {
	manifest    string
	basePath    string
	factoryFunc func() runtime.Object
}

type expectedEvent struct {
	eventType event.Type

	applyEventType  event.ApplyEventType
	statusEventType event.StatusEventType
	pruneEventType  event.PruneEventType
	deleteEventType event.DeleteEventType
}

func TestApplier(t *testing.T) {
	testCases := map[string]struct {
		namespace          string
		resources          []resourceInfo
		handlers           []handler
		reconcileTimeout   time.Duration
		prune              bool
		statusEvents       []pollevent.Event
		expectedEventTypes []expectedEvent
	}{
		"apply without status or prune": {
			namespace: "default",
			resources: []resourceInfo{
				resources["deployment"],
				resources["inventoryObject"],
			},
			handlers: []handler{
				&nsHandler{},
				&inventoryObjectHandler{},
				&genericHandler{
					resourceInfo: resources["deployment"],
					namespace:    "default",
				},
			},
			reconcileTimeout: time.Duration(0),
			prune:            false,
			expectedEventTypes: []expectedEvent{
				{
					eventType: event.InitType,
				},
				{
					eventType:      event.ApplyType,
					applyEventType: event.ApplyEventResourceUpdate,
				},
				{
					eventType:      event.ApplyType,
					applyEventType: event.ApplyEventCompleted,
				},
			},
		},
		"first apply with inventory object": {
			namespace: "default",
			resources: []resourceInfo{
				resources["deployment"],
				resources["inventoryObject"],
			},
			handlers: []handler{
				&nsHandler{},
				&inventoryObjectHandler{},
				&genericHandler{
					resourceInfo: resources["deployment"],
					namespace:    "default",
				},
			},
			reconcileTimeout: time.Minute,
			statusEvents: []pollevent.Event{
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: object.ObjMetadata{
							Name:      "foo-dcf2c498",
							Namespace: "default",
							GroupKind: schema.GroupKind{
								Group: "",
								Kind:  "ConfigMap",
							},
						},
						Status: status.CurrentStatus,
					},
				},
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["deployment"], "default"),
						Status:     status.InProgressStatus,
					},
				},
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["deployment"], "default"),
						Status:     status.CurrentStatus,
					},
				},
			},
			expectedEventTypes: []expectedEvent{
				{
					eventType: event.InitType,
				},
				{
					eventType:      event.ApplyType,
					applyEventType: event.ApplyEventResourceUpdate,
				},
				{
					eventType:      event.ApplyType,
					applyEventType: event.ApplyEventCompleted,
				},
				{
					eventType:       event.StatusType,
					statusEventType: event.StatusEventResourceUpdate,
				},
				{
					eventType:       event.StatusType,
					statusEventType: event.StatusEventResourceUpdate,
				},
				{
					eventType:       event.StatusType,
					statusEventType: event.StatusEventResourceUpdate,
				},
				{
					eventType:       event.StatusType,
					statusEventType: event.StatusEventCompleted,
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			inv, infos, err := createObjs(tc.resources)
			assert.NoError(t, err)

			tf := cmdtesting.NewTestFactory().WithNamespace(tc.namespace)
			defer tf.Cleanup()

			tf.UnstructuredClient = newFakeRESTClient(t, tc.handlers)

			cf := provider.NewProvider(tf)
			applier := NewApplier(cf)
			applier.infoHelper = &fakeInfoHelper{
				factory: tf,
			}

			err = applier.Initialize()
			if !assert.NoError(t, err) {
				return
			}
			applier.invClient = inventory.NewFakeInventoryClient([]object.ObjMetadata{})

			poller := &fakePoller{
				events: tc.statusEvents,
				start:  make(chan struct{}),
			}
			applier.StatusPoller = poller

			ctx := context.Background()
			eventChannel := applier.Run(ctx, inv, infos, Options{
				ReconcileTimeout: tc.reconcileTimeout,
				EmitStatusEvents: true,
				NoPrune:          !tc.prune,
			})

			var events []event.Event
			for e := range eventChannel {
				if e.Type == event.ApplyType && e.ApplyEvent.Type == event.ApplyEventCompleted {
					close(poller.start)
				}
				events = append(events, e)
			}

			assert.Equal(t, len(tc.expectedEventTypes), len(events))

			for i, e := range events {
				expected := tc.expectedEventTypes[i]
				assert.Equal(t, expected.eventType.String(), e.Type.String())

				switch expected.eventType {
				case event.InitType:
				case event.ApplyType:
					assert.Equal(t, expected.applyEventType.String(), e.ApplyEvent.Type.String())
				case event.StatusType:
					assert.Equal(t, expected.statusEventType.String(), e.StatusEvent.Type.String())
				case event.PruneType:
					assert.Equal(t, expected.pruneEventType.String(), e.PruneEvent.Type.String())
				case event.DeleteType:
					assert.Equal(t, expected.deleteEventType.String(), e.DeleteEvent.Type.String())
				default:
					assert.Fail(t, "unexpected event type %s", expected.eventType.String())
				}
			}
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
		inv       *unstructured.Unstructured
		objects   []*unstructured.Unstructured
		namespace *unstructured.Unstructured
	}{
		"Nil inventory object, no resources returns nil namespace": {
			inv:       nil,
			objects:   []*unstructured.Unstructured{},
			namespace: nil,
		},
		"Inventory object, but no resources returns nil namespace": {
			inv:       inventoryObj,
			objects:   []*unstructured.Unstructured{},
			namespace: nil,
		},
		"Inventory object, resources with no namespace returns nil namespace": {
			inv:       inventoryObj,
			objects:   []*unstructured.Unstructured{obj1, obj2},
			namespace: nil,
		},
		"Inventory object, different namespace returns nil namespace": {
			inv:       inventoryObj,
			objects:   []*unstructured.Unstructured{createNamespace("foo")},
			namespace: nil,
		},
		"Inventory object, inventory namespace returns inventory namespace": {
			inv:       inventoryObj,
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
		inventory *unstructured.Unstructured
		// locally read resources input into applier.prepareObjects
		resources []*unstructured.Unstructured
		// objects already stored in the cluster inventory
		clusterObjs []*unstructured.Unstructured
		// expected returned local inventory object
		localInv *unstructured.Unstructured
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
			inventory: inventoryObj,
			resources: []*unstructured.Unstructured{inventoryObj},
			isError:   true,
		},
		"only inventory object": {
			inventory: inventoryObj,
			localInv:  inventoryObj,
			isError:   false,
		},
		"only inventory object, prune one object": {
			inventory:   inventoryObj,
			clusterObjs: []*unstructured.Unstructured{obj1},
			localInv:    inventoryObj,
			pruneObjs:   []*unstructured.Unstructured{obj1},
			isError:     false,
		},
		"inventory object already at the beginning": {
			inventory: inventoryObj,
			resources: []*unstructured.Unstructured{obj1, clusterScopedObj},
			localInv:  inventoryObj,
			localObjs: []*unstructured.Unstructured{obj1, clusterScopedObj},
			isError:   false,
		},
		"inventory object already at the beginning, prune one": {
			inventory:   inventoryObj,
			resources:   []*unstructured.Unstructured{obj1, clusterScopedObj},
			clusterObjs: []*unstructured.Unstructured{obj2},
			localInv:    inventoryObj,
			localObjs:   []*unstructured.Unstructured{obj1, clusterScopedObj},
			pruneObjs:   []*unstructured.Unstructured{obj2},
			isError:     false,
		},
		"inventory object not at the beginning": {
			inventory:   inventoryObj,
			resources:   []*unstructured.Unstructured{obj1, obj2, clusterScopedObj},
			clusterObjs: []*unstructured.Unstructured{obj2},
			localInv:    inventoryObj,
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
			if tc.localInv.GetNamespace() != resourceObjs.LocalInv.GetNamespace() ||
				tc.localInv.GetName() != resourceObjs.LocalInv.GetName() {
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

func toIdentifier(t *testing.T, resourceInfo resourceInfo, namespace string) object.ObjMetadata {
	obj := resourceInfo.factoryFunc()
	err := runtime.DecodeInto(codec, []byte(resourceInfo.manifest), obj)
	if err != nil {
		t.Fatal(err)
	}

	accessor, err := meta.Accessor(obj)
	if err != nil {
		t.Fatal(err)
	}
	return object.ObjMetadata{
		GroupKind: obj.GetObjectKind().GroupVersionKind().GroupKind(),
		Name:      accessor.GetName(),
		Namespace: namespace,
	}
}

func createObjs(resources []resourceInfo) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	var objs []*unstructured.Unstructured
	for _, ri := range resources {
		u := &unstructured.Unstructured{}
		err := runtime.DecodeInto(codec, []byte(ri.manifest), u)
		if err != nil {
			return nil, nil, err
		}
		objs = append(objs, u)
	}
	return inventory.SplitUnstructureds(objs)
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
	resourceInfo resourceInfo
	namespace    string
}

func (g *genericHandler) handle(t *testing.T, req *http.Request) (*http.Response, bool, error) {
	obj := g.resourceInfo.factoryFunc()
	err := runtime.DecodeInto(codec, []byte(g.resourceInfo.manifest), obj)
	if err != nil {
		return nil, false, err
	}

	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, false, err
	}

	basePath := fmt.Sprintf(g.resourceInfo.basePath, g.namespace)
	resourcePath := path.Join(basePath, accessor.GetName())

	if req.URL.Path == resourcePath && req.Method == http.MethodGet {
		bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, obj)))
		return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
	}

	if req.URL.Path == resourcePath && req.Method == http.MethodPatch {
		bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, obj)))
		return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
	}
	return nil, false, nil
}

// inventoryObjectHandler knows how to handle requests on the inventory objects.
// It knows how to handle creation, list and get requests for inventory objects.
type inventoryObjectHandler struct {
	inventoryObj *v1.ConfigMap
}

var (
	cmPathRegex     = regexp.MustCompile(`^/namespaces/([^/]+)/configmaps$`)
	invObjNameRegex = regexp.MustCompile(`^[a-zA-Z]+-[a-z0-9]+$`)
	invObjPathRegex = regexp.MustCompile(`^/namespaces/([^/]+)/configmaps/[a-zA-Z]+-[a-z0-9]+$`)
)

func (i *inventoryObjectHandler) handle(t *testing.T, req *http.Request) (*http.Response, bool, error) {
	if req.Method == "POST" && cmPathRegex.Match([]byte(req.URL.Path)) {
		b, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, false, err
		}
		cm := v1.ConfigMap{}
		err = runtime.DecodeInto(codec, b, &cm)
		if err != nil {
			return nil, false, err
		}
		if invObjNameRegex.Match([]byte(cm.Name)) {
			i.inventoryObj = &cm
			bodyRC := ioutil.NopCloser(bytes.NewReader(b))
			return &http.Response{StatusCode: http.StatusCreated, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
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
		if i.inventoryObj != nil {
			cmList.Items = append(cmList.Items, *i.inventoryObj)
		}
		bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, &cmList)))
		return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
	}

	if req.Method == http.MethodGet && invObjPathRegex.Match([]byte(req.URL.Path)) {
		if i.inventoryObj == nil {
			return &http.Response{StatusCode: http.StatusNotFound, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.StringBody("")}, true, nil
		}
		bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, i.inventoryObj)))
		return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
	}
	return nil, false, nil
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
func (f *fakeInfoHelper) UpdateInfos(infos []*resource.Info) error {
	mapper, err := f.factory.ToRESTMapper()
	if err != nil {
		return err
	}
	for _, info := range infos {
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
	}
	return nil
}

func (f *fakeInfoHelper) BuildInfos(objs []*unstructured.Unstructured) ([]*resource.Info, error) {
	var infos []*resource.Info
	for _, obj := range objs {
		infos = append(infos, &resource.Info{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			Source:    "unstructured",
			Object:    obj,
		})
	}
	err := f.UpdateInfos(infos)
	if err != nil {
		return nil, err
	}
	return infos, nil
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
