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

package apply

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/internal/pkg/client"
	"sigs.k8s.io/cli-utils/internal/pkg/clik8s"
	"sigs.k8s.io/cli-utils/internal/pkg/util"
	"sigs.k8s.io/kustomize/pkg/inventory"
)

// Apply applies directories
type Apply struct {
	// DynamicClient is the client used to talk
	// with the cluster
	DynamicClient client.Client

	// Out stores the output
	Out io.Writer

	// Resources is a list of resource configurations
	Resources clik8s.ResourceConfigs

	// Commit is a git commit object
	Commit *object.Commit

	Prune bool

	DryRun bool
}

// Result contains the Apply Result
type Result struct {
	Resources clik8s.ResourceConfigs
}

const InventoryId = "config.k8s.io/InventoryId"

// Do executes the apply
func (a *Apply) Do() (Result, error) {
	fmt.Fprintf(a.Out, "Doing `cli-utils apply`\n")

	if a.DryRun {
		fmt.Fprintf(a.Out, "Dry Run...not actually persisting\n")
	}

	var director *unstructured.Unstructured
	var currentInv *unstructured.Unstructured
	var inventoryRef *metav1.OwnerReference
	for _, u := range normalizeResourceOrdering(a.Resources) {
		if util.HasAnnotation(u, inventory.ContentAnnotation) {
			// Validate that i == 0
			if director == nil {
				director = u.DeepCopy()
				err := a.DynamicClient.Get(context.Background(),
					types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, director)
				if err != nil && !errors.IsNotFound(err) {
					fmt.Fprintf(a.Out, "Failure during inventory director search: %v\n", err)
					break
				}
				if err != nil && errors.IsNotFound(err) {
					director, err = a.createInventoryDirector(u)
					if err != nil {
						fmt.Fprintf(a.Out, "Failure creating inventory director: %v\n", err)
						break
					}
				}
			}
			directorRef := createOwnerReference(director)
			a.addOwnerReference(u, *directorRef)
			suffix := createUniqueId()
			u.SetName(u.GetName() + "-" + suffix)
			inventoryId := director.GetAnnotations()[InventoryId]
			labels := u.GetLabels()
			if labels == nil {
				labels = make(map[string]string)
			}
			labels[InventoryId] = inventoryId
			u.SetLabels(labels)
		} else if inventoryRef != nil {
			a.addOwnerReference(u, *inventoryRef)
		}

		if !a.DryRun {
			err := a.DynamicClient.Apply(context.Background(), u)
			if err != nil {
				fmt.Fprintf(a.Out, "failed to apply the object: %s/%s: %v\n", u.GetKind(), u.GetName(), err)
				continue
			}
		}
		fmt.Fprintf(a.Out, "applied %s/%s\n", u.GetKind(), u.GetName())

		// Create after applying, so the UID field is included.
		if util.HasAnnotation(u, inventory.ContentAnnotation) {
			currentInv = u
			inventoryRef = createOwnerReference(u)
		}
	}

	a.prune(director, currentInv)

	return Result{Resources: a.Resources}, nil
}

