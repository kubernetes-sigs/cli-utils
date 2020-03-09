// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// CaptureIdentifiersFilter implements the Filter interface in the kio
// package. It captures the identifiers for all resources passed through
// the pipeline.
type CaptureIdentifiersFilter struct {
	Identifiers []object.ObjMetadata
	Mapper      meta.RESTMapper
}

var _ kio.Filter = &CaptureIdentifiersFilter{}

func (f *CaptureIdentifiersFilter) Filter(slice []*yaml.RNode) ([]*yaml.RNode,
	error) {
	for i := range slice {
		objectMeta, err := slice[i].GetMeta()
		if err != nil {
			return nil, err
		}
		id := objectMeta.GetIdentifier()
		gv, err := schema.ParseGroupVersion(id.APIVersion)
		if err != nil {
			return nil, err
		}
		gk := schema.GroupKind{
			Group: gv.Group,
			Kind:  id.Kind,
		}
		mapping, err := f.Mapper.RESTMapping(gk)
		if err != nil {
			return nil, err
		}
		var namespace string
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace &&
			id.Namespace == "" {
			namespace = "default"
		} else {
			namespace = id.Namespace
		}
		// We only want to add yaml that actually represents Kubernetes resources.
		// We also need to filter out grouping object templates, since there will
		// never be an actual resource with that name and namespace.
		if isValidKubernetesResource(id) && !isGroupingObject(objectMeta.Labels) {
			f.Identifiers = append(f.Identifiers, object.ObjMetadata{
				Name:      id.Name,
				Namespace: namespace,
				GroupKind: schema.GroupKind{
					Group: gv.Group,
					Kind:  id.Kind,
				},
			})
		}
	}
	return slice, nil
}

// isValidKubernetesResource checks if a yaml structure has the properties
// we expect to see in all Kubernetes resources.
func isValidKubernetesResource(id yaml.ResourceIdentifier) bool {
	return id.GetKind() != "" && id.GetAPIVersion() != "" && id.GetName() != ""
}

// isGroupingObject checks if the provided map of labels contain the
// inventory object label key.
func isGroupingObject(labels map[string]string) bool {
	for key := range labels {
		if key == prune.GroupingLabel {
			return true
		}
	}
	return false
}
