// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package clusterreader

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	deploymentGVK = appsv1.SchemeGroupVersion.WithKind("Deployment")
	rsGVK         = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	podGVK        = v1.SchemeGroupVersion.WithKind("Pod")
	crdGVK        = schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}
)

func TestSync(t *testing.T) {
	testCases := map[string]struct {
		identifiers    object.ObjMetadataSet
		expectedSynced []gkNamespace
	}{
		"no identifiers": {
			identifiers: object.ObjMetadataSet{},
		},
		"same GVK in multiple namespaces": {
			identifiers: object.ObjMetadataSet{
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
			expectedSynced: []gkNamespace{
				{
					GroupKind: deploymentGVK.GroupKind(),
					Namespace: "Foo",
				},
				{
					GroupKind: rsGVK.GroupKind(),
					Namespace: "Foo",
				},
				{
					GroupKind: podGVK.GroupKind(),
					Namespace: "Foo",
				},
				{
					GroupKind: deploymentGVK.GroupKind(),
					Namespace: "Bar",
				},
				{
					GroupKind: rsGVK.GroupKind(),
					Namespace: "Bar",
				},
				{
					GroupKind: podGVK.GroupKind(),
					Namespace: "Bar",
				},
			},
		},
	}

	fakeMapper := testutil.NewFakeRESTMapper(
		deploymentGVK,
		rsGVK,
		v1.SchemeGroupVersion.WithKind("Pod"),
	)

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			fakeReader := &fakeReader{}

			clusterReader, err := newCachingClusterReader(fakeReader, fakeMapper, tc.identifiers)
			require.NoError(t, err)

			err = clusterReader.Sync(context.Background())
			require.NoError(t, err)

			synced := fakeReader.syncedGVKNamespaces
			sortGVKNamespaces(synced)
			expectedSynced := tc.expectedSynced
			sortGVKNamespaces(expectedSynced)
			assert.Equal(t, expectedSynced, synced)

			assert.Equal(t, len(tc.expectedSynced), len(clusterReader.cache))
		})
	}
}

func TestSync_Errors(t *testing.T) {
	testCases := map[string]struct {
		mapper          meta.RESTMapper
		readerError     error
		expectSyncError bool
		cacheError      bool
		cacheErrorText  string
	}{
		"mapping and reader are successful": {
			mapper: testutil.NewFakeRESTMapper(
				crdGVK,
			),
			readerError:     nil,
			expectSyncError: false,
			cacheError:      false,
		},
		"reader returns NotFound error": {
			mapper: testutil.NewFakeRESTMapper(
				crdGVK,
			),
			readerError: errors.NewNotFound(schema.GroupResource{
				Group:    "apiextensions.k8s.io",
				Resource: "customresourcedefinitions",
			}, "my-crd"),
			expectSyncError: false,
			cacheError:      true,
			cacheErrorText:  `customresourcedefinitions.apiextensions.k8s.io "my-crd" not found`,
		},
		"reader returns other error": {
			mapper: testutil.NewFakeRESTMapper(
				crdGVK,
			),
			readerError:     errors.NewInternalError(fmt.Errorf("testing")),
			expectSyncError: false,
			cacheError:      true,
			cacheErrorText:  "Internal error occurred: testing",
		},
		"mapping not found": {
			mapper:          testutil.NewFakeRESTMapper(),
			expectSyncError: false,
			cacheError:      true,
			cacheErrorText:  `no matches for kind "CustomResourceDefinition" in group "apiextensions.k8s.io"`,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			identifiers := object.ObjMetadataSet{
				{
					Name: "my-crd",
					GroupKind: schema.GroupKind{
						Group: "apiextensions.k8s.io",
						Kind:  "CustomResourceDefinition",
					},
				},
			}

			fakeReader := &fakeReader{
				err: tc.readerError,
			}

			clusterReader, err := newCachingClusterReader(fakeReader, tc.mapper, identifiers)
			require.NoError(t, err)

			err = clusterReader.Sync(context.Background())

			if tc.expectSyncError {
				assert.Equal(t, tc.readerError, err)
				return
			}
			require.NoError(t, err)

			cacheEntry, found := clusterReader.cache[gkNamespace{
				GroupKind: crdGVK.GroupKind(),
			}]
			require.True(t, found)
			if tc.cacheError {
				assert.EqualError(t, cacheEntry.err, tc.cacheErrorText)
			}
		})
	}
}

// newCachingClusterReader creates a new CachingClusterReader and returns it as the concrete
// type instead of engine.ClusterReader.
func newCachingClusterReader(reader client.Reader, mapper meta.RESTMapper, identifiers object.ObjMetadataSet) (*CachingClusterReader, error) {
	r, err := NewCachingClusterReader(reader, mapper, identifiers)
	if err != nil {
		return nil, err
	}
	return r.(*CachingClusterReader), nil
}

func sortGVKNamespaces(gvkNamespaces []gkNamespace) {
	sort.Slice(gvkNamespaces, func(i, j int) bool {
		if gvkNamespaces[i].GroupKind.String() != gvkNamespaces[j].GroupKind.String() {
			return gvkNamespaces[i].GroupKind.String() < gvkNamespaces[j].GroupKind.String()
		}
		return gvkNamespaces[i].Namespace < gvkNamespaces[j].Namespace
	})
}

type fakeReader struct {
	syncedGVKNamespaces []gkNamespace
	err                 error
}

func (f *fakeReader) Get(_ context.Context, _ client.ObjectKey, _ client.Object) error {
	return nil
}

//nolint:gocritic
func (f *fakeReader) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	var namespace string
	for _, opt := range opts {
		switch opt := opt.(type) {
		case client.InNamespace:
			namespace = string(opt)
		}
	}

	gvk := list.GetObjectKind().GroupVersionKind()
	f.syncedGVKNamespaces = append(f.syncedGVKNamespaces, gkNamespace{
		GroupKind: gvk.GroupKind(),
		Namespace: namespace,
	})

	return f.err
}
