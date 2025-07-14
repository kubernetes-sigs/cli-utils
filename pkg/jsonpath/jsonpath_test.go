// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ktestutil "sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/yaml"
)

var o1y = `
apiVersion: v1
kind: Pod
metadata:
  name: pod-name
  namespace: pod-namespace
list:
- 1
- b
- false
map:
  a:
  - "1"
  - "2"
  - "3"
  b: null
  c:
  - x
  - ?: null
  - z
entries:
- name: a
  value: x
- name: b
  value: "y"
- name: c
  value: z
`

var o2y = `
apiVersion: v1
kind: Pod
metadata:
  name: pod-name
  namespace: pod-namespace
list:
- null
- null
- null
map:
  a:
  - null
  - null
  - null
  b: null
  c:
  - null
  - ?: null
  - null
entries:
- name: a
  value: x
- name: b
  value: "y"
- name: c
  value: z
`

func TestGet(t *testing.T) {
	o1 := ktestutil.YamlToUnstructured(t, o1y)

	testCases := map[string]struct {
		obj    *unstructured.Unstructured
		path   string
		values []any
		errMsg string
	}{
		"missing": {
			obj:    o1,
			path:   "$.nope",
			values: []any{},
		},
		"invalid jsonpath": {
			obj:    o1,
			path:   "$.nope[",
			values: nil,
			errMsg: "failed to evaluate jsonpath expression ($.nope[): unexpected end of file",
		},
		"string": {
			obj:    o1,
			path:   "$.kind",
			values: []any{"Pod"},
		},
		"string in map": {
			obj:    o1,
			path:   "$.metadata.name",
			values: []any{"pod-name"},
		},
		"int in array": {
			obj:    o1,
			path:   "$.list[0]",
			values: []any{1},
		},
		"string in array": {
			obj:    o1,
			path:   "$.list[1]",
			values: []any{"b"},
		},
		"bool in array": {
			obj:    o1,
			path:   "$.list[2]",
			values: []any{false},
		},
		"string in array in map": {
			obj:    o1,
			path:   "$.map.c[2]",
			values: []any{"z"},
		},
		"nil in map in array in map": {
			obj:    o1,
			path:   "$.map.c[1][\"?\"]",
			values: []any{nil},
		},
		"array in map": {
			obj:    o1,
			path:   "$.map.a",
			values: []any{[]any{"1", "2", "3"}},
		},
		"array values": {
			obj:    o1,
			path:   "$.map.a.*",
			values: []any{"1", "2", "3"},
		},
		"field selector": {
			obj:    o1,
			path:   `$.entries[?(@.name=="b")].value`,
			values: []any{"y"},
		},
		"multi-field selector": {
			obj:    o1,
			path:   `$.entries[?(@.name=="a" || @.name=="c")].value`,
			values: []any{"x", "z"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			testCtx := []any{"path: %s\nobject:\n%s", tc.path, toYaml(t, tc.obj.Object)}
			values, err := Get(tc.obj.Object, tc.path)
			if tc.errMsg != "" {
				require.EqualError(t, err, tc.errMsg, testCtx...)
			} else {
				require.NoError(t, err, testCtx...)
			}
			require.Equal(t, tc.values, values, testCtx...)
		})
	}
}

func TestSet(t *testing.T) {
	testCases := map[string]struct {
		obj   *unstructured.Unstructured
		path  string
		value any
		found int
		err   error
	}{
		"string": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  "$.kind",
			value: "Pod",
			found: 1,
		},
		"string in map": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  "$.metadata.name",
			value: "pod-name",
			found: 1,
		},
		"int in array": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  "$.list[0]",
			value: 1,
			found: 1,
		},
		"string in array": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  "$.list[1]",
			value: "b",
			found: 1,
		},
		"bool in array": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  "$.list[2]",
			value: false,
			found: 1,
		},
		"string in array in map": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  "$.map.c[2]",
			value: "z",
			found: 1,
		},
		"nil in map in array in map": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  "$.map.c[1][\"?\"]",
			value: nil,
			found: 1,
		},
		"array in map": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  "$.map.a",
			value: []any{"1", "2", "3"},
			found: 1,
		},
		"field selector": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  `$.entries[?(@.name=="b")].value`,
			value: "k",
			found: 1,
		},
		"multi-field selector": {
			obj:   ktestutil.YamlToUnstructured(t, o2y),
			path:  `$.entries[?(@.name=="a" || @.name=="c")].value`,
			value: "k",
			found: 2,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			found, err := Set(tc.obj.Object, tc.path, tc.value)
			testCtx := []any{"path: %s\nobject (mutated):\n%s", tc.path, toYaml(t, tc.obj.Object)}
			require.Equal(t, tc.err, err, testCtx...)
			require.Equal(t, tc.found, found, testCtx...)

			values, err := Get(tc.obj.Object, tc.path)
			require.NoError(t, err, testCtx...)
			for i, value := range values {
				testCtx := []any{"path: %s\nindex: %d\nobject (mutated):\n%s", tc.path, i, toYaml(t, tc.obj.Object)}
				require.IsType(t, tc.value, value, testCtx...)
				require.Equal(t, tc.value, value, testCtx...)
			}
		})
	}
}

func toYaml(t *testing.T, in any) string {
	yamlBytes, err := yaml.Marshal(in)
	require.NoError(t, err)
	return string(yamlBytes)
}
