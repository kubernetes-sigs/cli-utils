// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/multierror"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/dependson"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"
	mutationutil "sigs.k8s.io/cli-utils/pkg/object/mutation/testutil"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var (
	resources = map[string]string{
		"pod": `
kind: Pod
apiVersion: v1
metadata:
  name: test-pod
  namespace: test-namespace
`,
		"default-pod": `
kind: Pod
apiVersion: v1
metadata:
  name: pod-in-default-namespace
  namespace: default
`,
		"deployment": `
kind: Deployment
apiVersion: apps/v1
metadata:
  name: foo
  namespace: test-namespace
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
  namespace: test-namespace
  uid: secret-uid
  generation: 1
type: Opaque
spec:
  foo: bar
`,
		"namespace": `
kind: Namespace
apiVersion: v1
metadata:
  name: test-namespace
`,

		"crd": `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: crontabs.stable.example.com
spec:
  group: stable.example.com
  versions:
    - name: v1
      served: true
      storage: true
  scope: Namespaced
  names:
    plural: crontabs
    singular: crontab
    kind: CronTab
`,
		"crontab1": `
apiVersion: "stable.example.com/v1"
kind: CronTab
metadata:
  name: cron-tab-01
  namespace: test-namespace
`,
		"crontab2": `
apiVersion: "stable.example.com/v1"
kind: CronTab
metadata:
  name: cron-tab-02
  namespace: test-namespace
`,
	}
)

func TestSortObjs(t *testing.T) {
	testCases := map[string]struct {
		objs     []*unstructured.Unstructured
		expected []object.UnstructuredSet
		isError  bool
	}{
		"no objects returns no object sets": {
			objs:     []*unstructured.Unstructured{},
			expected: []object.UnstructuredSet{},
			isError:  false,
		},
		"one object returns single object set": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
			isError: false,
		},
		"two unrelated objects returns single object set with two objs": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["deployment"]),
					testutil.Unstructured(t, resources["secret"]),
				},
			},
			isError: false,
		},
		"one object depends on the other; two single object sets": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["secret"]),
				},
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
			isError: false,
		},
		"three objects depend on another; three single object sets": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["pod"]))),
				testutil.Unstructured(t, resources["pod"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["pod"]),
				},
				{
					testutil.Unstructured(t, resources["secret"]),
				},
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
			isError: false,
		},
		"Two objects depend on secret; two object sets": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["secret"]),
				},
				{
					testutil.Unstructured(t, resources["pod"]),
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
			isError: false,
		},
		"two objects applied with their namespace; two object sets": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["namespace"]),
				},
				{
					testutil.Unstructured(t, resources["secret"]),
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
			isError: false,
		},
		"two custom resources applied with their CRD; two object sets": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crontab2"]),
				testutil.Unstructured(t, resources["crd"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["crd"]),
				},
				{
					testutil.Unstructured(t, resources["crontab1"]),
					testutil.Unstructured(t, resources["crontab2"]),
				},
			},
			isError: false,
		},
		"two custom resources wit CRD and namespace; two object sets": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crontab2"]),
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["crd"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["crd"]),
					testutil.Unstructured(t, resources["namespace"]),
				},
				{
					testutil.Unstructured(t, resources["crontab1"]),
					testutil.Unstructured(t, resources["crontab2"]),
				},
			},
			isError: false,
		},
		"two objects depends on each other is cyclic dependency": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["deployment"]))),
			},
			expected: []object.UnstructuredSet{},
			isError:  true,
		},
		"three objects in cyclic dependency": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["pod"]))),
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["deployment"]))),
			},
			expected: []object.UnstructuredSet{},
			isError:  true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := SortObjs(tc.objs)
			if tc.isError {
				assert.NotNil(t, err, "expected error, but received none")
				return
			}
			assert.Nil(t, err, "unexpected error received")
			verifyObjSets(t, tc.expected, actual)
		})
	}
}

