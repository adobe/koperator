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

package kafkaclient

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

// newTimedKafka keeps protocol I/O in memory so synctest can advance the
// clock while the client and broker wait for requests, replies, or deadlines.
func newTimedKafka(t *testing.T, timeout int64, opts ...kfake.Opt) (*kfake.Cluster, *kafkaClient, kgo.Opt) {
	t.Helper()
	listeners := make(map[string]*pipeListener)
	listen := func(_, _ string) (net.Listener, error) {
		listener := &pipeListener{
			addr:     &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 10000 + len(listeners)},
			incoming: make(chan net.Conn),
			closed:   make(chan struct{}),
		}
		listeners[listener.addr.String()] = listener
		return listener, nil
	}
	dial := func(ctx context.Context, _, addr string) (net.Conn, error) {
		listener := listeners[addr]
		if listener == nil {
			return nil, &net.OpError{Op: "dial", Err: net.ErrClosed}
		}
		client, server := net.Pipe()
		select {
		case listener.incoming <- server:
			return client, nil
		case <-listener.closed:
			_ = client.Close()
			_ = server.Close()
			return nil, net.ErrClosed
		case <-ctx.Done():
			_ = client.Close()
			_ = server.Close()
			return nil, ctx.Err()
		}
	}
	cluster := kfake.MustCluster(append(opts, kfake.ListenFn(listen))...)
	t.Cleanup(cluster.Close)
	client := New(&KafkaConfig{BrokerURI: cluster.ListenAddrs()[0], OperationTimeout: timeout}).(*kafkaClient)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return cluster, client, kgo.Dialer(dial)
}

type pipeListener struct {
	addr     net.Addr
	incoming chan net.Conn
	closed   chan struct{}
	close    sync.Once
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case incoming := <-l.incoming:
		return incoming, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.close.Do(func() { close(l.closed) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return l.addr }
