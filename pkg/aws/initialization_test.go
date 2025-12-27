package awsinit

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInitializeServices_RequiresLogger(t *testing.T) {
	services, err := InitializeServices(context.Background(), ServiceConfig{}, nil)
	require.Nil(t, services)
	require.Equal(t, ErrLoggerRequired, err)
}

func TestServiceConfigPresets(t *testing.T) {
	api := APIServiceConfig()
	require.True(t, api.RequiresDynamoDB)
	require.True(t, api.RequiresS3)
	require.True(t, api.RequiresCloudWatch)
	require.False(t, api.RequiresSQS)
	require.Equal(t, "api", api.ServiceName)

	processor := ProcessorServiceConfig()
	require.True(t, processor.RequiresDynamoDB)
	require.True(t, processor.RequiresSQS)
	require.False(t, processor.RequiresS3)
	require.Equal(t, "processor", processor.ServiceName)

	basic := BasicServiceConfig()
	require.True(t, basic.RequiresDynamoDB)
	require.False(t, basic.RequiresS3)
	require.False(t, basic.RequiresCloudWatch)
	require.Equal(t, "basic", basic.ServiceName)
}

func TestCreateHTTPClient_SetsTimeoutAndTransport(t *testing.T) {
	client := createHTTPClient(17 * time.Second)
	require.Equal(t, 17*time.Second, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Equal(t, 100, transport.MaxIdleConns)
	require.Equal(t, 10, transport.MaxIdleConnsPerHost)
	require.Equal(t, 90*time.Second, transport.IdleConnTimeout)
}