func TestReverseSortObjs(t *testing.T) {
	testCases := map[string]struct {
		objs     []*unstructured.Unstructured
		expected []object.UnstructuredSet
		isError  bool
	}{
		"no objects returns no object sets": {
			objs:     []*unstructured.Unstructured{},
			expected: []object.UnstructuredSet{},
			isError:  false,
		},
		"one object returns single object set": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
			isError: false,
		},
		"three objects depend on another; three single object sets in opposite order": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["pod"]))),
				testutil.Unstructured(t, resources["pod"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
				{
					testutil.Unstructured(t, resources["secret"]),
				},
				{
					testutil.Unstructured(t, resources["pod"]),
				},
			},
			isError: false,
		},
		"two objects applied with their namespace; two sets in opposite order": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["secret"]),
					testutil.Unstructured(t, resources["deployment"]),
				},
				{
					testutil.Unstructured(t, resources["namespace"]),
				},
			},
			isError: false,
		},
		"two custom resources wit CRD and namespace; two object sets in opposite order": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crontab2"]),
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["crd"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["crontab1"]),
					testutil.Unstructured(t, resources["crontab2"]),
				},
				{
					testutil.Unstructured(t, resources["crd"]),
					testutil.Unstructured(t, resources["namespace"]),
				},
			},
			isError: false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := ReverseSortObjs(tc.objs)
			if tc.isError {
				assert.NotNil(t, err, "expected error, but received none")
				return
			}
			assert.Nil(t, err, "unexpected error received")
			verifyObjSets(t, tc.expected, actual)
		})
	}
}

func TestDependencyGraph(t *testing.T) {
	// Use a custom Asserter to customize the graph options
	asserter := testutil.NewAsserter(
		cmpopts.EquateErrors(),
		graphComparer(),
	)

	testCases := map[string]struct {
		objs          object.UnstructuredSet
		graph         *Graph
		expectedError error
	}{
		"no objects": {
			objs:  object.UnstructuredSet{},
			graph: New(),
		},
		"one object no dependencies": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"]),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {},
				},
			},
		},
		"two unrelated objects": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {},
					testutil.ToIdentifier(t, resources["secret"]):     {},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {},
					testutil.ToIdentifier(t, resources["secret"]):     {},
				},
			},
		},
		"two objects one dependency": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"]),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {
						testutil.ToIdentifier(t, resources["secret"]),
					},
					testutil.ToIdentifier(t, resources["secret"]): {},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {},
					testutil.ToIdentifier(t, resources["secret"]): {
						testutil.ToIdentifier(t, resources["deployment"]),
					},
				},
			},
		},
		"three objects two dependencies": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["pod"]))),
				testutil.Unstructured(t, resources["pod"]),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {
						testutil.ToIdentifier(t, resources["secret"]),
					},
					testutil.ToIdentifier(t, resources["secret"]): {
						testutil.ToIdentifier(t, resources["pod"]),
					},
					testutil.ToIdentifier(t, resources["pod"]): {},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["pod"]): {
						testutil.ToIdentifier(t, resources["secret"]),
					},
					testutil.ToIdentifier(t, resources["secret"]): {
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					testutil.ToIdentifier(t, resources["deployment"]): {},
				},
			},
		},
		"three objects two dependencies on the same object": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"]),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {
						testutil.ToIdentifier(t, resources["secret"]),
					},
					testutil.ToIdentifier(t, resources["pod"]): {
						testutil.ToIdentifier(t, resources["secret"]),
					},
					testutil.ToIdentifier(t, resources["secret"]): {},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["secret"]): {
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["pod"]),
					},
					testutil.ToIdentifier(t, resources["pod"]):        {},
					testutil.ToIdentifier(t, resources["deployment"]): {},
				},
			},
		},
		"two objects and their namespace": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					testutil.ToIdentifier(t, resources["secret"]): {
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					testutil.ToIdentifier(t, resources["namespace"]): {},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["namespace"]): {
						testutil.ToIdentifier(t, resources["secret"]),
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					testutil.ToIdentifier(t, resources["secret"]):     {},
					testutil.ToIdentifier(t, resources["deployment"]): {},
				},
			},
		},
		"two custom resources and their CRD": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crontab2"]),
				testutil.Unstructured(t, resources["crd"]),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["crontab1"]): {
						testutil.ToIdentifier(t, resources["crd"]),
					},
					testutil.ToIdentifier(t, resources["crontab2"]): {
						testutil.ToIdentifier(t, resources["crd"]),
					},
					testutil.ToIdentifier(t, resources["crd"]): {},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["crd"]): {
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					testutil.ToIdentifier(t, resources["crontab1"]): {},
					testutil.ToIdentifier(t, resources["crontab2"]): {},
				},
			},
		},
		"two custom resources with their CRD and namespace": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crontab2"]),
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["crd"]),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["crontab1"]): {
						testutil.ToIdentifier(t, resources["crd"]),
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					testutil.ToIdentifier(t, resources["crontab2"]): {
						testutil.ToIdentifier(t, resources["crd"]),
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					testutil.ToIdentifier(t, resources["crd"]):       {},
					testutil.ToIdentifier(t, resources["namespace"]): {},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["crd"]): {
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					testutil.ToIdentifier(t, resources["namespace"]): {
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					testutil.ToIdentifier(t, resources["crontab1"]): {},
					testutil.ToIdentifier(t, resources["crontab2"]): {},
				},
			},
		},
		"two object cyclic dependency": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["deployment"]))),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {
						testutil.ToIdentifier(t, resources["secret"]),
					},
					testutil.ToIdentifier(t, resources["secret"]): {
						testutil.ToIdentifier(t, resources["deployment"]),
					},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["secret"]): {
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					testutil.ToIdentifier(t, resources["deployment"]): {
						testutil.ToIdentifier(t, resources["secret"]),
					},
				},
			},
		},
		"three object cyclic dependency": {
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["pod"]))),
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["deployment"]))),
			},
			graph: &Graph{
				edges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {
						testutil.ToIdentifier(t, resources["secret"]),
					},
					testutil.ToIdentifier(t, resources["secret"]): {
						testutil.ToIdentifier(t, resources["pod"]),
					},
					testutil.ToIdentifier(t, resources["pod"]): {
						testutil.ToIdentifier(t, resources["deployment"]),
					},
				},
				reverseEdges: map[object.ObjMetadata]object.ObjMetadataSet{
					testutil.ToIdentifier(t, resources["deployment"]): {
						testutil.ToIdentifier(t, resources["pod"]),
					},
					testutil.ToIdentifier(t, resources["pod"]): {
						testutil.ToIdentifier(t, resources["secret"]),
					},
					testutil.ToIdentifier(t, resources["secret"]): {
						testutil.ToIdentifier(t, resources["deployment"]),
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			g, err := DependencyGraph(tc.objs)
			if tc.expectedError != nil {
				require.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			asserter.Equal(t, tc.graph, g)
		})
	}
}

