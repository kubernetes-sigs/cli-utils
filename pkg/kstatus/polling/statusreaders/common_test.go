// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakecr "sigs.k8s.io/cli-utils/pkg/kstatus/polling/clusterreader/fake"
	fakesr "sigs.k8s.io/cli-utils/pkg/kstatus/polling/statusreaders/fake"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/object"
	fakemapper "sigs.k8s.io/cli-utils/pkg/testutil"
)

var (
	deploymentGVK = appsv1.SchemeGroupVersion.WithKind("Deployment")
	deploymentGVR = appsv1.SchemeGroupVersion.WithResource("deployments")
	replicaSetGVK = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")

	rsGVK = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
)

func TestLookupResource(t *testing.T) {
	deploymentIdentifier := object.ObjMetadata{
		GroupKind: deploymentGVK.GroupKind(),
		Name:      "Foo",
		Namespace: "Bar",
	}

	testCases := map[string]struct {
		identifier         object.ObjMetadata
		readerErr          error
		expectErr          bool
		expectedErrMessage string
	}{
		"unknown GVK": {
			identifier: object.ObjMetadata{
				GroupKind: schema.GroupKind{
					Group: "custom.io",
					Kind:  "Custom",
				},
				Name:      "Bar",
				Namespace: "default",
			},
			expectErr:          true,
			expectedErrMessage: `no matches for kind "Custom" in group "custom.io"`,
		},
		"resource does not exist": {
			identifier:         deploymentIdentifier,
			readerErr:          errors.NewNotFound(deploymentGVR.GroupResource(), "Foo"),
			expectErr:          true,
			expectedErrMessage: `deployments.apps "Foo" not found`,
		},
		"getting resource fails": {
			identifier:         deploymentIdentifier,
			readerErr:          errors.NewInternalError(fmt.Errorf("this is a test")),
			expectErr:          true,
			expectedErrMessage: "Internal error occurred: this is a test",
		},
		"getting resource succeeds": {
			identifier: deploymentIdentifier,
		},
		"context cancelled": {
			identifier:         deploymentIdentifier,
			readerErr:          context.Canceled,
			expectErr:          true,
			expectedErrMessage: context.Canceled.Error(),
		},
		"rate would exceed context deadline": {
			identifier:         deploymentIdentifier,
			readerErr:          fmt.Errorf("client rate limiter Wait returned an error: %w", fmt.Errorf("rate: Wait(n=1) would exceed context deadline")),
			expectErr:          true,
			expectedErrMessage: "client rate limiter Wait returned an error: rate: Wait(n=1) would exceed context deadline",
		},
		"rate would exceed context deadline wrapped error": {
			identifier: deploymentIdentifier,
			readerErr: fmt.Errorf("wrapped deeper: %w",
				fmt.Errorf("client rate limiter Wait returned an error: %w", fmt.Errorf("rate: Wait(n=1) would exceed context deadline"))),
			expectErr:          true,
			expectedErrMessage: "wrapped deeper: client rate limiter Wait returned an error: rate: Wait(n=1) would exceed context deadline",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			fakeReader := &fakecr.ClusterReader{
				GetErr: tc.readerErr,
			}
			fakeMapper := fakemapper.NewFakeRESTMapper(deploymentGVK)

			statusReader := &baseStatusReader{
				mapper: fakeMapper,
			}

			u, err := statusReader.lookupResource(context.Background(), fakeReader, tc.identifier)

			if tc.expectErr {
				if err == nil {
					t.Errorf("expected error, but didn't get one")
				} else {
					assert.EqualError(t, err, tc.expectedErrMessage)
				}
				return
			}

			require.NoError(t, err)

			assert.Equal(t, deploymentGVK, u.GroupVersionKind())
		})
	}
}

func TestStatusForGeneratedResources(t *testing.T) {
	testCases := map[string]struct {
		manifest    string
		listObjects []unstructured.Unstructured
		listErr     error
		gk          schema.GroupKind
		path        []string
		expectError bool
		errMessage  string
	}{
		"invalid selector": {
			manifest: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: Foo
spec:
  replicas: 1
`,
			gk:          appsv1.SchemeGroupVersion.WithKind("ReplicaSet").GroupKind(),
			path:        []string{"spec", "selector"},
			expectError: true,
			errMessage:  "no selector found",
		},
		"Invalid GVK": {
			manifest: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: Foo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
`,
			gk: schema.GroupKind{
				Group: "custom.io",
				Kind:  "Custom",
			},
			path:        []string{"spec", "selector"},
			expectError: true,
			errMessage:  `no matches for kind "Custom" in group "custom.io"`,
		},
		"error listing replicasets": {
			manifest: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: Foo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
`,
			listErr:     fmt.Errorf("this is a test"),
			gk:          appsv1.SchemeGroupVersion.WithKind("ReplicaSet").GroupKind(),
			path:        []string{"spec", "selector"},
			expectError: true,
			errMessage:  "this is a test",
		},
		"successfully lists and polling the generated resources": {
			manifest: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: Foo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
`,
			listObjects: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "ReplicaSet",
						"metadata": map[string]interface{}{
							"name":      "Foo-12345",
							"namespace": "default",
						},
					},
				},
			},
			gk:          appsv1.SchemeGroupVersion.WithKind("ReplicaSet").GroupKind(),
			path:        []string{"spec", "selector"},
			expectError: false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			fakeClusterReader := &fakecr.ClusterReader{
				ListResources: &unstructured.UnstructuredList{
					Items: tc.listObjects,
				},
				ListErr: tc.listErr,
			}
			fakeMapper := fakemapper.NewFakeRESTMapper(rsGVK)
			fakeStatusReader := &fakesr.StatusReader{}

			object := testutil.YamlToUnstructured(t, tc.manifest)

			resourceStatuses, err := statusForGeneratedResources(context.Background(), fakeMapper, fakeClusterReader,
				fakeStatusReader, object, tc.gk, tc.path...)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected an error, but didn't get one")
					return
				}
				assert.EqualError(t, err, tc.errMessage)
				return
			}
			if !tc.expectError && err != nil {
				t.Errorf("did not expect an error, but got %v", err)
			}

			assert.Len(t, resourceStatuses, len(tc.listObjects))
			assert.True(t, sort.IsSorted(resourceStatuses))
		})
	}
}
