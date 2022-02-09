// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeepCopyObjectMetaInto copies all the fields supported by metav1.Object.
// If the metav1.Object interface ever changes, this will need to change too!
// But this implimentation at least avoids reflection and json round trip.
func DeepCopyObjectMetaInto(from, to metav1.Object) {
	to.SetNamespace(from.GetNamespace())
	to.SetName(from.GetName())
	to.SetGenerateName(from.GetGenerateName())
	to.SetUID(from.GetUID())
	to.SetResourceVersion(from.GetResourceVersion())
	to.SetGeneration(from.GetGeneration())
	to.SetSelfLink(from.GetSelfLink())
	to.SetCreationTimestamp(from.GetCreationTimestamp())
	to.SetClusterName(from.GetClusterName())

	deletionTimestamp := from.GetDeletionTimestamp()
	if deletionTimestamp != nil {
		to.SetDeletionTimestamp(deletionTimestamp.DeepCopy())
	} else {
		to.SetDeletionTimestamp(nil)
	}

	deletionGracePeriodSeconds := from.GetDeletionGracePeriodSeconds()
	if deletionGracePeriodSeconds != nil {
		to.SetDeletionGracePeriodSeconds(&(*deletionGracePeriodSeconds))
	} else {
		to.SetDeletionGracePeriodSeconds(nil)
	}

	labels := from.GetLabels()
	if labels != nil {
		c := make(map[string]string, len(labels))
		for key, val := range labels {
			c[key] = val
		}
		to.SetLabels(c)
	} else {
		to.SetLabels(nil)
	}

	annotations := from.GetAnnotations()
	if labels != nil {
		c := make(map[string]string, len(annotations))
		for key, val := range annotations {
			c[key] = val
		}
		to.SetAnnotations(c)
	} else {
		to.SetAnnotations(nil)
	}

	finalizers := from.GetFinalizers()
	if finalizers != nil {
		c := make([]string, len(finalizers))
		copy(c, finalizers)
		to.SetFinalizers(c)
	} else {
		to.SetFinalizers(nil)
	}

	ownerReferences := from.GetOwnerReferences()
	if ownerReferences != nil {
		c := make([]metav1.OwnerReference, len(ownerReferences))
		for i := range ownerReferences {
			ownerReferences[i].DeepCopyInto(&(c)[i])
		}
		to.SetOwnerReferences(c)
	} else {
		to.SetOwnerReferences(nil)
	}

	managedFields := from.GetManagedFields()
	if ownerReferences != nil {
		c := make([]metav1.ManagedFieldsEntry, len(managedFields))
		for i := range managedFields {
			managedFields[i].DeepCopyInto(&(c)[i])
		}
		to.SetManagedFields(c)
	} else {
		to.SetManagedFields(nil)
	}
}