func TestHydrateSetList(t *testing.T) {
	testCases := map[string]struct {
		idSetList []object.ObjMetadataSet
		objs      object.UnstructuredSet
		expected  []object.UnstructuredSet
	}{
		"no object sets": {
			idSetList: []object.ObjMetadataSet{},
			expected:  nil,
		},
		"one object set": {
			idSetList: []object.ObjMetadataSet{
				{
					testutil.ToIdentifier(t, resources["deployment"]),
				},
			},
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
		},
		"two out of three": {
			idSetList: []object.ObjMetadataSet{
				{
					testutil.ToIdentifier(t, resources["deployment"]),
				},
				{
					testutil.ToIdentifier(t, resources["secret"]),
				},
				{
					testutil.ToIdentifier(t, resources["pod"]),
				},
			},
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["pod"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
				{
					testutil.Unstructured(t, resources["pod"]),
				},
			},
		},
		"two uneven sets": {
			idSetList: []object.ObjMetadataSet{
				{
					testutil.ToIdentifier(t, resources["secret"]),
					testutil.ToIdentifier(t, resources["deployment"]),
				},
				{
					testutil.ToIdentifier(t, resources["namespace"]),
				},
			},
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
				testutil.Unstructured(t, resources["pod"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["secret"]),
					testutil.Unstructured(t, resources["deployment"]),
				},
				{
					testutil.Unstructured(t, resources["namespace"]),
				},
			},
		},
		"one of two sets": {
			idSetList: []object.ObjMetadataSet{
				{
					testutil.ToIdentifier(t, resources["namespace"]),
					testutil.ToIdentifier(t, resources["crd"]),
				},
				{
					testutil.ToIdentifier(t, resources["crontab1"]),
					testutil.ToIdentifier(t, resources["crontab2"]),
				},
			},
			objs: object.UnstructuredSet{
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["crd"]),
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["namespace"]),
					testutil.Unstructured(t, resources["crd"]),
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			objSetList := HydrateSetList(tc.idSetList, tc.objs)
			assert.Equal(t, tc.expected, objSetList)
		})
	}
}

