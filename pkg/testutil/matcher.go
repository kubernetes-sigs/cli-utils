// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega/format"
)

// Equal returns a matcher for use with Gomega that uses go-cmp's cmp.Equal to
// compare and cmp.Diff to show the difference, if there is one.
//
// Example Usage:
// Expect(receivedEvents).To(testutil.Equal(expectedEvents))
func Equal(expected interface{}) *EqualMatcher {
	return DefaultAsserter.EqualMatcher(expected)
}

type EqualMatcher struct {
	Expected interface{}
	Options  cmp.Options

	explanation error
}

func (cm *EqualMatcher) Match(actual interface{}) (bool, error) {
	match := cmp.Equal(cm.Expected, actual, cm.Options...)
	if !match {
		cm.explanation = errors.New(cmp.Diff(cm.Expected, actual, cm.Options...))
	}
	return match, nil
}

func (cm *EqualMatcher) FailureMessage(actual interface{}) string {
	return "\n" + format.Message(actual, "to deeply equal", cm.Expected) +
		"\nDiff (- Expected, + Actual):\n" + indent(cm.explanation.Error(), 1)
}

func (cm *EqualMatcher) NegatedFailureMessage(actual interface{}) string {
	return "\n" + format.Message(actual, "not to deeply equal", cm.Expected) +
		"\nDiff (- Expected, + Actual):\n" + indent(cm.explanation.Error(), 1)
}

func indent(in string, indentation uint) string {
	indent := strings.Repeat(format.Indent, int(indentation))
	lines := strings.Split(in, "\n")
	return indent + strings.Join(lines, fmt.Sprintf("\n%s", indent))
}

// EqualErrorType returns an error with an Is(error)bool function that matches
// any error with the same type as the supplied error.
//
// Use with testutil.Equal to handle error comparisons.
func EqualErrorType(err error) error {
	return equalErrorType{
		err: err,
	}
}

type equalErrorType struct {
	err error
}

func (e equalErrorType) Error() string {
	return "EqualErrorType"
}

func (e equalErrorType) Is(err error) bool {
	if err == nil {
		return false
	}
	return reflect.TypeOf(e.err) == reflect.TypeOf(err)
}

func (e equalErrorType) Unwrap() error {
	return e.err
}

// EqualErrorString returns an error with an Is(error)bool function that matches
// any error with the same Error() as the supplied string value.
//
// Use with testutil.Equal to handle error comparisons.
func EqualErrorString(err string) error {
	return equalErrorString{
		err: err,
	}
}

// equalError is an error that matches any non-nil error of the specified type.
type equalErrorString struct {
	err string
}

func (e equalErrorString) Error() string {
	return e.err
}

func (e equalErrorString) Is(err error) bool {
	if err == nil {
		return false
	}
	return e.err == err.Error()
}
