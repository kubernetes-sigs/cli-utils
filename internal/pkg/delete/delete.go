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
	"sigs.k8s.io/cli-experimental/internal/pkg/util"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-experimental/internal/pkg/client"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/kustomize/pkg/inventory"
)

// Delete applies directories
type Delete struct {
	// DynamicClient is the client used to talk
	// with the cluster
	DynamicClient client.Client

	// Out stores the output
	Out io.Writer

	// Resources is a list of resource configurations
	Resources clik8s.ResourceConfigs

	// Commit is a git commit object
	Commit *object.Commit
}

// Result contains the Apply Result
type Result struct {
	Resources clik8s.ResourceConfigs
}

// Do executes the delete
func (a *Delete) Do() (Result, error) {
	fmt.Fprintf(a.Out, "Doing `cli-experimental delete`\n")
	ctx := context.Background()
	for _, u := range normalizeResourceOrdering(a.Resources) {
		annotations := u.GetAnnotations()
		_, ok := annotations[inventory.ContentAnnotation]
		if ok {
			err := a.handleInventroy(ctx, annotations)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to delete leftovers for inventory %v\n", err)
				continue
			}
		}

		_, err := util.DeleteObject(a.DynamicClient, ctx, u.GroupVersionKind(), u.GetNamespace(), u.GetName())
		if err != nil {
			fmt.Fprint(os.Stderr, err)
		}

	}
	return Result{Resources: a.Resources}, nil
}

// handleInventory reads the inventory annotation
// and delete any object recorded in it that hasn't been deleted.
// When there is an inventory object in the resource configurations, the inventory
// object may record some objects that are applied previously and never been pruned.
// By delete command, those objects are supposed to be cleaned up as well.
func (a *Delete) handleInventroy(ctx context.Context, annotations map[string]string) error {
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
		_, err = util.DeleteObject(a.DynamicClient, ctx, gvk, id.Namespace, id.Name)
		if err != nil {
			fmt.Fprint(os.Stderr, err)
		}
	}
	return nil
}

// normalizeResourceOrdering move the inventory object to be the last resource
// This is to make sure the inventory object is the last object to be deleted.
func normalizeResourceOrdering(resources clik8s.ResourceConfigs) []*unstructured.Unstructured {
	var results []*unstructured.Unstructured
	index := -1
	for i, u := range resources {
		annotation := u.GetAnnotations()
		_, ok := annotation[inventory.ContentAnnotation]
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