func TestReverseSetList(t *testing.T) {
	testCases := map[string]struct {
		setList  []object.UnstructuredSet
		expected []object.UnstructuredSet
	}{
		"no object sets": {
			setList:  []object.UnstructuredSet{},
			expected: []object.UnstructuredSet{},
		},
		"one object set": {
			setList: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
		},
		"three object sets": {
			setList: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
				{
					testutil.Unstructured(t, resources["secret"]),
				},
				{
					testutil.Unstructured(t, resources["pod"]),
				},
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["pod"]),
				},
				{
					testutil.Unstructured(t, resources["secret"]),
				},
				{
					testutil.Unstructured(t, resources["deployment"]),
				},
			},
		},
		"two uneven sets": {
			setList: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["secret"]),
					testutil.Unstructured(t, resources["deployment"]),
				},
				{
					testutil.Unstructured(t, resources["namespace"]),
				},
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["namespace"]),
				},
				{
					testutil.Unstructured(t, resources["deployment"]),
					testutil.Unstructured(t, resources["secret"]),
				},
			},
		},
		"two even sets": {
			setList: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["crontab1"]),
					testutil.Unstructured(t, resources["crontab2"]),
				},
				{
					testutil.Unstructured(t, resources["crd"]),
					testutil.Unstructured(t, resources["namespace"]),
				},
			},
			expected: []object.UnstructuredSet{
				{
					testutil.Unstructured(t, resources["namespace"]),
					testutil.Unstructured(t, resources["crd"]),
				},
				{
					testutil.Unstructured(t, resources["crontab2"]),
					testutil.Unstructured(t, resources["crontab1"]),
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ReverseSetList(tc.setList)
			assert.Equal(t, tc.expected, tc.setList)
		})
	}
}

