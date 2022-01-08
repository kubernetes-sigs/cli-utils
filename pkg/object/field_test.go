// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFieldPath(t *testing.T) {
	tests := map[string]struct {
		fieldPath []interface{}
		expected  string
	}{
		"empty path": {
			fieldPath: []interface{}{},
			expected:  "",
		},
		"kind": {
			fieldPath: []interface{}{"kind"},
			expected:  ".kind",
		},
		"metadata.name": {
			fieldPath: []interface{}{"metadata", "name"},
			expected:  ".metadata.name",
		},
		"spec.versions[1].name": {
			fieldPath: []interface{}{"spec", "versions", 1, "name"},
			expected:  ".spec.versions[1].name",
		},
		"numeric": {
			fieldPath: []interface{}{"spec", "123"},
			expected:  `.spec["123"]`,
		},
		"alphanumeric, ends with number": {
			fieldPath: []interface{}{"spec", "abc123"},
			expected:  `.spec.abc123`,
		},
		"alphanumeric, ends with hyphen": {
			fieldPath: []interface{}{"spec", "abc123-"},
			expected:  `.spec["abc123-"]`,
		},
		"alphanumeric, ends with underscore": {
			fieldPath: []interface{}{"spec", "abc123_"},
			expected:  `.spec["abc123_"]`,
		},
		"alphanumeric, starts with hyphen": {
			fieldPath: []interface{}{"spec", "-abc123"},
			expected:  `.spec["-abc123"]`,
		},
		"alphanumeric, starts with underscore": {
			fieldPath: []interface{}{"spec", "_abc123"},
			expected:  `.spec["_abc123"]`,
		},
		"alphanumeric, starts with number": {
			fieldPath: []interface{}{"spec", "_abc123"},
			expected:  `.spec["_abc123"]`,
		},
		"alphanumeric, intrnal hyphen": {
			fieldPath: []interface{}{"spec", "abc-123"},
			expected:  `.spec.abc-123`,
		},
		"alphanumeric, intrnal underscore": {
			fieldPath: []interface{}{"spec", "abc_123"},
			expected:  `.spec.abc_123`,
		},
		"space": {
			fieldPath: []interface{}{"spec", "abc 123"},
			expected:  `.spec["abc 123"]`,
		},
		"tab": {
			fieldPath: []interface{}{"spec", "abc\t123"},
			expected:  `.spec["abc\t123"]`,
		},
		"linebreak": {
			fieldPath: []interface{}{"spec", "abc\n123"},
			expected:  `.spec["abc\n123"]`,
		},
		// result from invalid input doesn't matter, as long as it doesn't panic
		"invalid type: float": {
			fieldPath: []interface{}{"spec", float64(-1.0)},
			expected:  `.spec[-1]`,
		},
		"invalid type: struct": {
			fieldPath: []interface{}{"spec", struct{ Field string }{Field: "value"}},
			expected:  `.spec[struct { Field string }{Field:"value"}]`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := FieldPath(tc.fieldPath)
			assert.Equal(t, tc.expected, result)
		})
	}
}
