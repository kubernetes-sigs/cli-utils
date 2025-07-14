// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFieldPath(t *testing.T) {
	tests := map[string]struct {
		fieldPath []any
		expected  string
	}{
		"empty path": {
			fieldPath: []any{},
			expected:  "",
		},
		"kind": {
			fieldPath: []any{"kind"},
			expected:  ".kind",
		},
		"metadata.name": {
			fieldPath: []any{"metadata", "name"},
			expected:  ".metadata.name",
		},
		"spec.versions[1].name": {
			fieldPath: []any{"spec", "versions", 1, "name"},
			expected:  ".spec.versions[1].name",
		},
		"numeric": {
			fieldPath: []any{"spec", "123"},
			expected:  `.spec["123"]`,
		},
		"alphanumeric, ends with number": {
			fieldPath: []any{"spec", "abc123"},
			expected:  `.spec.abc123`,
		},
		"alphanumeric, ends with hyphen": {
			fieldPath: []any{"spec", "abc123-"},
			expected:  `.spec["abc123-"]`,
		},
		"alphanumeric, ends with underscore": {
			fieldPath: []any{"spec", "abc123_"},
			expected:  `.spec["abc123_"]`,
		},
		"alphanumeric, starts with hyphen": {
			fieldPath: []any{"spec", "-abc123"},
			expected:  `.spec["-abc123"]`,
		},
		"alphanumeric, starts with underscore": {
			fieldPath: []any{"spec", "_abc123"},
			expected:  `.spec["_abc123"]`,
		},
		"alphanumeric, starts with number": {
			fieldPath: []any{"spec", "_abc123"},
			expected:  `.spec["_abc123"]`,
		},
		"alphanumeric, intrnal hyphen": {
			fieldPath: []any{"spec", "abc-123"},
			expected:  `.spec.abc-123`,
		},
		"alphanumeric, intrnal underscore": {
			fieldPath: []any{"spec", "abc_123"},
			expected:  `.spec.abc_123`,
		},
		"space": {
			fieldPath: []any{"spec", "abc 123"},
			expected:  `.spec["abc 123"]`,
		},
		"tab": {
			fieldPath: []any{"spec", "abc\t123"},
			expected:  `.spec["abc\t123"]`,
		},
		"linebreak": {
			fieldPath: []any{"spec", "abc\n123"},
			expected:  `.spec["abc\n123"]`,
		},
		// result from invalid input doesn't matter, as long as it doesn't panic
		"invalid type: float": {
			fieldPath: []any{"spec", float64(-1.0)},
			expected:  `.spec[-1]`,
		},
		"invalid type: struct": {
			fieldPath: []any{"spec", struct{ Field string }{Field: "value"}},
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
