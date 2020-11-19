// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
package error

type UnknwonTypeError struct {
	err error
}

func (e *UnknwonTypeError) Error() string {
	return e.err.Error()
}

func NewUnknwonTypeError(err error) *UnknwonTypeError {
	return &UnknwonTypeError{err: err}
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
