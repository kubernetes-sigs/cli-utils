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

package prune_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/prune"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
)

func TestPruneEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	p, done, err := wiretest.InitializePrune(clik8s.ResourcePruneConfigs(nil), &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)
	r, err := p.Do()
	assert.NoError(t, err)
	assert.Equal(t, prune.Result{}, r)
}

func TestPrune(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := wiretest.InitializeKustomization()
	assert.NoError(t, err)
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)
	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()
	_, err = a.Do()
	assert.NoError(t, err)
	a.Resources, err = kp.GetConfig(fs[1])
	assert.NoError(t, err)
	_, err = a.Do()
	assert.NoError(t, err)

	pruneObject, err := kp.GetPruneConfig(fs[1])
	assert.NoError(t, err)
	p, donep, err := wiretest.InitializePrune(pruneObject, &object.Commit{}, buf)
	defer donep()
	assert.NoError(t, err)
	p.DynamicClient = a.DynamicClient
	pr, err := p.Do()
	assert.NoError(t, err)
	assert.Equal(t, len(pr.Resources), 1)
}
