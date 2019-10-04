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
	"sigs.k8s.io/kustomize/pkg/gvk"
	"sigs.k8s.io/kustomize/pkg/inventory"
	"sigs.k8s.io/kustomize/pkg/resid"
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

	// Flag; deletes objects previously applied, but not applied currently.
	Prune bool

	// Flag; don't actually persist created/updated/deleted resources.
	DryRun bool

	// List of previous inventory objects associated with director.
	PrevInventory []*unstructured.Unstructured

	// List of resources managed by previous inventory objects.
	PrevResources map[resid.ResId]bool
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
		fmt.Fprintf(a.Out, "\n**Dry Run...not actually persisting**\n")
	}
	fmt.Fprintf(a.Out, "\n")

	var director *unstructured.Unstructured
	var inventoryRef *metav1.OwnerReference
	for _, u := range normalizeResourceOrdering(a.Resources) {
		if util.HasAnnotation(u, inventory.ContentAnnotation) {
			// Validate that i == 0
			if director == nil {
				var err error
				director, err = a.getInventoryDirector(u)
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
				a.getPrevInventory(director)
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
				fmt.Fprintf(a.Out, "failed to apply the object: %s/%s: %v\n",
					u.GetKind(), u.GetName(), err)
				continue
			}
		}
		// TODO: clean this conversion up
		g := u.GroupVersionKind()
		currentGvk := gvk.Gvk{
			Group:   g.Group,
			Version: g.Version,
			Kind:    g.Kind,
		}
		currentResId := resid.NewResIdWithNamespace(currentGvk,
			u.GetName(), u.GetNamespace())
		if _, ok := a.PrevResources[currentResId]; ok {
			a.PrevResources[currentResId] = false
			fmt.Fprintf(a.Out, "[updated] %s/%s\n", u.GetKind(), u.GetName())
		} else {
			fmt.Fprintf(a.Out, "[created] %s/%s\n", u.GetKind(), u.GetName())
		}

		// Create after applying, so the UID field is included.
		if util.HasAnnotation(u, inventory.ContentAnnotation) {
			inventoryRef = createOwnerReference(u)
		}
	}

	if a.Prune {
		for resId, exists := range a.PrevResources {
			if exists {
				fmt.Fprintf(a.Out, "[deleted] %s/%s\n", resId.Gvk.Kind, resId.Name)
			}
		}
		a.prune()
	} else {
		for resId, exists := range a.PrevResources {
			if exists {
				fmt.Fprintf(a.Out, "[unmodified] %s/%s\n", resId.Gvk.Kind, resId.Name)
			}
		}
	}

	return Result{Resources: a.Resources}, nil
}

// Gets and stores the previous inventory objects in the Apply struct.
func (a *Apply) getPrevInventory(director *unstructured.Unstructured) error {
	// Get the director InventoryId annotation to identify inventory objects.
	annotations := director.GetAnnotations()
	if annotations == nil {
		return fmt.Errorf("Inventory director missing `InventoryId` annotation")
	}
	if _, ok := annotations[InventoryId]; !ok {
		return fmt.Errorf("Inventory director missing `InventoryId` annotation")
	}
	inventoryId := annotations[InventoryId]

	// Retrieve inventory object with the InventoryId label.
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
		return fmt.Errorf("Error etrieving previous inventory objects: %#v", err)
	}
	// Store the resource ids from each inventory object.
	for _, inv := range prevInventory.Items {
		newInv := inventory.NewInventory()
		err := newInv.LoadFromAnnotation(inv.GetAnnotations())
		if err != nil {
			return err
		}
		for resId, _ := range newInv.Current {
			//fmt.Fprintf(a.Out, "Storing resource id: %s\n", resId.String())
			if a.PrevResources == nil {
				a.PrevResources = make(map[resid.ResId]bool)
			}
			a.PrevResources[resId] = true
		}
		// Store the previous inventory object also.
		a.PrevInventory = append(a.PrevInventory, &inv)
	}

	return nil
}

func (a *Apply) getInventoryDirector(inv *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	director := inv.DeepCopy()
	err := a.DynamicClient.Get(context.Background(),
		types.NamespacedName{Namespace: inv.GetNamespace(), Name: inv.GetName()}, director)
	if err != nil {
		return nil, err
	}
	return director, nil
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
	fmt.Fprintf(a.Out, "[created] %s/%s\n", director.GetKind(), inv.GetName())

	return director, nil
}

func (a *Apply) prune() {
	for _, inv := range a.PrevInventory {
		// TODO: Look closer at DeleteOptions; esp. DeletePropogation
		if !a.DryRun {
			err := a.DynamicClient.Delete(context.Background(), inv, &metav1.DeleteOptions{})
			if err != nil {
				fmt.Fprintf(a.Out, "Error deleting inventory object: %#v", err)
			}
		}
		fmt.Fprintf(a.Out, "[deleted:meta] %s/%s\n", inv.GetKind(), inv.GetName())
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
