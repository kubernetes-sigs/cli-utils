package observers

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/observer"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type newResourceObserverFunc func(reader observer.ClusterReader, mapper meta.RESTMapper) observer.ResourceObserver

func basicObserverTest(t *testing.T, manifest string, generatedGVK schema.GroupVersionKind,
	generatedObjects []unstructured.Unstructured, newObserverFunc newResourceObserverFunc) {
	testCases := map[string]struct {
		manifest       string
		generatedGVK   schema.GroupVersionKind
		listObjects    []unstructured.Unstructured
		statusResult   *status.Result
		statusErr      error
		expectedStatus status.Status
		expectError    bool
		errMessage     string
	}{
		"observer has generated resources": {
			manifest:     manifest,
			generatedGVK: generatedGVK,
			listObjects:  generatedObjects,
			statusResult: &status.Result{
				Status: status.CurrentStatus,
			},
			expectedStatus: status.CurrentStatus,
			expectError:    false,
		},
		"looking up listObjects fails": {
			manifest: manifest,
			statusResult: &status.Result{
				Status: status.CurrentStatus,
			},
			expectedStatus: status.UnknownStatus,
			expectError:    true,
			errMessage:     "no matches for",
		},
		"computing status fails": {
			manifest:     manifest,
			generatedGVK: generatedGVK,
			statusResult: &status.Result{
				Status: status.CurrentStatus,
			},
			statusErr:      fmt.Errorf("this is a test"),
			expectedStatus: status.UnknownStatus,
			expectError:    true,
			errMessage:     "this is a test",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			fakeReader := &fakeObserverReader{
				listResources: &unstructured.UnstructuredList{
					Items: tc.listObjects,
				},
			}
			fakeMapper := testutil.NewFakeRESTMapper(tc.generatedGVK)

			object := testutil.YamlToUnstructured(t, tc.manifest)

			observer := newObserverFunc(fakeReader, fakeMapper)
			observer.SetComputeStatusFunc(func(u *unstructured.Unstructured) (*status.Result, error) {
				return tc.statusResult, tc.statusErr
			})

			observedResource := observer.ObserveObject(context.Background(), object)

			if tc.expectError {
				if observedResource.Error == nil {
					t.Errorf("expected error, but didn't get one")
				}
				assert.ErrorContains(t, observedResource.Error, tc.errMessage)
				return
			}

			assert.Equal(t, tc.expectedStatus, observedResource.Status)
			assert.Equal(t, len(tc.listObjects), len(observedResource.GeneratedResources))
			assert.Assert(t, sort.IsSorted(observedResource.GeneratedResources))
		})
	}
}

type fakeObserverReader struct {
	testutil.NoopObserverReader

	getResource *unstructured.Unstructured
	getErr      error

	listResources *unstructured.UnstructuredList
	listErr       error
}

func (f *fakeObserverReader) Get(_ context.Context, key client.ObjectKey, u *unstructured.Unstructured) error {
	if f.getResource != nil {
		u.Object = f.getResource.Object
	}
	return f.getErr
}

func (f *fakeObserverReader) ListNamespaceScoped(_ context.Context, list *unstructured.UnstructuredList, ns string, selector labels.Selector) error {
	if f.listResources != nil {
		list.Items = f.listResources.Items
	}
	return f.listErr
}

type fakeResourceObserver struct{}

func (f *fakeResourceObserver) Observe(ctx context.Context, resource wait.ResourceIdentifier) *event.ObservedResource {
	return nil
}

func (f *fakeResourceObserver) ObserveObject(ctx context.Context, object *unstructured.Unstructured) *event.ObservedResource {
	identifier := toIdentifier(object)
	return &event.ObservedResource{
		Identifier: identifier,
	}
}

func (f *fakeResourceObserver) SetComputeStatusFunc(_ observer.ComputeStatusFunc) {}