func TestApplyTimeMutationEdges(t *testing.T) {
	testCases := map[string]struct {
		objs          []*unstructured.Unstructured
		expected      []Edge
		expectedError error
	}{
		"no objects adds no graph edges": {
			objs:     []*unstructured.Unstructured{},
			expected: []Edge{},
		},
		"no depends-on annotations adds no graph edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
			},
			expected: []Edge{},
		},
		"no depends-on annotations, two objects, adds no graph edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []Edge{},
		},
		"two dependent objects, adds one edge": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(
					t,
					resources["deployment"],
					mutationutil.AddApplyTimeMutation(t, &mutation.ApplyTimeMutation{
						{
							SourceRef: mutation.ResourceReferenceFromObjMetadata(
								testutil.ToIdentifier(t, resources["secret"]),
							),
							SourcePath: "unused",
							TargetPath: "unused",
							Token:      "unused",
						},
					}),
				),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["deployment"]),
					To:   testutil.ToIdentifier(t, resources["secret"]),
				},
			},
		},
		"three dependent objects, adds two edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(
					t,
					resources["deployment"],
					mutationutil.AddApplyTimeMutation(t, &mutation.ApplyTimeMutation{
						{
							SourceRef: mutation.ResourceReferenceFromObjMetadata(
								testutil.ToIdentifier(t, resources["secret"]),
							),
							SourcePath: "unused",
							TargetPath: "unused",
							Token:      "unused",
						},
					}),
				),
				testutil.Unstructured(
					t,
					resources["pod"],
					mutationutil.AddApplyTimeMutation(t, &mutation.ApplyTimeMutation{
						{
							SourceRef: mutation.ResourceReferenceFromObjMetadata(
								testutil.ToIdentifier(t, resources["secret"]),
							),
							SourcePath: "unused",
							TargetPath: "unused",
							Token:      "unused",
						},
					}),
				),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["deployment"]),
					To:   testutil.ToIdentifier(t, resources["secret"]),
				},
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["secret"]),
				},
			},
		},
		"pod has two dependencies, adds two edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(
					t,
					resources["pod"],
					mutationutil.AddApplyTimeMutation(t, &mutation.ApplyTimeMutation{
						{
							SourceRef: mutation.ResourceReferenceFromObjMetadata(
								testutil.ToIdentifier(t, resources["secret"]),
							),
							SourcePath: "unused",
							TargetPath: "unused",
							Token:      "unused",
						},
						{
							SourceRef: mutation.ResourceReferenceFromObjMetadata(
								testutil.ToIdentifier(t, resources["deployment"]),
							),
							SourcePath: "unused",
							TargetPath: "unused",
							Token:      "unused",
						},
					}),
				),
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["secret"]),
				},
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["deployment"]),
				},
			},
		},
		"error: invalid annotation": {
			objs: []*unstructured.Unstructured{
				{
					Object: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]any{
							"name":      "foo",
							"namespace": "default",
							"annotations": map[string]any{
								mutation.Annotation: "invalid-mutation",
							},
						},
					},
				},
			},
			expected: []Edge{},
			expectedError: validation.NewError(
				object.InvalidAnnotationError{
					Annotation: mutation.Annotation,
					Cause: errors.New("error unmarshaling JSON: " +
						"while decoding JSON: json: " +
						"cannot unmarshal string into Go value of type mutation.ApplyTimeMutation"),
				},
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Name:      "foo",
					Namespace: "default",
				},
			),
		},
		"error: dependency not in object set": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"],
					mutationutil.AddApplyTimeMutation(t, &mutation.ApplyTimeMutation{
						{
							SourceRef: mutation.ResourceReferenceFromObjMetadata(
								testutil.ToIdentifier(t, resources["deployment"]),
							),
						},
					}),
				),
			},
			expected: []Edge{},
			expectedError: validation.NewError(
				object.InvalidAnnotationError{
					Annotation: mutation.Annotation,
					Cause: ExternalDependencyError{
						Edge: Edge{
							From: testutil.ToIdentifier(t, resources["pod"]),
							To:   testutil.ToIdentifier(t, resources["deployment"]),
						},
					},
				},
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			),
		},
		"error: two invalid objects": {
			objs: []*unstructured.Unstructured{
				{
					Object: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]any{
							"name":      "foo",
							"namespace": "default",
							"annotations": map[string]any{
								mutation.Annotation: "invalid-mutation",
							},
						},
					},
				},
				testutil.Unstructured(t, resources["pod"],
					mutationutil.AddApplyTimeMutation(t, &mutation.ApplyTimeMutation{
						{
							SourceRef: mutation.ResourceReferenceFromObjMetadata(
								testutil.ToIdentifier(t, resources["secret"]),
							),
						},
					}),
				),
			},
			expected: []Edge{},
			expectedError: multierror.New(
				validation.NewError(
					object.InvalidAnnotationError{
						Annotation: mutation.Annotation,
						Cause: errors.New("error unmarshaling JSON: " +
							"while decoding JSON: json: " +
							"cannot unmarshal string into Go value of type mutation.ApplyTimeMutation"),
					},
					object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Name:      "foo",
						Namespace: "default",
					},
				),
				validation.NewError(
					object.InvalidAnnotationError{
						Annotation: mutation.Annotation,
						Cause: ExternalDependencyError{
							Edge: Edge{
								From: testutil.ToIdentifier(t, resources["pod"]),
								To:   testutil.ToIdentifier(t, resources["secret"]),
							},
						},
					},
					object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "",
							Kind:  "Pod",
						},
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
				),
			),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			g := New()
			ids := object.UnstructuredSetToObjMetadataSet(tc.objs)
			err := addApplyTimeMutationEdges(g, tc.objs, ids)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			actual := edgeMapToList(g.edges)
			verifyEdges(t, tc.expected, actual)
		})
	}
}

