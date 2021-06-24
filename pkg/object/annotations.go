// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package object

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// Depends-on annotation constants.
const (
	DependsOnAnnotation = "config.kubernetes.io/depends-on"
	// Number of fields for a cluster-scoped depends-on object value. Example:
	//   rbac.authorization.k8s.io/ClusterRole/my-cluster-role-name
	NumFieldsClusterScoped = 3
	// Number of fields for a namespace-scoped depends-on object value. Example:
	//   apps/namespaces/my-namespace/Deployment/my-deployment-name
	NumFieldsNamespacedScoped = 5
	// Used to separate multiple depends-on objects.
	AnnotationSeparator = ","
	// Used to separate the fields for a depends-on object value.
	FieldSeparator  = "/"
	NamespacesField = "namespaces"
)

// HasAnnotation returns the annotation value and true if the passed annotation
// is present in the as one of the keys in the annotations map for the passed
// object; empty string and false otherwise.
func HasAnnotation(u *unstructured.Unstructured, key string) (string, bool) {
	if u == nil {
		return "", false
	}
	annotations := u.GetAnnotations()
	value, found := annotations[key]
	return value, found
}

// DependsOnObjs returns the slice of object references (ObjMetadata)
// that the passed unstructured object depends on based on an
// annotation within the passed unstructured object.
func DependsOnObjs(u *unstructured.Unstructured) ([]ObjMetadata, error) {
	objs := []ObjMetadata{}
	if u == nil {
		return objs, nil
	}
	objsEncoded, found := HasAnnotation(u, DependsOnAnnotation)
	if !found {
		return objs, nil
	}
	klog.V(5).Infof("depends-on annotation found for %s/%s: %s\n",
		u.GetNamespace(), u.GetName(), objsEncoded)
	return DependsOnAnnotationToObjMetas(objsEncoded)
}

// annotationToObjMeta parses the passed annotation as an
// object reference. The fields are separated by '/', and the
// string can have either three fields (cluster-scoped object)
// or five fields (namespace-scoped object). Examples are:
//   Cluster-Scoped: <group>/<kind>/<name> (3 fields)
//   Namespaced: <group>/namespaces/<namespace>/<kind>/<name> (5 fields)
// For the "core" group, the string is empty.
// Return the parsed ObjMetadata object or an error if unable
// to parse the obj ref annotation string.
func DependsOnAnnotationToObjMetas(o string) ([]ObjMetadata, error) {
	objs := []ObjMetadata{}
	for _, objStr := range strings.Split(o, AnnotationSeparator) {
		var group, kind, namespace, name string
		objStr := strings.TrimSpace(objStr)
		fields := strings.Split(objStr, FieldSeparator)
		if len(fields) != NumFieldsClusterScoped &&
			len(fields) != NumFieldsNamespacedScoped {
			return objs, fmt.Errorf("unable to parse depends on annotation into ObjMetadata: %s", o)
		}
		group = fields[0]
		if len(fields) == 3 {
			kind = fields[1]
			name = fields[2]
		} else {
			if fields[1] != NamespacesField {
				return objs, fmt.Errorf("depends on annotation missing 'namespaces' field: %s", o)
			}
			namespace = fields[2]
			kind = fields[3]
			name = fields[4]
		}
		obj, err := CreateObjMetadata(namespace, name, schema.GroupKind{Group: group, Kind: kind})
		if err != nil {
			return objs, err
		}
		objs = append(objs, obj)
	}
	return objs, nil
}
