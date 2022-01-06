// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var (
	on1 = object.ObjMetadata{Name: "obj1", Namespace: "ns1", GroupKind: schema.GroupKind{Group: "test", Kind: "foo"}}
	on2 = object.ObjMetadata{Name: "obj2", Namespace: "ns1", GroupKind: schema.GroupKind{Group: "test", Kind: "foo"}}
)

func TestExternalDependencyErrorString(t *testing.T) {
	testCases := map[string]struct {
		err            ExternalDependencyError
		expectedString string
	}{
		"cluster-scoped": {
			err: ExternalDependencyError{
				Edge: Edge{
					From: o1,
					To:   o2,
				},
			},
			expectedString: `external dependency: test/foo/obj1 -> test/foo/obj2`,
		},
		"namespace-scoped": {
			err: ExternalDependencyError{
				Edge: Edge{
					From: on1,
					To:   on2,
				},
			},
			expectedString: `external dependency: test/namespaces/ns1/foo/obj1 -> test/namespaces/ns1/foo/obj2`,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			assert.Equal(t, tc.expectedString, tc.err.Error())
		})
	}
}

func TestCyclicDependencyErrorString(t *testing.T) {
	testCases := map[string]struct {
		err            CyclicDependencyError
		expectedString string
	}{
		"two object cycle": {
			err: CyclicDependencyError{
				Edges: []Edge{
					{
						From: o1,
						To:   o2,
					},
					{
						From: o2,
						To:   o1,
					},
				},
			},
			expectedString: `cyclic dependency:
- test/foo/obj1 -> test/foo/obj2
- test/foo/obj2 -> test/foo/obj1`,
		},
		"three object cycle": {
			err: CyclicDependencyError{
				Edges: []Edge{
					{
						From: o1,
						To:   o2,
					},
					{
						From: o2,
						To:   o3,
					},
					{
						From: o3,
						To:   o1,
					},
				},
			},
			expectedString: `cyclic dependency:
- test/foo/obj1 -> test/foo/obj2
- test/foo/obj2 -> test/foo/obj3
- test/foo/obj3 -> test/foo/obj1`,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			assert.Equal(t, tc.expectedString, tc.err.Error())
		})
	}
}

func TestDuplicateDependencyErrorString(t *testing.T) {
	testCases := map[string]struct {
		err            DuplicateDependencyError
		expectedString string
	}{
		"cluster-scoped": {
			err: DuplicateDependencyError{
				Edge: Edge{
					From: o1,
					To:   o2,
				},
			},
			expectedString: `duplicate dependency: test/foo/obj1 -> test/foo/obj2`,
		},
		"namespace-scoped": {
			err: DuplicateDependencyError{
				Edge: Edge{
					From: on1,
					To:   on2,
				},
			},
			expectedString: `duplicate dependency: test/namespaces/ns1/foo/obj1 -> test/namespaces/ns1/foo/obj2`,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			assert.Equal(t, tc.expectedString, tc.err.Error())
		})
	}
}
