// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"fmt"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestDeepEqual(t *testing.T) {
	testCases := map[string]struct {
		actual   ResourceStatus
		expected ResourceStatus
		equal    bool
	}{
		"same resource should be equal": {
			actual: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Foo",
				},
				Resource: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"generation": int64(1),
						},
					},
				},
				Status:  status.UnknownStatus,
				Message: "Some message",
			},
			expected: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Foo",
				},
				Resource: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"generation": int64(2),
						},
					},
				},
				Status:  status.UnknownStatus,
				Message: "Some message",
			},
			equal: false,
		},
		"different resources with only name different": {
			actual: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Foo",
				},
				Status: status.CurrentStatus,
			},
			expected: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.CurrentStatus,
			},
			equal: false,
		},
		"different GroupKind otherwise same": {
			actual: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.CurrentStatus,
			},
			expected: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "custom.io",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.CurrentStatus,
			},
			equal: false,
		},
		"same resource with same error": {
			actual: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.UnknownStatus,
				Error:  fmt.Errorf("this is a test"),
			},
			expected: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.UnknownStatus,
				Error:  fmt.Errorf("this is a test"),
			},
			equal: true,
		},
		"same resource with different error": {
			actual: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.UnknownStatus,
				Error:  fmt.Errorf("this is a test"),
			},
			expected: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.UnknownStatus,
				Error:  fmt.Errorf("this is a different error"),
			},
			equal: false,
		},
		"same resource different status": {
			actual: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.CurrentStatus,
			},
			expected: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.InProgressStatus,
			},
			equal: false,
		},
		"same resource with different number of generated resources": {
			actual: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.InProgressStatus,
				GeneratedResources: []*ResourceStatus{
					{
						Identifier: object.ObjMetadata{
							GroupKind: schema.GroupKind{
								Group: "apps",
								Kind:  "ReplicaSet",
							},
							Namespace: "default",
							Name:      "Bar-123",
						},
						Status: status.InProgressStatus,
					},
				},
			},
			expected: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.InProgressStatus,
			},
			equal: false,
		},
		"same resource with different status on generated resources": {
			actual: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.InProgressStatus,
				GeneratedResources: []*ResourceStatus{
					{
						Identifier: object.ObjMetadata{
							GroupKind: schema.GroupKind{
								Group: "apps",
								Kind:  "ReplicaSet",
							},
							Namespace: "default",
							Name:      "Bar-123",
						},
						Status: status.InProgressStatus,
					},
				},
			},
			expected: ResourceStatus{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "default",
					Name:      "Bar",
				},
				Status: status.InProgressStatus,
				GeneratedResources: []*ResourceStatus{
					{
						Identifier: object.ObjMetadata{
							GroupKind: schema.GroupKind{
								Group: "apps",
								Kind:  "ReplicaSet",
							},
							Namespace: "default",
							Name:      "Bar-123",
						},
						Status: status.CurrentStatus,
					},
				},
			},
			equal: false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			res := ResourceStatusEqual(&tc.actual, &tc.expected)

			assert.Equal(t, tc.equal, res)
		})
	}
}
