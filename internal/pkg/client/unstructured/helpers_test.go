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

var testObj = map[string]interface{}{
	"f1": map[string]interface{}{
		"f2": map[string]interface{}{
			"i32":   int32(32),
			"i64":   int64(64),
			"float": 64.02,
			"ms": []interface{}{
				map[string]interface{}{"f1f2ms0f1": 22},
				map[string]interface{}{"f1f2ms1f1": "index1"},
			},
			"msbad": []interface{}{
				map[string]interface{}{"f1f2ms0f1": 22},
				32,
			},
		},
	},

	"ride": "dragon",

	"status": map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"f1f2ms0f1": 22},
			map[string]interface{}{"f1f2ms1f1": "index1"},
		},
	},
}

func TestGetIntField(t *testing.T) {
	v := helperu.GetIntField(testObj, ".f1.f2.i32", -1)
	assert.Equal(t, int(32), v)

	v = helperu.GetIntField(testObj, ".f1.f2.wrongname", -1)
	assert.Equal(t, int(-1), v)

	v = helperu.GetIntField(testObj, ".f1.f2.i64", -1)
	assert.Equal(t, int(64), v)

	v = helperu.GetIntField(testObj, ".f1.f2.float", -1)
	assert.Equal(t, int(-1), v)
}

func TestGetStringField(t *testing.T) {
	v := helperu.GetStringField(testObj, ".ride", "horse")
	assert.Equal(t, v, "dragon")

	v = helperu.GetStringField(testObj, ".destination", "north")
	assert.Equal(t, v, "north")
}
