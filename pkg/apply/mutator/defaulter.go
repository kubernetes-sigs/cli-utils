// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"
)

type Defaulter struct {
	Mapper meta.RESTMapper
}

// Name returns a mutator identifier for logging.
func (d *Defaulter) Name() string {
	return "Defaulter"
}

// Mutate parses the apply-time-mutation annotation and loops through the
// substitutions, checking each source resource. If not specified, the source
// resource namespace is inherited from the target resource, as long as both the
// target resource and source resource are namespaced.
func (d *Defaulter) Mutate(ctx context.Context, obj *unstructured.Unstructured) (bool, string, error) {
	mutated := false
	reason := ""

	subs, err := mutation.ReadAnnotation(obj)
	if err != nil {
		return mutated, reason, err
	}
	namespace := obj.GetNamespace()
	newSubs, err := d.applyDefaultNamespace(subs, obj.GetNamespace())
	if err != nil {
		return mutated, reason, err
	}
	if !newSubs.Equal(subs) {
		klog.V(5).Infof("updated mutation after defaults:\n%s", object.YamlStringer{O: subs})
		err = mutation.WriteAnnotation(obj, newSubs)
		if err != nil {
			return mutated, reason, err
		}
		mutated = true
		reason = fmt.Sprintf("annotation value updated to inherit namespace (annotation: %q, namespace: %q)", mutation.Annotation, namespace)
	}
	return mutated, reason, nil
}

// return fmt.Errorf("failed to update resource (%s) with defaults for apply-time-mutation: %w", targetRef, err)

func (d *Defaulter) applyDefaultNamespace(subs mutation.ApplyTimeMutation, namespace string) (mutation.ApplyTimeMutation, error) {
	newSubs := make(mutation.ApplyTimeMutation, len(subs))
	for i, sub := range subs {
		// lookup REST mapping
		sourceMapping, err := d.getMapping(sub.SourceRef)
		if err != nil {
			// If we can't find a match, just keep going. This can happen
			// if CRDs and CRs are applied at the same time.
			//
			// As long as Mutate() also applies the default namespace,
			// the only case that can't use namespace defaulting is resources
			// applied asynchrounously by another client, with  a CRD in this
			// apply set,
			if meta.IsNoMatchError(err) {
				klog.V(5).Infof("source resource (%s) scope: unknown", sub.SourceRef)
				newSubs[i] = sub
				continue
			}
			return subs, fmt.Errorf("failed to identify source resource mapping (%s): %w", sub.SourceRef, err)
		}

		klog.V(5).Infof("source resource (%s) scope: %s", sub.SourceRef, sourceMapping.Scope.Name())

		// Default source namespace to target namesapce, if namespace-scoped
		if sub.SourceRef.Namespace == "" && sourceMapping.Scope.Name() == meta.RESTScopeNameNamespace {
			// namespace required
			if namespace == "" {
				// Empty namespace could mean an invalid target resource
				// OR a cluster-scoped target resource.
				// But we'll use the same error for both,
				// to avoid needing to look up the mapping.
				return subs, fmt.Errorf("failed to inherit namespace for source resource reference (%s): target resource namespace is empty", sub.SourceRef)
			}
			sub.SourceRef.Namespace = namespace
			klog.V(5).Infof("source resource (%s) inherited target resource namespace (%s)", sub.SourceRef, namespace)
		}
		newSubs[i] = sub
	}
	return newSubs, nil
}

func (d *Defaulter) getMapping(ref mutation.ResourceReference) (*meta.RESTMapping, error) {
	// lookup resource using group api version, if specified
	sourceGvk := ref.GroupVersionKind()
	var mapping *meta.RESTMapping
	var err error
	if sourceGvk.Version != "" {
		mapping, err = d.Mapper.RESTMapping(sourceGvk.GroupKind(), sourceGvk.Version)
	} else {
		mapping, err = d.Mapper.RESTMapping(sourceGvk.GroupKind())
	}
	if err != nil {
		return nil, err
	}
	return mapping, nil
}
