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

package status

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clientu "sigs.k8s.io/cli-experimental/internal/pkg/client/unstructured"
)

func readyConditionReader(u *unstructured.Unstructured) (bool, error) {
	conditions := clientu.GetConditions(u.UnstructuredContent())
	for _, c := range conditions {
		if clientu.GetStringField(c, "type", "") == "Ready" && clientu.GetStringField(c, "status", "") == "False" {
			return false, nil
		}
	}
	return true, nil
}

// GetGenericReadyFn - True if we handle it as a known type
func GetGenericReadyFn(u *unstructured.Unstructured) IsReadyFn {
	return readyConditionReader
}
