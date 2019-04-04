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
	"k8s.io/apimachinery/pkg/runtime"
)

// ConfigProvider provides runtime.Objects for a path
type ConfigProvider interface {
	// IsSupported returns true if the ConfigProvider supports the given path
	IsSupported(path string) bool

	// GetConfig returns the Resource Config as runtime.Objects
	GetConfig(path string) []runtime.Object
}

type KustomizeProvider struct{}

func (p *KustomizeProvider) IsSupported(path string) bool {
	return false
}

func (p *KustomizeProvider) GetConfig(path string) []runtime.Object {
	return nil
}

type RawConfigFileProvider struct{}

func (p *RawConfigFileProvider) IsSupported(path string) bool {
	return false
}

func (p *RawConfigFileProvider) GetConfig(path string) []runtime.Object {
	return nil
}

type RawConfigHTTPProvider struct{}

func (p *RawConfigHTTPProvider) IsSupported(path string) bool {
	return false
}

func (p *RawConfigHTTPProvider) GetConfig(path string) []runtime.Object {
	return nil
}
