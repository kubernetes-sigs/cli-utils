// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// YamlStringer delays YAML marshalling for logging until String() is called.
type YamlStringer struct {
	O interface{}
}

// String marshals the wrapped object to a YAML string. If serializing errors,
// the error string will be returned instead. This is primarily for use with
// verbose logging.
func (ys YamlStringer) String() string {
	v := ys.O
	if obj, ok := v.(*unstructured.Unstructured); ok {
		v = obj.UnstructuredContent()
	}
	yamlBytes, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<<failed to serialize as yaml: %s>>", err)
	}
	return string(yamlBytes)
}
