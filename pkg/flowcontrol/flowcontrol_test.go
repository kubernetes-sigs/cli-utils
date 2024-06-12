// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package flowcontrol

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	flowcontrolapi "k8s.io/api/flowcontrol/v1beta2"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestIsEnabled(t *testing.T) {
	testCases := []struct {
		name            string
		handler         func(*http.Request) *http.Response
		expectedEnabled bool
		expectedError   error
	}{
		{
			name: "header found",
			handler: func(req *http.Request) *http.Response {
				defer req.Body.Close()
				headers := http.Header{}
				headers.Add(flowcontrolapi.ResponseHeaderMatchedFlowSchemaUID, "unused-uuid")

				return &http.Response{
					StatusCode: 200,
					Header:     headers,
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}
			},
			expectedEnabled: true,
		},
		{
			name: "header not found",
			handler: func(req *http.Request) *http.Response {
				defer req.Body.Close()
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{},
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}
			},
			expectedEnabled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				assert.Equal(t, "/livez/ping", req.URL.Path)
				resp := tc.handler(req)
				defer resp.Body.Close()
				for k, vs := range resp.Header {
					w.Header().Del(k)
					for _, v := range vs {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(resp.StatusCode)
				_, err := io.Copy(w, resp.Body)
				assert.NoError(t, err)
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := &rest.Config{
				Host: server.URL,
			}

			enabled, err := IsEnabled(ctx, cfg)
			testutil.AssertEqual(t, tc.expectedError, err)
			assert.Equal(t, tc.expectedEnabled, enabled)
		})
	}
}
