// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/yaml"
)

var codec = scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

// YamlStringer delays YAML marshalling for logging until String() is called.
type YamlStringer struct {
	O runtime.Object
}

// String marshals the wrapped object to a YAML string. If serializing errors,
// the error string will be returned instead. This is primarily for use with
// verbose logging.
func (ys YamlStringer) String() string {
	jsonBytes, err := runtime.Encode(unstructured.NewJSONFallbackEncoder(codec), ys.O)
	if err != nil {
		return fmt.Sprintf("<<failed to serialize as json: %s>>", err)
	}
	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return fmt.Sprintf("<<failed to convert from json to yaml: %s>>", err)
	}
	return string(yamlBytes)
}
