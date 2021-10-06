// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/jsonpath"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"
)

// ApplyTimeMutator mutates a resource by injecting values specified by the
// apply-time-mutation annotation.
// The optional ResourceCache will be used to speed up source resource lookups,
// if specified.
// Implements the Mutator interface
type ApplyTimeMutator struct {
	Client        dynamic.Interface
	Mapper        meta.RESTMapper
	ResourceCache cache.ResourceCache
}

// Name returns a mutator identifier for logging.
func (atm *ApplyTimeMutator) Name() string {
	return "ApplyTimeMutator"
}

// Mutate parses the apply-time-mutation annotation and loops through the
// substitutions, applying each of them to the supplied target object.
// Returns true with a reason, if mutation was performed.
func (atm *ApplyTimeMutator) Mutate(ctx context.Context, obj *unstructured.Unstructured) (bool, string, error) {
	mutated := false
	reason := ""

	targetRef := mutation.NewResourceReference(obj)

	if !mutation.HasAnnotation(obj) {
		return mutated, reason, nil
	}

	subs, err := mutation.ReadAnnotation(obj)
	if err != nil {
		return mutated, reason, fmt.Errorf("failed to read annotation in resource (%s): %w", targetRef, err)
	}

	klog.V(4).Infof("target resource (%s):\n%s", targetRef, object.YamlStringer{O: obj})

	// validate no self-references
	// Early validation to avoid GETs, but won't catch sources with implicit namespace.
	for _, sub := range subs {
		if targetRef.Equal(sub.SourceRef) {
			return mutated, reason, fmt.Errorf("invalid self-reference (%s)", sub.SourceRef)
		}
	}

	for _, sub := range subs {
		sourceRef := sub.SourceRef

		// lookup REST mapping
		sourceMapping, err := atm.getMapping(sourceRef)
		if err != nil {
			return mutated, reason, fmt.Errorf("failed to identify source resource mapping (%s): %w", sourceRef, err)
		}

		// Default source namespace to target namesapce, if namespace-scoped
		if sourceRef.Namespace == "" && sourceMapping.Scope.Name() == meta.RESTScopeNameNamespace {
			sourceRef.Namespace = targetRef.Namespace
		}

		// validate no self-references
		// Re-check to catch sources with implicit namespace.
		if targetRef.Equal(sub.SourceRef) {
			return mutated, reason, fmt.Errorf("invalid self-reference (%s)", sub.SourceRef)
		}

		// lookup source resource from cache or cluster
		sourceObj, err := atm.getObject(ctx, sourceMapping, sourceRef)
		if err != nil {
			return mutated, reason, fmt.Errorf("failed to get source resource (%s): %w", sourceRef, err)
		}

		klog.V(4).Infof("source resource (%s):\n%s", sourceRef, object.YamlStringer{O: sourceObj})

		// lookup target field in target resource
		targetValue, _, err := readFieldValue(obj, sub.TargetPath)
		if err != nil {
			return mutated, reason, fmt.Errorf("failed to read field (%s) from target resource (%s): %w", sub.TargetPath, targetRef, err)
		}

		// lookup source field in source resource
		sourceValue, found, err := readFieldValue(sourceObj, sub.SourcePath)
		if err != nil {
			return mutated, reason, fmt.Errorf("failed to read field (%s) from source resource (%s): %w", sub.SourcePath, sourceRef, err)
		}
		if !found {
			return mutated, reason, fmt.Errorf("source field (%s) not present in source resource (%s)", sub.SourcePath, sourceRef)
		}

		var newValue interface{}
		if sub.Token == "" {
			// token not specified, replace the entire target value with the source value
			newValue = sourceValue
		} else {
			// token specified, substitute token for source field value in target field value
			targetValueString, ok := targetValue.(string)
			if !ok {
				return mutated, reason, fmt.Errorf("token is specified, but target field value is %T, expected string", targetValue)
			}

			sourceValueString, err := valueToString(sourceValue)
			if err != nil {
				return mutated, reason, fmt.Errorf("failed to stringify source field value (%s): %w", targetRef, err)
			}

			// Substitute token for source field value, if present.
			// If not present, do nothing. This is common on updates.
			newValue = strings.ReplaceAll(targetValueString, sub.Token, sourceValueString)
		}

		klog.V(5).Infof("substitution: targetRef=(%s), sourceRef=(%s): sourceValue=(%v), token=(%s), oldTargetValue=(%v), newTargetValue=(%v)",
			targetRef, sourceRef, sourceValue, sub.Token, targetValue, newValue)

		// update target field in target resource
		err = writeFieldValue(obj, sub.TargetPath, newValue)
		if err != nil {
			return mutated, reason, fmt.Errorf("failed to set field in target resource (%s): %w", targetRef, err)
		}

		mutated = true
		reason = fmt.Sprintf("resource contained annotation: %s", mutation.Annotation)
	}

	if mutated {
		klog.V(4).Infof("mutated target resource (%s):\n%s", targetRef, object.YamlStringer{O: obj})
	}

	return mutated, reason, nil
}

