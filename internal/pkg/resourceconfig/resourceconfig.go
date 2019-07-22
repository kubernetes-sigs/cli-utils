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

package resourceconfig

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sigs.k8s.io/kustomize/k8sdeps/validator"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/kustomize/pkg/fs"
	"sigs.k8s.io/kustomize/pkg/ifc"
	"sigs.k8s.io/kustomize/pkg/ifc/transformer"
	"sigs.k8s.io/kustomize/pkg/loader"
	"sigs.k8s.io/kustomize/pkg/plugins"
	"sigs.k8s.io/kustomize/pkg/resmap"
	"sigs.k8s.io/kustomize/pkg/target"
	"sigs.k8s.io/kustomize/pkg/types"
	"sigs.k8s.io/yaml"
)

// ConfigProvider provides runtime.Objects for a path
type ConfigProvider interface {
	// IsSupported returns true if the ConfigProvider supports the given path
	IsSupported(path string) bool

	// GetConfig returns the Resource Config as runtime.Objects
	GetConfig(path string) ([]*unstructured.Unstructured, error)
}

var _ ConfigProvider = &KustomizeProvider{}
var _ ConfigProvider = &RawConfigFileProvider{}
var _ ConfigProvider = &RawConfigHTTPProvider{}

// KustomizeProvider provides configs from Kusotmize targets
type KustomizeProvider struct {
	RF *resmap.Factory
	TF transformer.Factory
	FS fs.FileSystem
	PC *types.PluginConfig
}

func (p *KustomizeProvider) getKustTarget(path string) (ifc.Loader, *target.KustTarget, error) {
	v := validator.NewKustValidator()
	ldr, err := loader.NewLoader(loader.RestrictionRootOnly, v, path, p.FS)
	if err != nil {
		return ldr, nil, err
	}
	kt, err := target.NewKustTarget(ldr, p.RF, p.TF, plugins.NewLoader(p.PC, p.RF))
	return ldr, kt, err
}

// IsSupported checks if the path is supported by KustomizeProvider
func (p *KustomizeProvider) IsSupported(path string) bool {
	ldr, _, err := p.getKustTarget(path)
	defer func() {
		if err := ldr.Cleanup(); err != nil {
			log.Fatal("failed to clean up the loader")
		}
	}()

	if err != nil {
		return false
	}
	return true
}

// GetConfig returns the resource configs
func (p *KustomizeProvider) GetConfig(path string) ([]*unstructured.Unstructured, error) {
	ldr, kt, err := p.getKustTarget(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := ldr.Cleanup(); err != nil {
			log.Fatal("failed to clean up the loader")
		}
	}()

	rm, err := kt.MakeCustomizedResMap()
	if err != nil {
		return nil, err
	}
	var results []*unstructured.Unstructured
	for _, r := range rm.Resources() {
		results = append(results, &unstructured.Unstructured{Object: r.Kunstructured.Map()})
	}
	return results, nil
}

// RawConfigFileProvider provides configs from raw K8s resources
type RawConfigFileProvider struct{}

// IsSupported checks if a path is a raw K8s configuration file
func (p *RawConfigFileProvider) IsSupported(path string) bool {
	// Don't allow running on kustomization.yaml, prevents weird things like globbing
	if filepath.Base(path) == "kustomization.yaml" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

// isYamlFile checks if the input file path has
// either .yaml or .yml extension
func isYamlFile(path string) bool {
	ext := filepath.Ext(path)
	if ext == ".yaml" || ext == ".yml" {
		return true
	}
	return false
}

// hasAPIVersionKind checks that the input bytes
// contains both apiVersion and kind
func hasApiVersionKind(content []byte) bool {
	if bytes.Contains(content, []byte("apiVersion:")) &&
		bytes.Contains(content, []byte("kind:")) {
		return true
	}
	return false
}

// GetConfig returns the resource configs
// from a directory or a file containing raw Kubernetes resource configurations
func (p *RawConfigFileProvider) GetConfig(root string) ([]*unstructured.Unstructured, error) {
	var values clik8s.ResourceConfigs

	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		if !isYamlFile(path) {
			return nil
		}
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		if !hasApiVersionKind(b) {
			return nil
		}

		objs := strings.Split(string(b), "---")
		for _, o := range objs {
			body := map[string]interface{}{}

			if err := yaml.Unmarshal([]byte(o), &body); err != nil {
				return err
			}
			values = append(values, &unstructured.Unstructured{Object: body})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return values, nil
}

// RawConfigHTTPProvider provides configs from HTTP urls
// TODO: implement RawConfigHTTPProvider
type RawConfigHTTPProvider struct{}

// IsSupported returns if the path points to a HTTP url target
func (p *RawConfigHTTPProvider) IsSupported(path string) bool {
	return false
}

// GetConfig returns the resource configs
func (p *RawConfigHTTPProvider) GetConfig(path string) ([]*unstructured.Unstructured, error) {
	return nil, nil
}
