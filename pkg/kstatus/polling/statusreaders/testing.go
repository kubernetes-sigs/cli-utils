// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

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
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type newStatusReaderFunc func(reader engine.ClusterReader, mapper meta.RESTMapper) engine.StatusReader

func basicStatusReaderTest(t *testing.T, manifest string, generatedGVK schema.GroupVersionKind,
	generatedObjects []unstructured.Unstructured, newStatusReaderFunc newStatusReaderFunc) {
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
		"engine has generated resources": {
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
			fakeReader := &fakeClusterReader{
				listResources: &unstructured.UnstructuredList{
					Items: tc.listObjects,
				},
			}
			fakeMapper := testutil.NewFakeRESTMapper(tc.generatedGVK)

			object := testutil.YamlToUnstructured(t, tc.manifest)

			statusReader := newStatusReaderFunc(fakeReader, fakeMapper)
			statusReader.SetComputeStatusFunc(func(u *unstructured.Unstructured) (*status.Result, error) {
				return tc.statusResult, tc.statusErr
			})

			resourceStatus := statusReader.ReadStatusForObject(context.Background(), object)

			if tc.expectError {
				if resourceStatus.Error == nil {
					t.Errorf("expected error, but didn't get one")
				}
				assert.ErrorContains(t, resourceStatus.Error, tc.errMessage)
				return
			}

			assert.Equal(t, tc.expectedStatus, resourceStatus.Status)
			assert.Equal(t, len(tc.listObjects), len(resourceStatus.GeneratedResources))
			assert.Assert(t, sort.IsSorted(resourceStatus.GeneratedResources))
		})
	}
}

type fakeClusterReader struct {
	testutil.NoopClusterReader

	getResource *unstructured.Unstructured
	getErr      error

	listResources *unstructured.UnstructuredList
	listErr       error
}

func (f *fakeClusterReader) Get(_ context.Context, _ client.ObjectKey, u *unstructured.Unstructured) error {
	if f.getResource != nil {
		u.Object = f.getResource.Object
	}
	return f.getErr
}

func (f *fakeClusterReader) ListNamespaceScoped(_ context.Context, list *unstructured.UnstructuredList, _ string, _ labels.Selector) error {
	if f.listResources != nil {
		list.Items = f.listResources.Items
	}
	return f.listErr
}

type fakeStatusReader struct{}

func (f *fakeStatusReader) ReadStatus(_ context.Context, _ object.ObjMetadata) *event.ResourceStatus {
	return nil
}

func (f *fakeStatusReader) ReadStatusForObject(_ context.Context, object *unstructured.Unstructured) *event.ResourceStatus {
	identifier := toIdentifier(object)
	return &event.ResourceStatus{
		Identifier: identifier,
	}
}

func (f *fakeStatusReader) SetComputeStatusFunc(_ engine.ComputeStatusFunc) {}
