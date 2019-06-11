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

package prune

import (
	"context"
	"fmt"
	"io"
	"os"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-experimental/internal/pkg/client"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/constants"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"sigs.k8s.io/kustomize/pkg/inventory"
)

// Prune prunes obsolete resources from a kustomization directory
// that are applied in previous applies but not show up in the
// latest apply.
type Prune struct {
	// DynamicClient is the client used to talk
	// with the cluster
	DynamicClient client.Client

	// Out stores the output
	Out io.Writer

	// Resources is the resource used for pruning
	Resources clik8s.ResourcePruneConfigs

	// Commit is a git commit object
	Commit *object.Commit
}

// Result contains the Prune Result
type Result struct {
	Resources clik8s.ResourceConfigs
}

// Do executes the prune
func (o *Prune) Do() (Result, error) {
	if o.Resources == nil {
		return Result{}, nil
	}
	fmt.Fprintf(o.Out, "Doing `cli-experimental prune`\n")
	ctx := context.Background()

	u, found, err := o.findInventoryObject()
	if err != nil {
		return Result{}, nil
	}

	// Couldn't find an inventory object
	// Will handle the pruning from
	// annotation kubectl.kubernetes.io/presence: EnsureDoesNotExist
	if !found {
		result, err := o.runPruneWithAnnotation(ctx)
		if err != nil {
			return Result{}, err
		}
		return Result{result}, nil
	}

	obj := u.DeepCopy()
	err = o.DynamicClient.Get(ctx,
		types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, obj)

	if err != nil {
		if errors.IsNotFound(err) {
			// no prune configmap set by apply, therefor we can't prune anything
			return Result{}, nil
		}
		fmt.Fprintf(os.Stderr, "retrieving current configuration of %s from server for %v", u.GetName(), err)
		return Result{}, err
	}
	obj, results, err := o.runPruneWithInventory(ctx, obj)
	if err != nil {
		return Result{}, err
	}

	err = o.DynamicClient.Apply(context.Background(), obj)
	if err != nil {
		return Result{}, err
	}

	return Result{Resources: results}, nil
}

// runPruneWithInventory deletes the obsolete objects.
// The obsolete objects is derived by parsing
// an Inventory annotation, which is defined in
// Kustomize.
//     https://github.com/kubernetes-sigs/kustomize/tree/master/pkg/inventory
// This is based on the KEP
//     https://github.com/kubernetes/enhancements/pull/810
func (o *Prune) runPruneWithInventory(ctx context.Context, obj *unstructured.Unstructured) (
	*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	var results []*unstructured.Unstructured
	annotations := obj.GetAnnotations()
	inv := inventory.NewInventory()
	inv.LoadFromAnnotation(annotations)
	items := inv.Prune()
	for _, item := range items {
		gvk := schema.GroupVersionKind{
			Group:   item.Group,
			Version: item.Version,
			Kind:    item.Kind,
		}
		u, err := util.DeleteObject(o.DynamicClient, ctx, gvk, item.Namespace, item.Name)
		if err != nil {
			return nil, nil, err
		}
		if u != nil {
			results = append(results, u)
		}
	}
	inv.UpdateAnnotations(annotations)
	obj.SetAnnotations(annotations)
	return obj, results, nil
}

// runPruneWithAnnotation deletes the objects
// that are with the annotation
// kubectl.kubernetes.io/presence: EnsureDoesNotExist
func (o *Prune) runPruneWithAnnotation(ctx context.Context) (
	[]*unstructured.Unstructured, error) {
	var results []*unstructured.Unstructured
	for _, r := range o.Resources {
		annotation := r.GetAnnotations()
		presence, ok := annotation[constants.Presence]
		if ok && presence == constants.EnsureNoExist {
			u, err := util.DeleteObject(o.DynamicClient, ctx, r.GroupVersionKind(), r.GetNamespace(), r.GetName())
			if err != nil {
				return nil, err
			}
			if u != nil {
				results = append(results, u)
			}
		}
	}
	return results, nil
}

// findInventoryObject find if there is an inventory object in
// the resources
func (o *Prune) findInventoryObject() (*unstructured.Unstructured, bool, error) {
	var u *unstructured.Unstructured
	found := false
	for _, r := range o.Resources {
		annotation := r.GetAnnotations()
		_, ok := annotation[inventory.ContentAnnotation]
		if ok {
			if !found {
				found = true
				u = r
			} else {
				return nil, false,
					fmt.Errorf("multiple objects with the inventory annotation")
			}
		}
	}
	return u, found, nil
}
