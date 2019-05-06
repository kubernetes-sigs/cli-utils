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

package delete

import (
	"context"
	"fmt"
	"io"
	"os"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-experimental/internal/pkg/client"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/kustomize/pkg/inventory"
)

// Delete applies directories
type Delete struct {
	DynamicClient client.Client
	Out           io.Writer
	Resources     clik8s.ResourceConfigs
	Commit        *object.Commit
}

// Result contains the Apply Result
type Result struct {
	Resources clik8s.ResourceConfigs
}

// Do executes the delete
func (a *Delete) Do() (Result, error) {
	fmt.Fprintf(a.Out, "Doing `cli-experimental delete`\n")
	for _, u := range adjustOrder(a.Resources) {
		annotations := u.GetAnnotations()
		_, ok := annotations[inventory.InventoryAnnotation]
		if ok {
			err := a.deleteLeftOvers(annotations)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to delete leftovers for inventory %v\n", err)
				continue
			}
		}

		err := a.deleteObject(u.GroupVersionKind(), u.GetNamespace(), u.GetName())
		if err != nil {
			fmt.Fprint(os.Stderr, err)
		}

	}
	return Result{Resources: a.Resources}, nil
}

func (a *Delete) deleteLeftOvers(annotations map[string]string) error {
	inv := inventory.NewInventory()
	err := inv.LoadFromAnnotation(annotations)
	if err != nil {
		return err
	}
	refs := inv.Previous
	refs.Merge(inv.Current)

	for id := range refs {
		gvk := schema.GroupVersionKind{
			Group:   id.Group,
			Version: id.Version,
			Kind:    id.Kind,
		}
		err = a.deleteObject(gvk, id.Namespace, id.Name)
		if err != nil {
			fmt.Fprint(os.Stderr, err)
		}
	}
	return nil
}

func (a *Delete) deleteObject(gvk schema.GroupVersionKind, ns, nm string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(ns)
	obj.SetName(nm)

	err := a.DynamicClient.Delete(context.Background(), obj, &metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete %s/%s: %v", gvk.Kind, nm, err)
	}
	return nil
}

// adjustOrder move the inventory object to be the last resource
func adjustOrder(resources clik8s.ResourceConfigs) []*unstructured.Unstructured {
	var results []*unstructured.Unstructured
	index := -1
	for i, u := range resources {
		annotation := u.GetAnnotations()
		_, ok := annotation[inventory.InventoryAnnotation]
		if ok {
			index = i
		} else {
			results = append(results, u)
		}
	}
	if index >= 0 {
		return append(results, resources[index])
	}
	return results
}