func TestAddDependsOnEdges(t *testing.T) {
	testCases := map[string]struct {
		objs          []*unstructured.Unstructured
		expected      []Edge
		expectedError error
	}{
		"no objects adds no graph edges": {
			objs:     []*unstructured.Unstructured{},
			expected: []Edge{},
		},
		"no depends-on annotations adds no graph edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
			},
			expected: []Edge{},
		},
		"no depends-on annotations, two objects, adds no graph edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []Edge{},
		},
		"two dependent objects, adds one edge": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["deployment"]),
					To:   testutil.ToIdentifier(t, resources["secret"]),
				},
			},
		},
		"three dependent objects, adds two edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["deployment"]),
					To:   testutil.ToIdentifier(t, resources["secret"]),
				},
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["secret"]),
				},
			},
		},
		"pod has two dependencies, adds two edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t,
						testutil.ToIdentifier(t, resources["secret"]),
						testutil.ToIdentifier(t, resources["deployment"]),
					),
				),
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["secret"]),
				},
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["deployment"]),
				},
			},
		},
		"error: invalid annotation": {
			objs: []*unstructured.Unstructured{
				{
					Object: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]any{
							"name":      "foo",
							"namespace": "default",
							"annotations": map[string]any{
								dependson.Annotation: "invalid-obj-ref",
							},
						},
					},
				},
			},
			expected: []Edge{},
			expectedError: validation.NewError(
				object.InvalidAnnotationError{
					Annotation: dependson.Annotation,
					Cause: errors.New("failed to parse object reference (index: 0): " +
						`expected 3 or 5 fields, found 1: "invalid-obj-ref"`),
				},
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Name:      "foo",
					Namespace: "default",
				},
			),
		},
		"error: duplicate reference": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t,
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["deployment"]),
					),
				),
				testutil.Unstructured(t, resources["deployment"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["deployment"]),
				},
			},
			expectedError: validation.NewError(
				object.InvalidAnnotationError{
					Annotation: dependson.Annotation,
					Cause: DuplicateDependencyError{
						Edge: Edge{
							From: testutil.ToIdentifier(t, resources["pod"]),
							To:   testutil.ToIdentifier(t, resources["deployment"]),
						},
					},
				},
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			),
		},
		"error: external dependency": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t,
						testutil.ToIdentifier(t, resources["deployment"]),
					),
				),
			},
			expected: []Edge{},
			expectedError: validation.NewError(
				object.InvalidAnnotationError{
					Annotation: dependson.Annotation,
					Cause: ExternalDependencyError{
						Edge: Edge{
							From: testutil.ToIdentifier(t, resources["pod"]),
							To:   testutil.ToIdentifier(t, resources["deployment"]),
						},
					},
				},
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			),
		},
		"error: two invalid objects": {
			objs: []*unstructured.Unstructured{
				{
					Object: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]any{
							"name":      "foo",
							"namespace": "default",
							"annotations": map[string]any{
								dependson.Annotation: "invalid-obj-ref",
							},
						},
					},
				},
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t,
						testutil.ToIdentifier(t, resources["secret"]),
					),
				),
			},
			expected: []Edge{},
			expectedError: multierror.New(
				validation.NewError(
					object.InvalidAnnotationError{
						Annotation: dependson.Annotation,
						Cause: errors.New("failed to parse object reference (index: 0): " +
							`expected 3 or 5 fields, found 1: "invalid-obj-ref"`),
					},
					object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Name:      "foo",
						Namespace: "default",
					},
				),
				validation.NewError(
					object.InvalidAnnotationError{
						Annotation: dependson.Annotation,
						Cause: ExternalDependencyError{
							Edge: Edge{
								From: testutil.ToIdentifier(t, resources["pod"]),
								To:   testutil.ToIdentifier(t, resources["secret"]),
							},
						},
					},
					object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "",
							Kind:  "Pod",
						},
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
				),
			),
		},
		"error: one object with two errors": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t,
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["deployment"]),
					),
				),
			},
			expected: []Edge{},
			expectedError: validation.NewError(
				multierror.New(
					object.InvalidAnnotationError{
						Annotation: dependson.Annotation,
						Cause: ExternalDependencyError{
							Edge: Edge{
								From: testutil.ToIdentifier(t, resources["pod"]),
								To:   testutil.ToIdentifier(t, resources["deployment"]),
							},
						},
					},
					object.InvalidAnnotationError{
						Annotation: dependson.Annotation,
						Cause: DuplicateDependencyError{
							Edge: Edge{
								From: testutil.ToIdentifier(t, resources["pod"]),
								To:   testutil.ToIdentifier(t, resources["deployment"]),
							},
						},
					},
				),
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			g := New()
			ids := object.UnstructuredSetToObjMetadataSet(tc.objs)
			err := addDependsOnEdges(g, tc.objs, ids)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			actual := edgeMapToList(g.edges)
			verifyEdges(t, tc.expected, actual)
		})
	}
}

