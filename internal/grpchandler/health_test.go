package grpchandler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

const embeddingServiceName = "embeddings.v1.EmbeddingService"

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name       string
		service    string
		wantStatus healthpb.HealthCheckResponse_ServingStatus
		wantCode   codes.Code
	}{
		{
			name:       "overall server",
			service:    "",
			wantStatus: healthpb.HealthCheckResponse_SERVING,
		},
		{
			name:       "embedding service",
			service:    embeddingServiceName,
			wantStatus: healthpb.HealthCheckResponse_SERVING,
		},
		{
			name:     "unknown service",
			service:  "does.not.Exist",
			wantCode: codes.NotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _, _ := startTestServerConn(t, &fakeEmbeddingService{})
			client := healthpb.NewHealthClient(conn)

			resp, err := client.Check(t.Context(), &healthpb.HealthCheckRequest{Service: tt.service})

			if tt.wantCode != codes.OK {
				require.Error(t, err)
				assert.Equal(t, tt.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, resp.GetStatus())
		})
	}
}

// Shutdown is what main calls on SIGTERM, so load balancers stop sending
// traffic while in-flight requests drain.
func TestHealthShutdownReportsNotServing(t *testing.T) {
	conn, hs, _ := startTestServerConn(t, &fakeEmbeddingService{})
	client := healthpb.NewHealthClient(conn)

	hs.Shutdown()

	for _, service := range []string{"", embeddingServiceName} {
		resp, err := client.Check(t.Context(), &healthpb.HealthCheckRequest{Service: service})
		require.NoError(t, err, "service %q", service)
		assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, resp.GetStatus(), "service %q", service)
	}
}
