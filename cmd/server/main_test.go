package main

import (
	"net"
	"net/http"
	"os"
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

// get returns the status code of a GET to url, or 0 if the request failed.
func get(url string) int {
	resp, err := http.Get(url)
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return resp.StatusCode
}

// In a sidecar the API ports are bound to localhost, where the kubelet cannot
// reach them, so the probes must be served on the admin port.
func TestRunServesProbesOnAdminPort(t *testing.T) {
	httpPort := freePort(t)
	adminPort := freePort(t)

	t.Setenv("PORT", httpPort)
	t.Setenv("GRPC_PORT", freePort(t))
	t.Setenv("METRICS_PORT", adminPort)
	t.Setenv("LISTEN_HOST", "127.0.0.1")
	t.Setenv("PROVIDER", "ollama")
	t.Setenv("OLLAMA_ENDPOINT_URL", "http://127.0.0.1:1/api/embed")
	t.Setenv("OLLAMA_MODEL", "test-model")
	t.Setenv("API_KEY", "")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("UPSTREAM_TIMEOUT", "1s")
	t.Setenv("DRAIN_DELAY", "1s")
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")

	done := make(chan error, 1)
	go func() { done <- run() }()

	admin := "http://127.0.0.1:" + adminPort
	require.Eventually(t, func() bool { return get(admin+"/readyz") == http.StatusOK },
		5*time.Second, 20*time.Millisecond, "admin port never became ready")

	assert.Equal(t, http.StatusOK, get(admin+"/healthz"))
	assert.Equal(t, http.StatusOK, get(admin+"/metrics"))
	assert.Equal(t, http.StatusNotFound, get("http://127.0.0.1:"+httpPort+"/healthz"),
		"probes belong on the admin port only")

	// Reaching /readyz means run is past signal.NotifyContext, so this
	// SIGTERM goes to run and not to the test process.
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	// During the drain delay readiness fails, but liveness must not, or the
	// kubelet would restart the container mid-shutdown.
	require.Eventually(t, func() bool { return get(admin+"/readyz") == http.StatusServiceUnavailable },
		900*time.Millisecond, 10*time.Millisecond, "readiness did not fail during the drain")
	assert.Equal(t, http.StatusOK, get(admin+"/healthz"))

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after SIGTERM")
	}
}
