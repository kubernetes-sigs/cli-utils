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

/*
Package status provides primitives for extracting standardized conditions from
unstructured objects.

# Standardized Conditions
The package generates standardize conditions for core kubernetes resources.
The Status conditions are grouped into these categories:
- Level
- Terminal
- Progress


Level Conditions:
Conditions that indicate stability, availability of affected objects. The controller in most cases does not have any pending work reconciling the .spec. The controller continues to react to spec changes and external inputs.
- ConditionReady
- ConditionSettled

Terminal Conditions:
Conditions that indicate terminal conditions for the resource. Usually the controller does not do any more work and does not react to external inputs. Spec changes may be honored.
- ConditionFailed
- ConditionCompleted
- ConditionTerminating

Progress Conditions:
These conditions indicating progress. Indicates controller is actively working on the resource.
- ConditionProgress


# Resources
Custom client side logic is added to handle a set of core kubernetes resources.
This is implemented in legacy_status.go
For resources not matching legacy resource list and custom resources we attempt to look for standard conditions.

*/
package status
