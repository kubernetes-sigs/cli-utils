// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package clusterreader

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	deploymentGVK = appsv1.SchemeGroupVersion.WithKind("Deployment")
	rsGVK         = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	podGVK        = v1.SchemeGroupVersion.WithKind("Pod")
)

func TestSync(t *testing.T) {
	testCases := map[string]struct {
		identifiers    []object.ObjMetadata
		expectedSynced []gvkNamespace
	}{
		"no identifiers": {
			identifiers: []object.ObjMetadata{},
		},
		"same GVK in multiple namespaces": {
			identifiers: []object.ObjMetadata{
				{
					GroupKind: deploymentGVK.GroupKind(),
					Name:      "deployment",
					Namespace: "Foo",
				},
				{
					GroupKind: deploymentGVK.GroupKind(),
					Name:      "deployment",
					Namespace: "Bar",
				},
			},
			expectedSynced: []gvkNamespace{
				{
					GVK:       deploymentGVK,
					Namespace: "Foo",
				},
				{
					GVK:       rsGVK,
					Namespace: "Foo",
				},
				{
					GVK:       podGVK,
					Namespace: "Foo",
				},
				{
					GVK:       deploymentGVK,
					Namespace: "Bar",
				},
				{
					GVK:       rsGVK,
					Namespace: "Bar",
				},
				{
					GVK:       podGVK,
					Namespace: "Bar",
				},
			},
		},
	}

	fakeMapper := testutil.NewFakeRESTMapper(
		appsv1.SchemeGroupVersion.WithKind("Deployment"),
		appsv1.SchemeGroupVersion.WithKind("ReplicaSet"),
		v1.SchemeGroupVersion.WithKind("Pod"),
	)

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			fakeReader := &fakeReader{}

			clusterReader, err := NewCachingClusterReader(fakeReader, fakeMapper, tc.identifiers)
			assert.NilError(t, err)

			err = clusterReader.Sync(context.Background())
			assert.NilError(t, err)

			synced := fakeReader.syncedGVKNamespaces
			sortGVKNamespaces(synced)
			expectedSynced := tc.expectedSynced
			sortGVKNamespaces(expectedSynced)
			assert.DeepEqual(t, expectedSynced, synced)

			assert.Equal(t, len(tc.expectedSynced), len(clusterReader.cache))
		})
	}
}

func TestSync_Errors(t *testing.T) {
	testCases := map[string]struct {
		readerError     error
		expectSyncError bool
	}{
		"reader returns NotFound error": {
			readerError: errors.NewNotFound(schema.GroupResource{
				Group:    "apiextensions.k8s.io",
				Resource: "customresourcedefinitions",
			}, "my-crd"),
			expectSyncError: false,
		},
		"reader returns other error": {
			readerError:     errors.NewInternalError(fmt.Errorf("testing")),
			expectSyncError: true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			identifiers := []object.ObjMetadata{
				{
					Name: "my-crd",
					GroupKind: schema.GroupKind{
						Group: "apiextensions.k8s.io",
						Kind:  "CustomResourceDefinition",
					},
				},
			}

			fakeMapper := testutil.NewFakeRESTMapper(
				apiextv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"),
			)

			fakeReader := &fakeReader{
				err: tc.readerError,
			}

			clusterReader, err := NewCachingClusterReader(fakeReader, fakeMapper, identifiers)
			assert.NilError(t, err)

			err = clusterReader.Sync(context.Background())

			if tc.expectSyncError {
				assert.Equal(t, tc.readerError, err)
				return
			}
			assert.NilError(t, err)

			cacheEntry, found := clusterReader.cache[gvkNamespace{
				GVK: apiextv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"),
			}]
			assert.Check(t, found)
			assert.ErrorContains(t, cacheEntry.err, "not found")
		})
	}
}

func sortGVKNamespaces(gvkNamespaces []gvkNamespace) {
	sort.Slice(gvkNamespaces, func(i, j int) bool {
		if gvkNamespaces[i].GVK.String() != gvkNamespaces[j].GVK.String() {
			return gvkNamespaces[i].GVK.String() < gvkNamespaces[j].GVK.String()
		}
		return gvkNamespaces[i].Namespace < gvkNamespaces[j].Namespace
	})
}

type fakeReader struct {
	syncedGVKNamespaces []gvkNamespace
	err                 error
}

func (f *fakeReader) Get(_ context.Context, _ client.ObjectKey, _ runtime.Object) error {
	return nil
}

//nolint:gocritic
func (f *fakeReader) List(_ context.Context, list runtime.Object, opts ...client.ListOption) error {
	var namespace string
	for _, opt := range opts {
		switch opt := opt.(type) {
		case client.InNamespace:
			namespace = string(opt)
		}
	}

	gvk := list.GetObjectKind().GroupVersionKind()
	f.syncedGVKNamespaces = append(f.syncedGVKNamespaces, gvkNamespace{
		GVK:       gvk,
		Namespace: namespace,
	})

	return f.err
}
