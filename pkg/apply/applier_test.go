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
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
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

// resourceInfo contains information about a specific resource, such
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
				{
					EventType:       pollevent.CompletedEvent,
					AggregateStatus: status.CurrentStatus,
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

			applier.statusPoller = &fakePoller{
				events: tc.statusEvents,
			}

			ctx := context.Background()
			eventChannel := applier.Run(ctx)

			var events []event.Event
			for e := range eventChannel {
				events = append(events, e)
			}

			assert.Equal(t, len(tc.expectedEventTypes), len(events))

			for i, e := range events {
				expected := tc.expectedEventTypes[i]
				assert.Equal(t, expected.eventType, e.Type)

				switch expected.eventType {
				case event.ApplyType:
					assert.Equal(t, expected.applyEventType, e.ApplyEvent.Type)
				case event.StatusType:
					assert.Equal(t, expected.statusEventType, e.StatusEvent.EventType)
				case event.PruneType:
					assert.Equal(t, expected.pruneEventType, e.PruneEvent.Type)
				case event.DeleteType:
					assert.Equal(t, expected.deleteEventType, e.DeleteEvent.Type)
				default:
					assert.Fail(t, "unexpected event type %s", expected.eventType.String())
				}
			}
		})
	}
}

var namespace = "test-namespace"

var obj1Info = &resource.Info{
	Namespace: namespace,
	Name:      "foo",
}

var obj2Info = &resource.Info{
	Namespace: namespace,
	Name:      "bar",
}

var obj3Info = &resource.Info{
	Namespace: namespace,
	Name:      "baz",
}

var obj4Info = &resource.Info{
	Namespace: "wrong",
	Name:      "diff",
}

var obj5Info = &resource.Info{
	Namespace: "",
	Name:      "diff",
}

var obj6Info = &resource.Info{
	Namespace: "default",
	Name:      "diff",
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
		"Three resources with same namespace is valid": {
			objects: []*resource.Info{obj1Info, obj2Info, obj3Info},
			isValid: true,
		},
		"Empty namespace is equal to default namespace": {
			objects: []*resource.Info{obj5Info, obj6Info},
			isValid: true,
		},
		"Two resources with differing namespaces is not valid": {
			objects: []*resource.Info{obj1Info, obj4Info},
			isValid: false,
		},
		"Three resources, one with differing namespace is not valid": {
			objects: []*resource.Info{obj1Info, obj4Info, obj3Info},
			isValid: false,
		},
		"Empty namespace not equal to other namespaces": {
			objects: []*resource.Info{obj5Info, obj3Info},
			isValid: false,
		},
		"Default namespace not equal to other namespaces": {
			objects: []*resource.Info{obj6Info, obj3Info},
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
	events []pollevent.Event
}

func (f *fakePoller) Poll(_ context.Context, _ []object.ObjMetadata, _ polling.Options) <-chan pollevent.Event {
	eventChannel := make(chan pollevent.Event)
	go func() {
		defer close(eventChannel)
		for _, f := range f.events {
			eventChannel <- f
		}
	}()
	return eventChannel
}
