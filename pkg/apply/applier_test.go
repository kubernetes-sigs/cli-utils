// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var (
	codec     = scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
	resources = map[string]resourceInfo{
		"groupingObject": {
			manifest: `
  kind: ConfigMap
  apiVersion: v1
  metadata:
    labels:
      cli-utils.sigs.k8s.io/inventory-id: test
    name: foo
`,
			fileName: "groupingObject.yaml",
		},
		"deployment": {
			manifest: `
  kind: Deployment
  apiVersion: apps/v1
  metadata:
    name: foo
  spec:
    replicas: 1
`,
			fileName:    "deployment.yaml",
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
	fileName    string
	basePath    string
	factoryFunc func() runtime.Object
}

type expectedEvent struct {
	eventType event.Type

	applyEventType  event.ApplyEventType
	statusEventType pollevent.EventType
	pruneEventType  event.PruneEventType
	deleteEventType event.DeleteEventType
}

func TestApplier(t *testing.T) {
	testCases := map[string]struct {
		namespace          string
		resources          []resourceInfo
		handlers           []handler
		status             bool
		prune              bool
		statusEvents       []pollevent.Event
		expectedEventTypes []expectedEvent
	}{
		"apply without status or prune": {
			namespace: "apply-test",
			resources: []resourceInfo{
				resources["deployment"],
				resources["groupingObject"],
			},
			handlers: []handler{
				&nsHandler{},
				&groupingObjectHandler{},
				&genericHandler{
					resourceInfo: resources["deployment"],
					namespace:    "apply-test",
				},
			},
			status: false,
			prune:  false,
			expectedEventTypes: []expectedEvent{
				{
					eventType:      event.ApplyType,
					applyEventType: event.ApplyEventResourceUpdate,
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
		"first apply with grouping object": {
			namespace: "apply-test",
			resources: []resourceInfo{
				resources["deployment"],
				resources["groupingObject"],
			},
			handlers: []handler{
				&nsHandler{},
				&groupingObjectHandler{},
				&genericHandler{
					resourceInfo: resources["deployment"],
					namespace:    "apply-test",
				},
			},
			status: true,
			statusEvents: []pollevent.Event{
				{
					EventType:       pollevent.ResourceUpdateEvent,
					AggregateStatus: status.InProgressStatus,
					Resource: &pollevent.ResourceStatus{
						Identifier: object.ObjMetadata{
							Name:      "foo-91afd0fc",
							Namespace: "apply-test",
							GroupKind: schema.GroupKind{
								Group: "",
								Kind:  "ConfigMap",
							},
						},
						Status: status.CurrentStatus,
					},
				},
				{
					EventType:       pollevent.ResourceUpdateEvent,
					AggregateStatus: status.InProgressStatus,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["deployment"], "apply-test"),
						Status:     status.InProgressStatus,
					},
				},
				{
					EventType:       pollevent.ResourceUpdateEvent,
					AggregateStatus: status.CurrentStatus,
					Resource: &pollevent.ResourceStatus{
						Identifier: toIdentifier(t, resources["deployment"], "apply-test"),
						Status:     status.CurrentStatus,
					},
				},
			},
			expectedEventTypes: []expectedEvent{
				{
					eventType:      event.ApplyType,
					applyEventType: event.ApplyEventResourceUpdate,
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
					statusEventType: pollevent.ResourceUpdateEvent,
				},
				{
					eventType:       event.StatusType,
					statusEventType: pollevent.ResourceUpdateEvent,
				},
				{
					eventType:       event.StatusType,
					statusEventType: pollevent.ResourceUpdateEvent,
				},
				{
					eventType:       event.StatusType,
					statusEventType: pollevent.CompletedEvent,
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			dirPath, cleanup, err := writeResourceManifests(tc.resources)
			if !assert.NoError(t, err) {
				return
			}
			defer cleanup()

			tf := cmdtesting.NewTestFactory().WithNamespace(tc.namespace)
			defer tf.Cleanup()

			tf.UnstructuredClient = newFakeRESTClient(t, tc.handlers)

			ioStreams, _, _, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			applier := NewApplier(tf, ioStreams)

			applier.StatusOptions.period = 2 * time.Second
			applier.StatusOptions.wait = tc.status
			applier.NoPrune = !tc.prune

			cmd := &cobra.Command{}
			_ = applier.SetFlags(cmd)
			cmd.Flags().BoolVar(&applier.DryRun, "dry-run", applier.DryRun, "")
			cmdutil.AddValidateFlags(cmd)
			cmdutil.AddServerSideApplyFlags(cmd)
			err = applier.Initialize(cmd, []string{dirPath})
			if !assert.NoError(t, err) {
				return
			}

			poller := &fakePoller{
				events: tc.statusEvents,
				start:  make(chan struct{}),
			}
			applier.statusPoller = poller

			ctx := context.Background()
			eventChannel := applier.Run(ctx)

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
				case event.ApplyType:
					assert.Equal(t, expected.applyEventType.String(), e.ApplyEvent.Type.String())
				case event.StatusType:
					assert.Equal(t, expected.statusEventType.String(), e.StatusEvent.EventType.String())
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

var groupingObjInfo = &resource.Info{
	Namespace: namespace,
	Name:      "test-grouping-obj",
	Object: &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-grouping-obj",
				"namespace": namespace,
				"labels": map[string]interface{}{
					prune.GroupingLabel: "test-app-label",
				},
			},
		},
	},
}

var obj1Info = &resource.Info{
	Namespace: namespace,
	Name:      "obj1",
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "obj1",
				"namespace": namespace,
			},
		},
	},
}

var obj2Info = &resource.Info{
	Namespace: namespace,
	Name:      "obj2",
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      "obj2",
				"namespace": namespace,
			},
		},
	},
}

