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
	"sigs.k8s.io/cli-experimental/internal/pkg/util"

	"k8s.io/apimachinery/pkg/types"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-experimental/internal/pkg/client"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
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

	u := (*unstructured.Unstructured)(o.Resources)
	annotation := u.GetAnnotations()
	_, ok := annotation[inventory.ContentAnnotation]
	if !ok {
		return Result{}, nil
	}

	obj := u.DeepCopy()
	err := o.DynamicClient.Get(ctx,
		types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, obj)

	if err != nil {
		if errors.IsNotFound(err) {
			// no prune configmap set by apply, therefor we can't prune anything
			return Result{}, nil
		}
		fmt.Fprintf(os.Stderr, "retrieving current configuration of %s from server for %v", u.GetName(), err)
		return Result{}, err
	}
	obj, results, err := o.runPrune(ctx, obj)
	if err != nil {
		return Result{}, err
	}

	err = o.DynamicClient.Apply(context.Background(), obj)
	if err != nil {
		return Result{}, err
	}

	return Result{Resources: results}, nil
}

// runPrune deletes the obsolete objects.
// The obsolete objects is derived by parsing
// an Inventory annotation, which is defined in
// Kustomize.
//     https://github.com/kubernetes-sigs/kustomize/tree/master/pkg/inventory
// This is based on the KEP
//     https://github.com/kubernetes/enhancements/pull/810
func (o *Prune) runPrune(ctx context.Context, obj *unstructured.Unstructured) (
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

