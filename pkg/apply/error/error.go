// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
package error

import "sigs.k8s.io/cli-utils/pkg/apply/event"

type UnknownTypeError struct {
	err error
}

func (e *UnknownTypeError) Error() string {
	return e.err.Error()
}

func NewUnknownTypeError(err error) *UnknownTypeError {
	return &UnknownTypeError{err: err}
}

type ApplyRunError struct {
	err error
}

func (e *ApplyRunError) Error() string {
	return e.err.Error()
}

func NewApplyRunError(err error) *ApplyRunError {
	return &ApplyRunError{err: err}
}

type InitializeApplyOptionError struct {
	err error
}

func (e *InitializeApplyOptionError) Error() string {
	return e.err.Error()
}

func NewInitializeApplyOptionError(err error) *InitializeApplyOptionError {
	return &InitializeApplyOptionError{err: err}
}

// HandleError sends a ErrorType event onto eventChannel.
func HandleError(eventChannel chan event.Event, err error) {
	eventChannel <- event.Event{
		Type: event.ErrorType,
		ErrorEvent: event.ErrorEvent{
			Err: err,
		},
	}
}
