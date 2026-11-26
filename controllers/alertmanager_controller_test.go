// Copyright 2026 Adobe. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAlertListenerReportsBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	err = (AController{}).start(t.Context(), listener.Addr().String())
	require.ErrorIs(t, err, syscall.EADDRINUSE)
}

func TestAlertListenerStopsOnCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- (AController{}).start(ctx, addr) }()

	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	require.Eventually(t, func() bool {
		resp, err := client.Get("http://" + addr + "/")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusMethodNotAllowed
	}, 2*time.Second, 10*time.Millisecond, "alert server did not start")

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("alert server did not stop after cancellation")
	}
	listener, err = net.Listen("tcp", addr)
	require.NoError(t, err, "alert port stayed bound after cancellation")
	defer listener.Close()
}
