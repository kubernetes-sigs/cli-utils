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

package wireconfig

import (
	"github.com/google/wire"

	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/resourceconfig"
	"sigs.k8s.io/kustomize/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/k8sdeps/kv/plugin"
	ktransformer "sigs.k8s.io/kustomize/k8sdeps/transformer"
	"sigs.k8s.io/kustomize/pkg/fs"
	"sigs.k8s.io/kustomize/pkg/ifc/transformer"
	"sigs.k8s.io/kustomize/pkg/resmap"
	"sigs.k8s.io/kustomize/pkg/resource"
	"sigs.k8s.io/kustomize/pkg/types"

	// for connecting to various types of hosted clusters
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// ConfigProviderSet defines dependencies for initializing ConfigProvider
var ConfigProviderSet = wire.NewSet(
	NewPluginConfig,
	NewResMapFactory,
	NewTransformerFactory,
	NewFileSystem,
	NewKustomizeProvider,
	wire.Bind(new(resourceconfig.ConfigProvider), new(*resourceconfig.KustomizeProvider)),
	NewResourceConfig,
	NewResourcePruneConfig,
)

// NewPluginConfig returns a new PluginConfig
func NewPluginConfig() *types.PluginConfig {
	pc := plugin.DefaultPluginConfig()
	pc.GoEnabled = true
	return pc
}

// NewResMapFactory returns a rew ResMap Factory
func NewResMapFactory(pc *types.PluginConfig) *resmap.Factory {
	uf := kunstruct.NewKunstructuredFactoryWithGeneratorArgs(
		&types.GeneratorMetaArgs{
			PluginConfig: pc,
		})
	return resmap.NewFactory(resource.NewFactory(uf))
}

// NewTransformerFactory returns a new transformer factory
func NewTransformerFactory() transformer.Factory {
	return ktransformer.NewFactoryImpl()
}

// NewFileSystem returns a new filesystem
func NewFileSystem() fs.FileSystem {
	return fs.MakeRealFS()
}

// NewKustomizeProvider returns a new KustomizeProvider
func NewKustomizeProvider(rf *resmap.Factory,
	fSys fs.FileSystem, tf transformer.Factory,
	pc *types.PluginConfig) *resourceconfig.KustomizeProvider {
	return &resourceconfig.KustomizeProvider{
		RF: rf,
		TF: tf,
		FS: fSys,
		PC: pc,
	}
}

// NewResourceConfig provides ResourceConfigs read from the ResourceConfigPath and FileSystem.
func NewResourceConfig(rcp clik8s.ResourceConfigPath,
	cp resourceconfig.ConfigProvider) (clik8s.ResourceConfigs, error) {
	p := string(rcp)

	if cp.IsSupported(p) {
		return cp.GetConfig(p)
	}
	return nil, nil
}

// NewResourcePruneConfig provides ResourceConfigs read from the ResourceConfigPath and FileSystem.
func NewResourcePruneConfig(rcp clik8s.ResourceConfigPath,
	cp resourceconfig.ConfigProvider) (clik8s.ResourcePruneConfigs, error) {
	p := string(rcp)

	if cp.IsSupported(p) {
		return cp.GetPruneConfig(p)
	}

	return nil, nil
}
