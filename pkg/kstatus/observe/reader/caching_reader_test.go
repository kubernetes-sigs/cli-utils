// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package reader

import (
	"context"
	"sort"
	"testing"

	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	deploymentGVK = appsv1.SchemeGroupVersion.WithKind("Deployment")
	rsGVK         = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	podGVK        = v1.SchemeGroupVersion.WithKind("Pod")
)

func TestSync(t *testing.T) {
	testCases := map[string]struct {
		identifiers    []wait.ResourceIdentifier
		expectedSynced []gvkNamespace
	}{
		"no identifiers": {
			identifiers: []wait.ResourceIdentifier{},
		},
		"same GVK in multiple namespaces": {
			identifiers: []wait.ResourceIdentifier{
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

			observerReader, err := NewCachingClusterReader(fakeReader, fakeMapper, tc.identifiers)
			assert.NilError(t, err)

			err = observerReader.Sync(context.Background())
			assert.NilError(t, err)

			synced := fakeReader.syncedGVKNamespaces
			sortGVKNamespaces(synced)
			expectedSynced := tc.expectedSynced
			sortGVKNamespaces(expectedSynced)
			assert.DeepEqual(t, expectedSynced, synced)

			assert.Equal(t, len(tc.expectedSynced), len(observerReader.cache))
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

	return nil
}
