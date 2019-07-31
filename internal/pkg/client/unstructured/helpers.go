/*
Copyright 2019 The Kubernetes Authors.
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
	"strings"

	api_unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ObjWithConditions Represent meta object with status.condition array
type ObjWithConditions struct {
	// Status as expected to be present in most compliant kubernetes resources
	Status ConditionStatus `json:"status"`
}

// ConditionStatus represent status with condition array
type ConditionStatus struct {
	// Array of Conditions as expected to be present in kubernetes resources
	Conditions []BasicCondition `json:"conditions"`
}

// BasicCondition fields that are expected in a condition
type BasicCondition struct {
	// Type Condition type
	Type string `json:"type"`
	// Status is one of True,False,Unknown
	Status string `json:"status"`
	// Reason simple single word reason in CamleCase
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message human readable reason
	// +optional
	Message string `json:"message,omitempty"`
}

// GetObjectWithConditions return typed object
func GetObjectWithConditions(in map[string]interface{}) *ObjWithConditions {
	var out = new(ObjWithConditions)
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(in, out)
	if err != nil {
		fmt.Printf("err: %s", err)
	}
	return out
}

// GetStringField return field as string defaulting to value if not found
func GetStringField(obj map[string]interface{}, fieldPath string, defaultValue string) string {
	var rv = defaultValue

	fields := strings.Split(fieldPath, ".")
	if fields[0] == "" {
		fields = fields[1:]
	}

	val, found, err := api_unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return rv
	}

	switch val.(type) {
	case string:
		rv = val.(string)
	}
	return rv
}

// GetIntField return field as string defaulting to value if not found
func GetIntField(obj map[string]interface{}, fieldPath string, defaultValue int) int {
	var rv = defaultValue

	fields := strings.Split(fieldPath, ".")
	if fields[0] == "" {
		fields = fields[1:]
	}

	val, found, err := api_unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return rv
	}

	switch val.(type) {
	case int:
		rv = val.(int)
	case int32:
		rv = int(val.(int32))
	case int64:
		rv = int(val.(int64))
	}
	return rv
}
