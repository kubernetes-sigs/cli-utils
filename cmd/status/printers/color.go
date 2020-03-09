// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package printers

const (
	// RESET is the escape sequence for unsetting any previous commands.
	RESET = 0
	// ESC is the escape sequence used to send ANSI commands in the terminal.
	ESC          = 27
	RED    color = 31
	GREEN  color = 32
	YELLOW color = 33
)

// color is a type that captures the ANSI code for colors on the
// terminal.
type color int