func (a *Apply) createInventoryDirector(inv *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	director := &unstructured.Unstructured{}
	director.SetGroupVersionKind(inv.GroupVersionKind())
	director.SetNamespace(inv.GetNamespace())
	director.SetName(inv.GetName())
	inventoryId := createUniqueId()
	annotations := map[string]string{InventoryId: inventoryId}
	director.SetAnnotations(annotations)
	if !a.DryRun {
		err := a.DynamicClient.Create(context.Background(), director, &metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
	}
	fmt.Fprintf(a.Out, "Created inventory director: %s/%s\n", director.GetKind(), inv.GetName())

	return director, nil
}

func (a *Apply) prune(director *unstructured.Unstructured, currentInv *unstructured.Unstructured) {
	if a.Prune && director != nil && currentInv != nil {
		fmt.Fprintf(a.Out, "Pruning...")
		annotations := director.GetAnnotations()
		if annotations == nil {
			fmt.Fprintf(a.Out, "Error: Inventory director missing InventoryId annotation")
			return
		}
		if inventoryId, ok := annotations[InventoryId]; ok {
			selector := fmt.Sprintf("%s=%s", InventoryId, inventoryId)
			listOptions := &metav1.ListOptions{LabelSelector: selector}
			prevInventory := &unstructured.UnstructuredList{}
			prevInventory.SetGroupVersionKind(schema.GroupVersionKind{
				Kind:    "ConfigMapList",
				Version: "v1",
			})
			err := a.DynamicClient.List(context.Background(), prevInventory,
				director.GetNamespace(), listOptions)
			if err != nil {
				fmt.Fprintf(a.Out, "Error retrieving previous inventory objects: %#v", err)
				return
			}
			fmt.Fprintf(a.Out, "Retrieved %d previous inventory objects\n", len(prevInventory.Items))
			for _, inv := range prevInventory.Items {
				if currentInv.GetUID() != inv.GetUID() {
					err := a.DynamicClient.Delete(context.Background(), &inv, &metav1.DeleteOptions{})
					if err != nil {
						fmt.Fprintf(a.Out, "Error deleting inventory object: %#v", err)
					}
				}
			}
		}
	}
}

// createUniqueId creates a string from the current Unix time.
func createUniqueId() string {
	// TODO: probably change this to a random number/string
	nanoSeconds := time.Now().UnixNano()
	return strconv.FormatInt(nanoSeconds, 10) // base 10
}

var falsePtr = false

func createOwnerReference(u *unstructured.Unstructured) *metav1.OwnerReference {
	if u == nil {
		return nil
	}
	// TODO: Validate the owner referent
	// TODO: Print warning if UID is empty (happens in dry-run)
	return &metav1.OwnerReference{
		APIVersion:         u.GetAPIVersion(),
		Kind:               u.GetKind(),
		Name:               u.GetName(),
		UID:                u.GetUID(),
		Controller:         &falsePtr,
		BlockOwnerDeletion: &falsePtr, // We never want to block in foreground deletion
	}
}

func (a *Apply) addOwnerReference(u *unstructured.Unstructured, ownerRef metav1.OwnerReference) {
	ownerReferences := u.GetOwnerReferences()
	// Check if the OwnerReference is already in the list.
	found := false
	for _, existingRef := range ownerReferences {
		if ownerRef.UID == existingRef.UID {
			found = true
			break
		}
	}
	// Only add the OwnerReference if it doesn't already exist in the current list.
	if !found {
		ownerReferences = append(ownerReferences, ownerRef)
		u.SetOwnerReferences(ownerReferences)
	}
}

// func (a Apply) updateInventoryObject(u *unstructured.Unstructured) (*unstructured.Unstructured, error) {
// 	obj := u.DeepCopy()
// 	err := a.DynamicClient.Get(context.Background(),
// 		types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, obj)
// 	if err != nil && !errors.IsNotFound(err) {
// 		return nil, err
// 	}
// 	if errors.IsNotFound(err) {
// 		return u, nil
// 	}

// 	oldAnnotation := obj.GetAnnotations()
// 	newAnnotation := u.GetAnnotations()
// 	oldhash, okold := oldAnnotation[inventory.HashAnnotation]
// 	newhash, oknew := newAnnotation[inventory.HashAnnotation]
// 	if okold && oknew && oldhash == newhash {
// 		return obj, nil
// 	}

// 	return mergeContentAnnotation(u, obj)
// }

// func mergeContentAnnotation(newObj, oldObj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
// 	newInv := inventory.NewInventory()
// 	err := newInv.LoadFromAnnotation(newObj.GetAnnotations())
// 	if err != nil {
// 		return nil, err
// 	}
// 	oldInv := inventory.NewInventory()
// 	err = oldInv.LoadFromAnnotation(oldObj.GetAnnotations())
// 	if err != nil {
// 		return nil, err
// 	}
// 	newInv.Previous.Merge(oldInv.Previous)
// 	newInv.Previous.Merge(oldInv.Current)

// 	annotations := newObj.GetAnnotations()
// 	err = newInv.UpdateAnnotations(annotations)
// 	if err != nil {
// 		return nil, err
// 	}
// 	newObj.SetAnnotations(annotations)
// 	return newObj, nil
// }

// normalizeResourceOrdering moves the inventory object to be the first resource
func normalizeResourceOrdering(resources clik8s.ResourceConfigs) []*unstructured.Unstructured {
	var results []*unstructured.Unstructured
	index := -1
	for i, u := range resources {
		if util.HasAnnotation(u, inventory.ContentAnnotation) {
			index = i
		} else {
			results = append(results, u)
		}
	}
	if index >= 0 {
		return append([]*unstructured.Unstructured{resources[index]}, results...)
	}
	return results
}
