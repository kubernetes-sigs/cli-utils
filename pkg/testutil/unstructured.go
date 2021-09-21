// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"fmt"
)

// NestedField gets a value from a KRM map, if it exists, otherwise nil.
// Fields can be string (map key) or int (array index).
func NestedField(obj map[string]interface{}, fields ...interface{}) (interface{}, bool, error) {
	var val interface{} = obj

	for i, field := range fields {
		if val == nil {
			return nil, false, nil
		}
		switch typedField := field.(type) {
		case string:
			if m, ok := val.(map[string]interface{}); ok {
				val, ok = m[typedField]
				if !ok {
					// not in map
					return nil, false, nil
				}
			} else {
				return nil, false, fmt.Errorf("%v accessor error: %v is of the type %T, expected map[string]interface{}", jsonPath(fields[:i+1]), val, val)
			}
		case int:
			if s, ok := val.([]interface{}); ok {
				if typedField >= len(s) {
					// index out of range
					return nil, false, nil
				}
				val = s[typedField]
			} else {
				return nil, false, fmt.Errorf("%v accessor error: %v is of the type %T, expected []interface{}", jsonPath(fields[:i+1]), val, val)
			}
		default:
			return nil, false, fmt.Errorf("%v accessor error: field %v is of the type %T, expected string or int", jsonPath(fields[:i+1]), field, typedField)
		}
	}
	return val, true, nil
}

// Simplistic jsonpath formatter, just for NestedField errors.
func jsonPath(fields []interface{}) string {
	path := ""
	for _, field := range fields {
		switch typedField := field.(type) {
		case string:
			path += fmt.Sprintf(".%s", typedField)
		case int:
			path += fmt.Sprintf("[%d]", typedField)
		default:
			// invalid. try anyway...
			path += fmt.Sprintf(".%v", typedField)
		}
	}
	return path
}
