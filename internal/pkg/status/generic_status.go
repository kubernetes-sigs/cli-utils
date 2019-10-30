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
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clientu "sigs.k8s.io/cli-utils/internal/pkg/client/unstructured"
)

// checkGenericProperties look at the common properties for k8s resources. We
// make these checks before looking at type-specific properties.
// Currently this only checks generation and observedGeneration, but when
// we have a set of standard conditions for CRDs (and eventually the built-in
// types), these could be added here.
func checkGenericProperties(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	// Check if the resource is scheduled for deletion
	deletionTimestamp, found, err := unstructured.NestedString(obj, "metadata", "deletionTimestamp")
	if err != nil {
		return nil, fmt.Errorf("unable to fetch deletionTimestamp from resource: %v", err)
	}
	if found && deletionTimestamp != "" {
		return &Result{
			Status: TerminatingStatus,
			Message: "Resource scheduled for deletion",
			Conditions: []Condition{},
		}, nil
	}

	// ensure that the meta generation is observed
	metaGeneration := clientu.GetIntField(obj, ".metadata.generation", -1)
	observedGeneration := clientu.GetIntField(obj, ".status.observedGeneration", metaGeneration)
	if observedGeneration != metaGeneration {
		message := fmt.Sprintf("%s generation is %d, but latest observed generation is %d", u.GetKind(), metaGeneration, observedGeneration)
		return &Result{
			Status: InProgressStatus,
			Message: message,
			Conditions: []Condition{newInProgressCondition("LatestGenerationNotObserved", message)},
		}, nil
	}

	return nil, nil
}
