// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// MultiValidationError captures validation errors for multiple resources.
type MultiValidationError struct {
	Errors []*ValidationError
}

func (ae MultiValidationError) Error() string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%d resources failed validation\n", len(ae.Errors))
	for _, e := range ae.Errors {
		b.WriteString(e.Error())
	}
	return b.String()
}

// ValidationError captures errors resulting from validation of a resources.
type ValidationError struct {
	GroupVersionKind schema.GroupVersionKind
	Name             string
	Namespace        string
	FieldErrors      field.ErrorList
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Resource: %q, Name: %q, Namespace: %q\n",
		e.GroupVersionKind.String(), e.Name, e.Namespace))
	b.WriteString(e.FieldErrors.ToAggregate().Error())
	return b.String()
}

// Validator contains functionality for validating a set of resources prior
// to being used by the Apply functionality. This imposes some constraint not
// always required, such as namespaced resources must have the namespace set.
type Validator struct {
	Mapper meta.RESTMapper
}

// Validate validates the provided resources. A RESTMapper will be used
// to fetch type information from the live cluster.
func (v *Validator) Validate(resources []*unstructured.Unstructured) error {
	crds := findCRDs(resources)
	var errs []*ValidationError
	for _, r := range resources {
		var errList field.ErrorList
		if err := v.validateName(r); err != nil {
			if fieldErr, ok := isFieldError(err); ok {
				errList = append(errList, fieldErr)
			} else {
				return err
			}
		}
		if err := v.validateNamespace(r, crds); err != nil {
			if fieldErr, ok := isFieldError(err); ok {
				errList = append(errList, fieldErr)
			} else {
				return err
			}
		}
		if len(errList) > 0 {
			errs = append(errs, &ValidationError{
				GroupVersionKind: r.GroupVersionKind(),
				Name:             r.GetName(),
				Namespace:        r.GetNamespace(),
				FieldErrors:      errList,
			})
		}
	}

	if len(errs) > 0 {
		return &MultiValidationError{
			Errors: errs,
		}
	}
	return nil
}

// isFieldError checks if an error is of type *field.Error. If so,
// a reference to an error of that type is returned.
func isFieldError(err error) (*field.Error, bool) {
	var fieldErr *field.Error
	if errors.As(err, &fieldErr) {
		return fieldErr, true
	}
	return nil, false
}

// findCRDs looks through the provided resources and returns a slice with
// the resources that are CRDs.
func findCRDs(us []*unstructured.Unstructured) []*unstructured.Unstructured {
	var crds []*unstructured.Unstructured
	for _, u := range us {
		if IsCRD(u) {
			crds = append(crds, u)
		}
	}
	return crds
}

// validateName validates the value of the name field of the resource.
func (v *Validator) validateName(u *unstructured.Unstructured) error {
	if u.GetName() == "" {
		return field.Required(field.NewPath("metadata", "name"), "name is required")
	}
	return nil
}

// validateNamespace validates the value of the namespace field of the resource.
func (v *Validator) validateNamespace(u *unstructured.Unstructured, crds []*unstructured.Unstructured) error {
	scope, err := LookupResourceScope(u, crds, v.Mapper)
	if err != nil {
		return err
	}

	ns := u.GetNamespace()
	if scope == meta.RESTScopeNamespace && ns == "" {
		return field.Required(field.NewPath("metadata", "namespace"), "namespace is required")
	}
	if scope == meta.RESTScopeRoot && ns != "" {
		return field.Invalid(field.NewPath("metadata", "namespace"), ns, "namespace must be empty")
	}
	return nil
}
