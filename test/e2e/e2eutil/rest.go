// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2eutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/onsi/gomega"
)

const unknown = "unknown"

// UserAgent returns the a User-Agent for use with HTTP clients.
// The string corresponds to the current version of the binary being executed,
// using metadata from git and go.
func UserAgent(suffix string) string {
	return fmt.Sprintf("%s/%s (%s/%s) cli-utils/%s/%s",
		adjustCommand(os.Args[0]),
		adjustVersion(gitVersion()),
		runtime.GOOS,
		runtime.GOARCH,
		adjustCommit(gitCommit()),
		suffix)
}

// gitVersion returns the output from `git describe`
func gitVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "describe")

	// Ginkgo sets the working directory to the current test dir
	cwd, err := os.Getwd()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	cmd.Dir = cwd

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "STDERR: %s", errBuf.String())

	return strings.TrimSpace(outBuf.String())
}

// gitCommit returns the output from `git rev-parse HEAD`
func gitCommit() string {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")

	// Ginkgo sets the working directory to the current test dir
	cwd, err := os.Getwd()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	cmd.Dir = cwd

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "STDERR: %s", errBuf.String())

	return strings.TrimSpace(outBuf.String())
}

// adjustCommand returns the last component of the
// OS-specific command path for use in User-Agent.
func adjustCommand(p string) string {
	// Unlikely, but better than returning "".
	if len(p) == 0 {
		return unknown
	}
	return filepath.Base(p)
}

// adjustVersion strips "alpha", "beta", etc. from version in form
// major.minor.patch-[alpha|beta|etc].
func adjustVersion(v string) string {
	if len(v) == 0 {
		return unknown
	}
	seg := strings.SplitN(v, "-", 2)
	return seg[0]
}

// adjustCommit returns sufficient significant figures of the commit's git hash.
func adjustCommit(c string) string {
	if len(c) == 0 {
		return unknown
	}
	if len(c) > 7 {
		return c[:7]
	}
	return c
}
