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

// readyConditionReader reads Ready condition from the unstructured object
func readyConditionReader(u *unstructured.Unstructured) ([]Condition, error) {
	rv := []Condition{}
	ready := false
	obj := u.UnstructuredContent()

	// ensure that the meta generation is observed
	metaGeneration := clientu.GetIntField(obj, ".metadata.generation", -1)
	observedGeneration := clientu.GetIntField(obj, ".status.observedGeneration", metaGeneration)
	if observedGeneration != metaGeneration {
		reason := "Controller has not observed the latest change. Status generation does not match with metadata"
		rv = append(rv, Condition{ConditionReady, "False", "", reason})
		return rv, nil
	}

	// Conditions
	objc := clientu.GetObjectWithConditions(obj)
	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Ready":
			ready = true
			if c.Status == "False" {
				rv = append(rv, Condition{ConditionReady, "False", c.Reason, c.Message})
			} else {
				rv = append(rv, Condition{ConditionReady, "True", c.Reason, c.Message})
			}
		}
	}
	if !ready {
		rv = append(rv, Condition{ConditionReady, "True", "NoReadyCondition", "No Ready condition found in status"})
	}

	return rv, nil
}

// GetGenericConditionsFn Return a function that returns condition for an unstructured object
func GetGenericConditionsFn(u *unstructured.Unstructured) GetConditionsFn {
	return readyConditionReader
}
