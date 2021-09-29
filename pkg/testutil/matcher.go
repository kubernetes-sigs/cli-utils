// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package testutil

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/gomega/format"
)

var EqualOptions = []cmp.Option{
	cmpopts.EquateErrors(),
}

// Equal returns a matcher for use with Gomega that uses go-cmp's cmp.Equal to
// compare and cmp.Diff to show the difference, if there is one.
//
// Example Usage:
// Expect(receivedEvents).To(testutil.Equal(expectedEvents))
func Equal(expected interface{}) *cmpMatcher {
	return &cmpMatcher{expected: expected}
}

type cmpMatcher struct {
	expected    interface{}
	explanation error
}

func (cm *cmpMatcher) Match(actual interface{}) (bool, error) {
	match := cmp.Equal(actual, cm.expected, EqualOptions...)
	if !match {
		cm.explanation = errors.New(cmp.Diff(actual, cm.expected, EqualOptions...))
	}
	return match, nil
}

func (cm *cmpMatcher) FailureMessage(actual interface{}) string {
	return format.Message(actual, "to deeply equal", cm.expected) +
		"\nDiff:\n" + indent(cm.explanation.Error(), 1)
}

func (cm *cmpMatcher) NegatedFailureMessage(actual interface{}) string {
	return format.Message(actual, "not to deeply equal", cm.expected) +
		"\nDiff:\n" + indent(cm.explanation.Error(), 1)
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
func EqualErrorType(err error) equalErrorType {
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
func EqualErrorString(err string) equalErrorString {
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

// AssertEqual fails the test if the actual value does not deeply equal the
// expected value. Prints a diff on failure.
func AssertEqual(t *testing.T, actual, expected interface{}) {
	matcher := Equal(expected)
	match, err := matcher.Match(actual)
	if err != nil {
		t.Errorf("errored testing equality: %s", err)
	}
	if !match {
		t.Error(matcher.FailureMessage(actual))
	}
}

// AssertNotEqual fails the test if the actual value deeply equals the
// expected value. Prints a diff on failure.
func AssertNotEqual(t *testing.T, actual, expected interface{}) {
	matcher := Equal(expected)
	match, err := matcher.Match(actual)
	if err != nil {
		t.Errorf("errored testing equality: %s", err)
	}
	if match {
		t.Error(matcher.NegatedFailureMessage(actual))
	}
}
