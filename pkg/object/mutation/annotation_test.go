// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package mutation

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ktestutil "sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
)

var configmap1y = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map1-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourcePath: $.status.number
        sourceRef:
          group: resourcemanager.cnrm.cloud.google.com
          kind: Project
          name: example-name
          namespace: example-namespace
        targetPath: $.spec.member
        token: ${project-number}
data: {}
`

var m1 = ApplyTimeMutation{
	{
		SourceRef: ResourceReference{
			Group:     "resourcemanager.cnrm.cloud.google.com",
			Kind:      "Project",
			Name:      "example-name",
			Namespace: "example-namespace",
		},
		SourcePath: "$.status.number",
		TargetPath: "$.spec.member",
		Token:      "${project-number}",
	},
}

var configmap2y = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map1-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourcePath: .status.field
        sourceRef:
          kind: ConfigMap
          name: example-name
        targetPath: .spec.field
data: {}
`

var m2 = ApplyTimeMutation{
	{
		SourceRef: ResourceReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "example-name",
		},
		SourcePath: ".status.field",
		TargetPath: ".spec.field",
	},
}

// inline json, no spaces or linebreaks
var u1j = &unstructured.Unstructured{
	Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "unused",
			"namespace": "unused",
			"annotations": map[string]any{
				Annotation: `[` +
					`{` +
					`"sourceRef":{` +
					`"group":"resourcemanager.cnrm.cloud.google.com",` +
					`"kind":"Project",` +
					`"name":"example-name",` +
					`"namespace":"example-namespace"` +
					`},` +
					`"sourcePath": "$.status.number",` +
					`"targetPath": "$.spec.member",` +
					`"token": "${project-number}"` +
					`},` +
					`]`,
			},
		},
	},
}

// yaml w/ multiple subs
var configmap3y = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map1-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourcePath: $.status.number
        sourceRef:
          group: resourcemanager.cnrm.cloud.google.com
          kind: Project
          name: example-name
          namespace: example-namespace
        targetPath: $.spec.member
        token: ${project-number}
      - sourcePath: .status.field
        sourceRef:
          kind: ConfigMap
          name: example-name
        targetPath: .spec.field
data: {}
`

// json w/ multiple subs
var configmap4y = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map1-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      [
        {
          "sourceRef": {
            "group": "resourcemanager.cnrm.cloud.google.com",
            "kind": "Project",
            "name": "example-name",
            "namespace": "example-namespace"
          },
          "sourcePath": "$.status.number",
          "targetPath": "$.spec.member",
          "token": "${project-number}"
        },
        {
          "sourceRef": {
            "kind": "ConfigMap",
            "name": "example-name"
          },
          "sourcePath": ".status.field",
          "targetPath": ".spec.field"
        }
      ]
data: {}
`

var m3 = ApplyTimeMutation{
	{
		SourceRef: ResourceReference{
			Group:     "resourcemanager.cnrm.cloud.google.com",
			Kind:      "Project",
			Name:      "example-name",
			Namespace: "example-namespace",
		},
		SourcePath: "$.status.number",
		TargetPath: "$.spec.member",
		Token:      "${project-number}",
	},
	{
		SourceRef: ResourceReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "example-name",
		},
		SourcePath: ".status.field",
		TargetPath: ".spec.field",
	},
}

var noAnnotationsYAML = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map1-name
  namespace: map-namespace
data: {}
`

var invalidAnnotationsYAML = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map1-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: this string is not a substitution
data: {}
`

func TestReadAnnotation(t *testing.T) {
	configmap1 := ktestutil.YamlToUnstructured(t, configmap1y)
	configmap2 := ktestutil.YamlToUnstructured(t, configmap2y)
	configmap3 := ktestutil.YamlToUnstructured(t, configmap3y)
	configmap4 := ktestutil.YamlToUnstructured(t, configmap4y)
	noAnnotations := ktestutil.YamlToUnstructured(t, noAnnotationsYAML)
	invalidAnnotations := ktestutil.YamlToUnstructured(t, invalidAnnotationsYAML)

	testCases := map[string]struct {
		obj      *unstructured.Unstructured
		expected ApplyTimeMutation
		isError  bool
	}{
		"nil object is not found": {
			obj:      nil,
			expected: ApplyTimeMutation{},
		},
		"Object with no annotations returns not found": {
			obj:      noAnnotations,
			expected: ApplyTimeMutation{},
		},
		"Unparseable depends on annotation returns not found": {
			obj:      invalidAnnotations,
			expected: ApplyTimeMutation{},
			isError:  true,
		},
		"Namespace-scoped object apply-time-mutation annotation yaml": {
			obj:      configmap1,
			expected: m1,
		},
		"Namespace-scoped object apply-time-mutation annotation json": {
			obj:      u1j,
			expected: m1,
		},
		"Cluster-scoped object apply-time-mutation annotation yaml": {
			obj:      configmap2,
			expected: m2,
		},
		"Multiple objects specified in annotation yaml": {
			obj:      configmap3,
			expected: m3,
		},
		"Multiple objects specified in annotation json": {
			obj:      configmap4,
			expected: m3,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := ReadAnnotation(tc.obj)
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error not received")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
				if !actual.Equal(tc.expected) {
					t.Errorf("\nexpected:\t%#v\nreceived:\t%#v", tc.expected, actual)
				}
			}
		})
	}
}

func TestWriteAnnotation(t *testing.T) {
	configmap1 := ktestutil.YamlToUnstructured(t, configmap1y)
	configmap2 := ktestutil.YamlToUnstructured(t, configmap2y)
	configmap3 := ktestutil.YamlToUnstructured(t, configmap3y)

	testCases := map[string]struct {
		obj      *unstructured.Unstructured
		mutation ApplyTimeMutation
		expected *string
		isError  bool
	}{
		"nil object": {
			obj:      nil,
			mutation: ApplyTimeMutation{},
			expected: nil,
			isError:  true,
		},
		"empty mutation": {
			obj:      &unstructured.Unstructured{},
			mutation: ApplyTimeMutation{},
			expected: nil,
			isError:  true,
		},
		"Namespace-scoped object": {
			obj:      &unstructured.Unstructured{},
			mutation: m1,
			expected: getApplyTimeMutation(configmap1),
		},
		"Cluster-scoped object": {
			obj:      &unstructured.Unstructured{},
			mutation: m2,
			expected: getApplyTimeMutation(configmap2),
		},
		"Multiple objects": {
			obj:      &unstructured.Unstructured{},
			mutation: m3,
			expected: getApplyTimeMutation(configmap3),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			err := WriteAnnotation(tc.obj, tc.mutation)
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error not received")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
				received := getApplyTimeMutation(tc.obj)

				if received != tc.expected && (received == nil || tc.expected == nil) {
					t.Errorf("\nexpected:\t%#v\nreceived:\t%#v", tc.expected, received)
				}

				require.Equal(t, *tc.expected, *received, "unexpected mutation string")
			}
		})
	}
}

func getApplyTimeMutation(obj *unstructured.Unstructured) *string {
	value, found := obj.GetAnnotations()[Annotation]
	if !found {
		return nil
	}
	return &value
}
