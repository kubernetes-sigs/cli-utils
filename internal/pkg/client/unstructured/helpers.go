/*
Copyright 2015 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package unstructured

import (
	"fmt"
	api_unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"strings"
)

func jsonPath(fields []string) string {
	return "." + strings.Join(fields, ".")
}

// NestedInt returns the int value of a nested field.
// Returns false if value is not found and an error if not an int
func NestedInt(obj map[string]interface{}, fields ...string) (int, bool, error) {
	var i int
	var i32 int32
	var i64 int64
	var ok bool

	val, found, err := api_unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return 0, found, err
	}
	i, ok = val.(int)
	if !ok {
		i32, ok = val.(int32)
		if ok {
			i = int(i32)
		}
	}
	if !ok {
		i64, ok = val.(int64)
		if ok {
			i = int(i64)
		}
	}
	if !ok {
		return 0, true, fmt.Errorf("%v accessor error: %v is of the type %T, expected int", jsonPath(fields), val, val)
	}
	return i, true, nil
}

// NestedMapSlice returns the value of a nested field.
// Returns false if value is not found and an error if not an slice of maps.
func NestedMapSlice(obj map[string]interface{}, fields ...string) ([]map[string]interface{}, bool, error) {
	val, found, err := api_unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	array, ok := val.([]interface{})
	if !ok {
		return nil, true, fmt.Errorf("%v accessor error: %v is of the type %T, expected []interface{}", jsonPath(fields), val, val)
	}

	conditions := []map[string]interface{}{}

	for i := range array {
		entry, ok := array[i].(map[string]interface{})
		if !ok {
			return nil, true, fmt.Errorf("%v accessor error: %v[%d] is of the type %T, expected map[string]interface{}", jsonPath(fields), i, val, val)
		}
		conditions = append(conditions, entry)

	}
	return conditions, true, nil
}

// GetStringField - return field as string defaulting to value if not found
func GetStringField(obj map[string]interface{}, field, defaultValue string) string {
	value := defaultValue
	fieldV, ok := obj[field]
	if ok {
		stringV, ok := fieldV.(string)
		if ok {
			value = stringV
		}
	}
	return value
}

// GetConditions - return conditions array as []map[string]interface{}
func GetConditions(obj map[string]interface{}) []map[string]interface{} {
	conditions, ok, err := NestedMapSlice(obj, "status", "conditions")
	if err != nil {
		fmt.Printf("err: %s", err)
		return []map[string]interface{}{}
	}
	if !ok {
		return []map[string]interface{}{}
	}
	return conditions
}
