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

package patch

import (
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/apimachinery/pkg/util/mergepatch"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes/scheme"
)

// Patch encapsulates a Kubernetes patch operation data
// Patch contains the patch type as well as the patch request contents.
type Patch struct {
	Type types.PatchType
	Data []byte
}

var metadataAccessor = meta.NewAccessor()

// GetClientSideApplyPatch updates a resource using the same approach as running `kubectl apply`.
// The implementation here has been mostly extracted from the apply command: k8s.io/kubernetes/pkg/kubectl/cmd/apply.go
func GetClientSideApplyPatch(current, desired runtime.Object) (Patch, error) {
	var patch Patch
	// Serialize the current configuration of the object.
	currentb, err := runtime.Encode(unstructured.UnstructuredJSONScheme, current)
	if err != nil {
		return patch, fmt.Errorf("could not serialize current %v", current)
	}
	// Retrieve the last applied configuration of the object from the annotation.
	last, err := GetLastApplied(current)
	if err != nil {
		return patch, fmt.Errorf("could not retrieve previously applied configuration from annotation on %v", desired)
	}

	// Serialize the modified configuration of the object, populating the last applied annotation as well.
	modified, err := SerializeLastApplied(desired, true)
	if err != nil {
		return patch, fmt.Errorf("could not serialize intended configuration from %v", desired)
	}
	gvk := desired.GetObjectKind().GroupVersionKind()

	versionedObject, err := Scheme.New(gvk)
	_, unversioned := Scheme.IsUnversioned(desired)
	switch {
	case runtime.IsNotRegisteredError(err) || unversioned:
		// create 3-way patch for CRD, version-skewed or unknown type
		preconditions := []mergepatch.PreconditionFunc{
			mergepatch.RequireKeyUnchanged("apiVersion"),
			mergepatch.RequireKeyUnchanged("kind"),
			mergepatch.RequireMetadataKeyUnchanged("name"),
		}
		patch.Data, err = jsonmergepatch.CreateThreeWayJSONMergePatch(last, modified, currentb, preconditions...)
		patch.Type = types.MergePatchType
		if err != nil {
			name, err := metadataAccessor.Name(desired)
			if mergepatch.IsPreconditionFailed(err) {
				return patch, fmt.Errorf("at least one of apiVersion, kind and name was changed for %s/%s", gvk, name)
			}
			return patch, err
		}
	case err == nil:
		// create 3-way patch for compiled in type
		patch.Type = types.StrategicMergePatchType
		lookupPatchMeta, err := strategicpatch.NewPatchMetaFromStruct(versionedObject)
		if err != nil {
			return patch, err
		}
		patch.Data, err = strategicpatch.CreateThreeWayMergePatch(last, modified, currentb, lookupPatchMeta, true)
		if err != nil {
			return patch, err
		}
	case err != nil:
		return patch, err
	}
	return patch, nil
}

// GetMergePatch - generate merge patch from original and modified objects
func GetMergePatch(original, modified runtime.Object) (Patch, error) {
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return Patch{}, err
	}

	modifiedJSON, err := json.Marshal(modified)
	if err != nil {
		return Patch{}, err
	}

	patch, err := jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return Patch{}, err
	}
	return Patch{types.MergePatchType, patch}, nil
}

// GetLastApplied retrieves the original configuration of the object
// from the annotation, or nil if no annotation was found.
func GetLastApplied(obj runtime.Object) ([]byte, error) {
	annots, err := metadataAccessor.Annotations(obj)
	if err != nil {
		return nil, err
	}

	if annots == nil {
		return nil, nil
	}

	original, ok := annots[v1.LastAppliedConfigAnnotation]
	if !ok {
		return nil, nil
	}

	return []byte(original), nil
}

// SetLastApplied patch
func SetLastApplied(obj runtime.Object) error {
	modified, err := SerializeLastApplied(obj, false)
	if err != nil {
		return err
	}
	annots, err := metadataAccessor.Annotations(obj)
	if err != nil {
		return err
	}

	if annots == nil {
		annots = map[string]string{}
	}

	annots[v1.LastAppliedConfigAnnotation] = string(modified)
	return metadataAccessor.SetAnnotations(obj, annots)
}

// SerializeLastApplied returns the json serialization of the incoming object
// If annotate is true, the returned serialization will contain a serialized annotation
// of itself (kubectl.kubernetes.io/last-applied-configuration), otherwise it
// will not have this annotation.
func SerializeLastApplied(obj runtime.Object, annotate bool) (modified []byte, err error) {
	// First serialize the object without the annotation to prevent recursion,
	// then add that serialization to it as the annotation and serialize it again.
	annots, err := metadataAccessor.Annotations(obj)
	if err != nil {
		return nil, err
	}
	if annots == nil {
		annots = map[string]string{}
	}

	// set the LastAppliedConfigAnnotation to be the passed in object
	original := annots[v1.LastAppliedConfigAnnotation]
	delete(annots, v1.LastAppliedConfigAnnotation)

	// Restore the object to its original condition.
	defer func() {
		annots[v1.LastAppliedConfigAnnotation] = original
		err = metadataAccessor.SetAnnotations(obj, annots)
	}()

	if err = metadataAccessor.SetAnnotations(obj, annots); err != nil {
		return nil, err
	}

	modified, err = runtime.Encode(unstructured.UnstructuredJSONScheme, obj)
	if err != nil {
		return nil, err
	}

	if annotate {
		annots[v1.LastAppliedConfigAnnotation] = string(modified)
		if err = metadataAccessor.SetAnnotations(obj, annots); err != nil {
			return nil, err
		}

		modified, err = runtime.Encode(unstructured.UnstructuredJSONScheme, obj)
		if err != nil {
			return nil, err
		}
	}

	return modified, nil
}

// init
func init() {
	utilruntime.Must(scheme.AddToScheme(Scheme))
}