func (atm *ApplyTimeMutator) getMapping(ref mutation.ResourceReference) (*meta.RESTMapping, error) {
	// lookup resource using group api version, if specified
	sourceGvk := ref.GroupVersionKind()
	var mapping *meta.RESTMapping
	var err error
	if sourceGvk.Version != "" {
		mapping, err = atm.Mapper.RESTMapping(sourceGvk.GroupKind(), sourceGvk.Version)
	} else {
		mapping, err = atm.Mapper.RESTMapping(sourceGvk.GroupKind())
	}
	if err != nil {
		return nil, err
	}
	return mapping, nil
}

// getObject returns a cached resource, if cached and cache exists, otherwise
// the resource is retrieved from the cluster.
func (atm *ApplyTimeMutator) getObject(ctx context.Context, mapping *meta.RESTMapping, ref mutation.ResourceReference) (*unstructured.Unstructured, error) {
	// validate source reference
	if ref.Name == "" {
		return nil, fmt.Errorf("invalid source reference: empty name")
	}
	if ref.Kind == "" {
		return nil, fmt.Errorf("invalid source reference: empty kind")
	}
	id := ref.ObjMetadata()

	// get resource from cache
	if atm.ResourceCache != nil {
		result := atm.ResourceCache.Get(id)
		// Use the cached version, if current/reconciled.
		// Otherwise, get it from the cluster.
		if result.Resource != nil && result.Status == status.CurrentStatus {
			return result.Resource, nil
		}
	}

	// get resource from cluster
	namespacedClient := atm.Client.Resource(mapping.Resource).Namespace(ref.Namespace)
	obj, err := namespacedClient.Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		// Skip NotFound so the cache gets updated.
		return nil, fmt.Errorf("failed to retrieve resource from cluster: %w", err)
	}

	// add resource to cache
	if atm.ResourceCache != nil {
		// If it's not cached or not current, update the cache.
		// This will add external resources to the cache,
		// but the user won't get status events for them.
		atm.ResourceCache.Put(id, computeStatus(obj))
	}

	if err != nil {
		// NotFound
		return nil, fmt.Errorf("resource not found: %w", err)
	}

	return obj, nil
}

// computeStatus compares the spec to the status and returns the result.
func computeStatus(obj *unstructured.Unstructured) cache.ResourceStatus {
	if obj == nil {
		return cache.ResourceStatus{
			Resource:      obj,
			Status:        status.NotFoundStatus,
			StatusMessage: "Resource not found",
		}
	}
	result, err := status.Compute(obj)
	if err != nil {
		if klog.V(3).Enabled() {
			ref := mutation.NewResourceReference(obj)
			klog.Info("failed to compute resource status (%s): %d", ref, err)
		}
		return cache.ResourceStatus{
			Resource: obj,
			Status:   status.UnknownStatus,
			//StatusMessage: fmt.Sprintf("Failed to compute status: %s", err),
		}
	}
	return cache.ResourceStatus{
		Resource:      obj,
		Status:        result.Status,
		StatusMessage: result.Message,
	}
}

func readFieldValue(obj *unstructured.Unstructured, path string) (interface{}, bool, error) {
	if path == "" {
		return nil, false, errors.New("empty path expression")
	}

	values, err := jsonpath.Get(obj.Object, path)
	if err != nil {
		return nil, false, err
	}
	if len(values) != 1 {
		return nil, false, fmt.Errorf("expected 1 match, but found %d)", len(values))
	}
	return values[0], true, nil
}

func writeFieldValue(obj *unstructured.Unstructured, path string, value interface{}) error {
	if path == "" {
		return errors.New("empty path expression")
	}

	found, err := jsonpath.Set(obj.Object, path, value)
	if err != nil {
		return err
	}
	if found != 1 {
		return fmt.Errorf("expected 1 match, but found %d)", found)
	}
	return nil
}

// valueToString converts an interface{} to a string, formatting as json for
// maps, lists. Designed to handle yaml/json/krm primitives.
func valueToString(value interface{}) (string, error) {
	var valueString string
	switch valueTyped := value.(type) {
	case string:
		valueString = valueTyped
	case int, int32, int64, float32, float64, bool:
		valueString = fmt.Sprintf("%v", valueTyped)
	default:
		jsonBytes, err := json.Marshal(valueTyped)
		if err != nil {
			return "", fmt.Errorf("failed to marshal value to json: %#v", value)
		}
		valueString = string(jsonBytes)
	}
	return valueString, nil
}