func TestAddNamespaceEdges(t *testing.T) {
	testCases := map[string]struct {
		objs     []*unstructured.Unstructured
		expected []Edge
	}{
		"no namespace objects adds no graph edges": {
			objs:     []*unstructured.Unstructured{},
			expected: []Edge{},
		},
		"single namespace adds no graph edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["namespace"]),
			},
			expected: []Edge{},
		},
		"pod within namespace adds one edge": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["pod"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["namespace"]),
				},
			},
		},
		"pod not in namespace does not add edge": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["default-pod"]),
			},
			expected: []Edge{},
		},
		"pod, secret, and namespace adds two edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["secret"]),
				testutil.Unstructured(t, resources["pod"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["namespace"]),
				},
				{
					From: testutil.ToIdentifier(t, resources["secret"]),
					To:   testutil.ToIdentifier(t, resources["namespace"]),
				},
			},
		},
		"one pod in namespace, one not, adds only one edge": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["default-pod"]),
				testutil.Unstructured(t, resources["pod"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["pod"]),
					To:   testutil.ToIdentifier(t, resources["namespace"]),
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			g := New()
			ids := object.UnstructuredSetToObjMetadataSet(tc.objs)
			addNamespaceEdges(g, tc.objs, ids)
			actual := edgeMapToList(g.edges)
			verifyEdges(t, tc.expected, actual)
		})
	}
}

func TestAddCRDEdges(t *testing.T) {
	testCases := map[string]struct {
		objs     []*unstructured.Unstructured
		expected []Edge
	}{
		"no CRD objects adds no graph edges": {
			objs:     []*unstructured.Unstructured{},
			expected: []Edge{},
		},
		"single namespace adds no graph edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crd"]),
			},
			expected: []Edge{},
		},
		"two custom resources adds no graph edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crontab2"]),
			},
			expected: []Edge{},
		},
		"two custom resources with crd adds two edges": {
			objs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crd"]),
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crontab2"]),
			},
			expected: []Edge{
				{
					From: testutil.ToIdentifier(t, resources["crontab1"]),
					To:   testutil.ToIdentifier(t, resources["crd"]),
				},
				{
					From: testutil.ToIdentifier(t, resources["crontab2"]),
					To:   testutil.ToIdentifier(t, resources["crd"]),
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			g := New()
			ids := object.UnstructuredSetToObjMetadataSet(tc.objs)
			addCRDEdges(g, tc.objs, ids)
			actual := edgeMapToList(g.edges)
			verifyEdges(t, tc.expected, actual)
		})
	}
}

// verifyObjSets ensures the expected and actual slice of object sets are the same,
// and the sets are in order.
func verifyObjSets(t *testing.T, expected []object.UnstructuredSet, actual []object.UnstructuredSet) {
	if len(expected) != len(actual) {
		t.Fatalf("expected (%d) object sets, got (%d)", len(expected), len(actual))
		return
	}
	// Order matters
	for i := range expected {
		expectedSet := expected[i]
		actualSet := actual[i]
		if len(expectedSet) != len(actualSet) {
			t.Fatalf("set %d: expected object size (%d), got (%d)", i, len(expectedSet), len(actualSet))
			return
		}
		for _, actualObj := range actualSet {
			if !containsObjs(expectedSet, actualObj) {
				t.Fatalf("set #%d: actual object (%v) not found in set of expected objects", i, actualObj)
				return
			}
		}
	}
}

// containsUnstructured returns true if the passed object is within the passed
// slice of objects; false otherwise. Order is not important.
func containsObjs(objs []*unstructured.Unstructured, obj *unstructured.Unstructured) bool {
	ids := object.UnstructuredSetToObjMetadataSet(objs)
	id := object.UnstructuredToObjMetadata(obj)
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

// verifyEdges ensures the slices of directed Edges contain the same elements.
// Order is not important.
func verifyEdges(t *testing.T, expected []Edge, actual []Edge) {
	if len(expected) != len(actual) {
		t.Fatalf("expected (%d) edges, got (%d)", len(expected), len(actual))
		return
	}
	for _, actualEdge := range actual {
		if !containsEdge(expected, actualEdge) {
			t.Errorf("actual Edge (%v) not found in expected Edges", actualEdge)
			return
		}
	}
}

// containsEdge return true if the passed Edge is in the slice of Edges;
// false otherwise.
func containsEdge(edges []Edge, edge Edge) bool {
	for _, e := range edges {
		if e.To == edge.To && e.From == edge.From {
			return true
		}
	}
	return false
}

// waitTaskComparer allows comparison of WaitTasks, ignoring private fields.
func graphComparer() cmp.Option {
	return cmp.Comparer(func(x, y *Graph) bool {
		if x == nil {
			return y == nil
		}
		if y == nil {
			return false
		}
		return cmp.Equal(x.edges, y.edges) &&
			cmp.Equal(x.reverseEdges, y.reverseEdges)
	})
}
