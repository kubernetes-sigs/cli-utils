package util

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-experimental/internal/pkg/client"
	"sigs.k8s.io/cli-experimental/internal/pkg/constants"
)

// DeleteObject delete an object given a client and Group,Version,Kind,Name,Namespace of an object
func DeleteObject(c client.Client, ctx context.Context, gvk schema.GroupVersionKind, ns, nm string) (
	*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(ns)
	obj.SetName(nm)

	err := c.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      nm,
	}, obj)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get %s/%s: %v", gvk.Kind, nm, err)
	}

	annotations := obj.GetAnnotations()
	if presence, ok := annotations[constants.Presence]; ok {
		if presence == constants.EnsureExist {
			// not delete the resource
			return nil, nil
		}
	}
	err = c.Delete(ctx, obj, &metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to delete %s/%s: %v", gvk.Kind, nm, err)
	}
	return obj, nil
}
