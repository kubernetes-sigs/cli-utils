// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
)

var testNamespace = "test-grouping-namespace"
var groupingObjName = "test-grouping-obj"
var pod1Name = "pod-1"
var pod2Name = "pod-2"
var pod3Name = "pod-3"

var testGroupingLabel = "test-app-label"

var groupingObj = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      groupingObjName,
			"namespace": testNamespace,
			"labels": map[string]interface{}{
				prune.GroupingLabel: testGroupingLabel,
			},
		},
	},
}

var pod1 = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      pod1Name,
			"namespace": testNamespace,
		},
	},
}

var pod1Info = &resource.Info{
	Namespace: testNamespace,
	Name:      pod1Name,
	Object:    &pod1,
}

var pod2 = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      pod2Name,
			"namespace": testNamespace,
		},
	},
}

var pod2Info = &resource.Info{
	Namespace: testNamespace,
	Name:      pod2Name,
	Object:    &pod2,
}

var pod3 = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      pod3Name,
			"namespace": testNamespace,
		},
	},
}

var pod3Info = &resource.Info{
	Namespace: testNamespace,
	Name:      pod3Name,
	Object:    &pod3,
}

func TestPrependGroupingObject(t *testing.T) {
	tests := []struct {
		infos []*resource.Info
	}{
		{
			infos: []*resource.Info{copyGroupingInfo()},
		},
		{
			infos: []*resource.Info{pod1Info, pod3Info, copyGroupingInfo()},
		},
		{
			infos: []*resource.Info{pod1Info, pod2Info, copyGroupingInfo(), pod3Info},
		},
	}

	for _, test := range tests {
		applyOptions := createApplyOptions(test.infos)
		f := prependGroupingObject(applyOptions)
		err := f()
		if err != nil {
			t.Errorf("Error running pre-processor callback: %s", err)
		}
		infos, _ := applyOptions.GetObjects()
		if len(test.infos) != len(infos) {
			t.Fatalf("Wrong number of objects after prepending grouping object")
		}
		groupingInfo := infos[0]
		if !prune.IsGroupingObject(groupingInfo.Object) {
			t.Fatalf("First object is not the grouping object")
		}
		inventory, _ := prune.RetrieveInventoryFromGroupingObj(infos)
		if len(inventory) != (len(infos) - 1) {
			t.Errorf("Wrong number of inventory items stored in grouping object")
		}
	}

}

// createApplyOptions is a helper function to assemble the ApplyOptions
// with the passed objects (infos).
func createApplyOptions(infos []*resource.Info) *apply.ApplyOptions {
	applyOptions := &apply.ApplyOptions{}
	applyOptions.SetObjects(infos)
	return applyOptions
}

func copyGroupingInfo() *resource.Info {
	groupingObjCopy := groupingObj.DeepCopy()
	var groupingInfo = &resource.Info{
		Namespace: testNamespace,
		Name:      groupingObjName,
		Object:    groupingObjCopy,
	}
	return groupingInfo
}