var obj3Info = &resource.Info{
	Namespace: "different-namespace",
	Name:      "obj3",
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "obj3",
				"namespace": "different-namespace",
			},
		},
	},
}

var defaultObjInfo = &resource.Info{
	Namespace: metav1.NamespaceDefault,
	Name:      "default-obj",
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "default-obj",
				"namespace": metav1.NamespaceDefault,
			},
		},
	},
}

var clusterScopedObjInfo = &resource.Info{
	Name: "cluster-scoped-1",
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeRoot,
	},
	Object: &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "cluster-scoped-1",
			},
		},
	},
}

var clusterScopedObj2Info = &resource.Info{
	Name: "cluster-scoped-2",
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeRoot,
	},
	Object: &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name": "cluster-scoped-2",
			},
		},
	},
}

func TestValidateNamespace(t *testing.T) {
	tests := map[string]struct {
		objects []*resource.Info
		isValid bool
	}{
		"No resources is valid": {
			objects: []*resource.Info{},
			isValid: true,
		},
		"One resource is valid": {
			objects: []*resource.Info{obj1Info},
			isValid: true,
		},
		"Two resources with same namespace is valid": {
			objects: []*resource.Info{obj1Info, obj2Info},
			isValid: true,
		},
		"Two resources with same namespace and cluster-scoped obj is valid": {
			objects: []*resource.Info{obj1Info, clusterScopedObjInfo, obj2Info},
			isValid: true,
		},
		"Single cluster-scoped obj is valid": {
			objects: []*resource.Info{clusterScopedObjInfo},
			isValid: true,
		},
		"Multiple cluster-scoped objs is valid": {
			objects: []*resource.Info{clusterScopedObjInfo, clusterScopedObj2Info},
			isValid: true,
		},
		"Two resources with differing namespaces is not valid": {
			objects: []*resource.Info{obj1Info, obj3Info},
			isValid: false,
		},
		"Two resources with differing namespaces and cluster-scoped obj is not valid": {
			objects: []*resource.Info{clusterScopedObjInfo, obj1Info, obj3Info},
			isValid: false,
		},
		"Three resources, one with differing namespace is not valid": {
			objects: []*resource.Info{obj1Info, obj2Info, obj3Info},
			isValid: false,
		},
		"Default namespace not equal to other namespaces": {
			objects: []*resource.Info{obj3Info, defaultObjInfo},
			isValid: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actualValid := validateNamespace(tc.objects)
			if tc.isValid != actualValid {
				t.Errorf("Expected valid namespace (%t), got (%t)", tc.isValid, actualValid)
			}
		})
	}
}

