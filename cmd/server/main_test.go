package main

import (
	"net"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freePort returns a port that was free a moment ago. Another process can
// grab it before run binds it, but that is rare enough for a test.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	require.NoError(t, l.Close())
	return port
}

func TestRunStartupFailure(t *testing.T) {
	busy, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	t.Cleanup(func() { busy.Close() })

	httpPort := freePort(t)
	grpcPort := freePort(t)
	metricsPort := strconv.Itoa(busy.Addr().(*net.TCPAddr).Port)

	t.Setenv("PORT", httpPort)
	t.Setenv("GRPC_PORT", grpcPort)
	t.Setenv("METRICS_PORT", metricsPort)
	t.Setenv("PROVIDER", "ollama")
	t.Setenv("OLLAMA_ENDPOINT_URL", "http://127.0.0.1:1/api/embed")
	t.Setenv("OLLAMA_MODEL", "test-model")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("UPSTREAM_TIMEOUT", "1s")
	// Nothing routes traffic to a server that never started, so a startup
	// failure must skip the drain delay; if it didn't, run would outlast the
	// 10s wait below.
	t.Setenv("DRAIN_DELAY", "20s")
	t.Setenv("SHUTDOWN_TIMEOUT", "25s")

	done := make(chan error, 1)
	go func() { done <- run() }()

	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after the metrics server failed to start")
	}

	require.Error(t, runErr)
	assert.ErrorIs(t, runErr, syscall.EADDRINUSE)
	assert.ErrorContains(t, runErr, "metrics server")

	// The other servers must have been shut down, not left running. They may
	// still be starting when run returns, so keep probing for a while.
	for name, port := range map[string]string{"http": httpPort, "grpc": grpcPort} {
		assert.Never(t, func() bool {
			conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), 50*time.Millisecond)
			if err != nil {
				return false
			}
			conn.Close()
			return true
		}, 300*time.Millisecond, 20*time.Millisecond, "%s server still accepts connections after run returned", name)
	}
}
