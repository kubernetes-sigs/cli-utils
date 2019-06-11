package util

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
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
		if presence == constants.PreventDeletion {
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

func ObjectExist(c client.Client, ctx context.Context, u *unstructured.Unstructured) (bool, error) {
	key := types.NamespacedName{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
	}
	err := c.Get(ctx, key, u)
	if err == nil {
		return true, nil
	}
	if errors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

// MatchAnnotations checks if an unstructured matches a key, value pair in its annotation
func MatchAnnotations(u *unstructured.Unstructured, annotations map[string]string) bool {
	if u.GetAnnotations() == nil {
		return false
	}
	s := labels.SelectorFromSet(labels.Set(annotations))
	if s.Matches(labels.Set(u.GetAnnotations())) {
		return true
	}
	return false
}

// HasAnnotation checks if an unstructured has a given annotation key
func HasAnnotation(u *unstructured.Unstructured, key string) bool {
	annotation := u.GetAnnotations()
	if annotation == nil {
		return false
	}
	_, ok := annotation[key]
	if ok {
		return true
	}
	return false
}
