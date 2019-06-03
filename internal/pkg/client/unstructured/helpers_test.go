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

package unstructured_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	helperu "sigs.k8s.io/cli-experimental/internal/pkg/client/unstructured"
)

var emptyObj = map[string]interface{}{}
var testObj = map[string]interface{}{
	"f1": map[string]interface{}{
		"f1f2": map[string]interface{}{
			"f1f2i32":   int32(32),
			"f1f2i64":   int64(64),
			"f1f2float": 64.02,
			"f1f2ms": []interface{}{
				map[string]interface{}{"f1f2ms0f1": 22},
				map[string]interface{}{"f1f2ms1f1": "index1"},
			},
			"f1f2msbad": []interface{}{
				map[string]interface{}{"f1f2ms0f1": 22},
				32,
			},
		},
	},
	"f2": map[string]interface{}{
		"f2f2": map[string]interface{}{},
	},

	"ride": "dragon",

	"status": map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"f1f2ms0f1": 22},
			map[string]interface{}{"f1f2ms1f1": "index1"},
		},
	},
}

func TestNestedInt(t *testing.T) {
	v, found, err := helperu.NestedInt(testObj, "f1", "f1f2", "f1f2i32")
	assert.NoError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, int(32), v)

	v, found, err = helperu.NestedInt(testObj, "f1", "f1f2", "wrongname")
	assert.NoError(t, err)
	assert.Equal(t, found, false)
	assert.Equal(t, int(0), v)

	v, found, err = helperu.NestedInt(testObj, "f1", "f1f2", "f1f2i64")
	assert.NoError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, int(64), v)

	v, found, err = helperu.NestedInt(testObj, "f1", "f1f2", "f1f2float")
	assert.Error(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, int(0), v)
}

func TestGetStringField(t *testing.T) {
	v := helperu.GetStringField(testObj, "ride", "horse")
	assert.Equal(t, v, "dragon")

	v = helperu.GetStringField(testObj, "destination", "north")
	assert.Equal(t, v, "north")
}

func TestNestedMapSlice(t *testing.T) {
	v, found, err := helperu.NestedMapSlice(testObj, "f1", "f1f2", "f1f2ms")
	assert.NoError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, []map[string]interface{}{
		map[string]interface{}{"f1f2ms0f1": 22},
		map[string]interface{}{"f1f2ms1f1": "index1"},
	}, v)

	v, found, err = helperu.NestedMapSlice(testObj, "f1", "f1f2", "f1f2msbad")
	assert.Error(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, []map[string]interface{}(nil), v)

	v, found, err = helperu.NestedMapSlice(testObj, "f1", "f1f2", "wrongname")
	assert.NoError(t, err)
	assert.Equal(t, found, false)
	assert.Equal(t, []map[string]interface{}(nil), v)

	v, found, err = helperu.NestedMapSlice(testObj, "f1", "f1f2", "f1f2i64")
	assert.Error(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, []map[string]interface{}(nil), v)
}

func TestGetConditions(t *testing.T) {
	v := helperu.GetConditions(emptyObj)
	assert.Equal(t, []map[string]interface{}{}, v)

	v = helperu.GetConditions(testObj)
	assert.Equal(t, []map[string]interface{}{
		map[string]interface{}{"f1f2ms0f1": 22},
		map[string]interface{}{"f1f2ms1f1": "index1"},
	}, v)
}
