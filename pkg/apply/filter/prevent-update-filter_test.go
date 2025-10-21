// Copyright 2025 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	metadatafake "k8s.io/client-go/metadata/fake"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestPreventUpdateFilter(t *testing.T) {
	tests := map[string]struct {
		objAnnotations map[string]string
		expectedError  error
	}{
		"empty object annotations": {
			objAnnotations: map[string]string{},
		},
		"object contains matching key but mismatched value": {
			objAnnotations: map[string]string{
				common.LifecycleMutationAnnotation: "foo",
			},
		},
		"object contains matching annotation key/value": {
			objAnnotations: map[string]string{
				common.LifecycleMutationAnnotation: common.IgnoreMutation,
			},
			expectedError: &AnnotationPreventedUpdateError{
				Annotation: common.LifecycleMutationAnnotation,
				Value:      common.IgnoreMutation,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			obj := defaultObj.DeepCopy()
			obj.SetAnnotations(tc.objAnnotations)
			metadataObj := defaultMetadataObj.DeepCopy()
			metadataObj.SetAnnotations(tc.objAnnotations)
			filter := PreventUpdateFilter{
				Client: metadatafake.NewSimpleMetadataClient(scheme.Scheme, metadataObj),
				Mapper: testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...),
			}
			err := filter.Filter(t.Context(), obj)
			testutil.AssertEqual(t, tc.expectedError, err)
		})
	}
}