func TestReadAndPrepareObjects(t *testing.T) {
	testCases := map[string]struct {
		resources     []*resource.Info
		expectedError bool
	}{
		"no grouping object": {
			resources:     []*resource.Info{obj1Info},
			expectedError: true,
		},
		"multiple grouping objects": {
			resources:     []*resource.Info{groupingObjInfo, groupingObjInfo},
			expectedError: true,
		},
		"only grouping object": {
			resources:     []*resource.Info{groupingObjInfo},
			expectedError: false,
		},
		"grouping object already at the beginning": {
			resources: []*resource.Info{groupingObjInfo, obj1Info,
				clusterScopedObjInfo},
			expectedError: false,
		},
		"grouping object not at the beginning": {
			resources: []*resource.Info{obj1Info, obj2Info, groupingObjInfo,
				clusterScopedObjInfo},
			expectedError: false,
		},
		"objects can not be in different namespaces": {
			resources: []*resource.Info{obj1Info, obj2Info,
				groupingObjInfo, obj3Info},
			expectedError: true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("namespace")
			defer tf.Cleanup()

			ioStreams, _, _, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			applier := NewApplier(tf, ioStreams)

			applier.ApplyOptions.SetObjects(tc.resources)

			objects, err := applier.readAndPrepareObjects()

			if tc.expectedError {
				if err == nil {
					t.Errorf("expected error, but didn't get one")
				}
				return
			}

			if !tc.expectedError && err != nil {
				t.Errorf("didn't expect error, but got %v", err)
				return
			}

			groupingObj := objects[0]
			if !prune.IsGroupingObject(groupingObj.Object) {
				t.Errorf(
					"expected first item to be grouping object, but it wasn't")
			}

			inventory, err := prune.RetrieveInventoryFromGroupingObj(
				[]*resource.Info{groupingObj})
			if err != nil {
				t.Error(err)
			}

			if want, got := len(tc.resources)-1, len(inventory); want != got {
				t.Errorf("expected %d resources in inventory, got %d", want, got)
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

func writeResourceManifests(resources []resourceInfo) (string, func(), error) {
	d, err := ioutil.TempDir("", "kustomize-apply-test")
	cleanup := func() { _ = os.RemoveAll(d) }
	if err != nil {
		cleanup()
		return d, cleanup, err
	}
	for _, r := range resources {
		p := filepath.Join(d, r.fileName)
		err = ioutil.WriteFile(p, []byte(r.manifest), 0600)
		if err != nil {
			cleanup()
			return d, cleanup, err
		}
	}
	return d, cleanup, nil
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

// groupingObjectHandler knows how to handle requests on the grouping objects.
// It knows how to handle creation, list and get requests for grouping objects.
type groupingObjectHandler struct {
	groupingObj *v1.ConfigMap
}

var (
	cmPathRegex       = regexp.MustCompile(`^/namespaces/([^/]+)/configmaps$`)
	groupObjNameRegex = regexp.MustCompile(`^[a-zA-Z]+-[a-z0-9]+$`)
	groupObjPathRegex = regexp.MustCompile(`^/namespaces/([^/]+)/configmaps/[a-zA-Z]+-[a-z0-9]+$`)
)

func (g *groupingObjectHandler) handle(t *testing.T, req *http.Request) (*http.Response, bool, error) {
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
		if groupObjNameRegex.Match([]byte(cm.Name)) {
			g.groupingObj = &cm
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
		if g.groupingObj != nil {
			cmList.Items = append(cmList.Items, *g.groupingObj)
		}
		bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, &cmList)))
		return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, true, nil
	}

	if req.Method == http.MethodGet && groupObjPathRegex.Match([]byte(req.URL.Path)) {
		if g.groupingObj == nil {
			return &http.Response{StatusCode: http.StatusNotFound, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.StringBody("")}, true, nil
		}
		bodyRC := ioutil.NopCloser(bytes.NewReader(toJSONBytes(t, g.groupingObj)))
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
