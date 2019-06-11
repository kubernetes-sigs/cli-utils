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

package pkg

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-experimental/internal/pkg/apply"
	"sigs.k8s.io/cli-experimental/internal/pkg/delete"
	"sigs.k8s.io/cli-experimental/internal/pkg/prune"
	"sigs.k8s.io/cli-experimental/internal/pkg/status"
)

// TODO
// Standardize these APIs so that this library could be used by others
// ideally we would need something like this:
// cmd := NewCommand(client, ...)
// result := cmd.Apply(unstructures_list)
// result := cmd.Status(unstructures_list)

// Cmd is a wrapper for different structs:
//   apply, prune and delete
// These structs share the same client
type Cmd struct {
	Applier      *apply.Apply
	Pruner       *prune.Prune
	Deleter      *delete.Delete
	StatusGetter *status.Status
}

// Apply applies resources given the input as a slice of unstructured resources
func (a *Cmd) Apply(resources []*unstructured.Unstructured) error {
	a.Applier.Resources = resources
	_, err := a.Applier.Do()
	return err
}

// Prune prunes resources given the input as a slice of unstructured resources
func (a *Cmd) Prune(resources []*unstructured.Unstructured) error {
	a.Pruner.Resources = resources
	_, err := a.Pruner.Do()
	return err
}

// Delete deletes resources given the input as a slice of unstructured resources
func (a *Cmd) Delete(resources []*unstructured.Unstructured) error {
	a.Deleter.Resources = resources
	_, err := a.Deleter.Do()
	return err
}

// Status returns the status given the input as a slice of unstructured resources
func (a *Cmd) Status(resources []*unstructured.Unstructured) error {
	a.StatusGetter.Resources = resources
	_ = a.StatusGetter.Do()
	return nil
}
