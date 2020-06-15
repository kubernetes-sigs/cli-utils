// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
)

// ResetRESTMapperTask resets the provided RESTMapper.
type ResetRESTMapperTask struct {
	Mapper meta.RESTMapper
}

// Start creates a new goroutine that will unwrap the provided RESTMapper
// to get the underlying DeferredDiscoveryRESTMapper and then reset it. It
// will send a TaskResult on the taskChannel to signal that the task has
// been completed.
func (r *ResetRESTMapperTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		ddRESTMapper, err := extractDeferredDiscoveryRESTMapper(r.Mapper)
		if err != nil {
			r.sendTaskResult(taskContext, err)
			return
		}
		ddRESTMapper.Reset()
		r.sendTaskResult(taskContext, nil)
	}()
}

// extractDeferredDiscoveryRESTMapper unwraps the provided RESTMapper
// interface to get access to the underlying DeferredDiscoveryRESTMapper
// that can be reset.
func extractDeferredDiscoveryRESTMapper(mapper meta.RESTMapper) (*restmapper.DeferredDiscoveryRESTMapper,
	error) {
	val := reflect.ValueOf(mapper)
	if val.Type().Kind() != reflect.Struct {
		return nil, fmt.Errorf("unexpected RESTMapper type: %s", val.Type().String())
	}
	fv := val.FieldByName("RESTMapper")
	ddRESTMapper, ok := fv.Interface().(*restmapper.DeferredDiscoveryRESTMapper)
	if !ok {
		return nil, fmt.Errorf("unexpected RESTMapper type")
	}
	return ddRESTMapper, nil
}

func (r *ResetRESTMapperTask) sendTaskResult(taskContext *taskrunner.TaskContext, err error) {
	taskContext.TaskChannel() <- taskrunner.TaskResult{
		Err: err,
	}
}

// ClearTimeout doesn't do anything as ResetRESTMapperTask doesn't support
// timeouts.
func (r *ResetRESTMapperTask) ClearTimeout() {}
