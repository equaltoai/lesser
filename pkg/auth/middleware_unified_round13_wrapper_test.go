package auth

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestUnifiedAuthMiddleware_OptionalWrapperConstructors(t *testing.T) {
	wrappers := map[string]func(common.OAuthServiceInterface, *zap.Logger) apptheory.Middleware{
		"api":        CreateAPIAuthMiddleware,
		"graphql":    CreateGraphQLAuthMiddleware,
		"federation": CreateFederationAuthMiddleware,
	}

	for serviceName, build := range wrappers {
		t.Run(serviceName+" logs wrapper service name on invalid header", func(t *testing.T) {
			core, observed := observer.New(zap.WarnLevel)
			logger := zap.New(core)
			ctx := newTestContext("GET", "/"+serviceName, withHeaders(map[string]string{
				"Authorization": "Bearer invalid-token",
			}))

			nextCalled := false
			resp, err := build(oauthServiceStub{err: ErrInvalidToken}, logger)(func(ctx *apptheory.Context) (*apptheory.Response, error) {
				nextCalled = true
				assert.False(t, IsAuthenticated(ctx))
				return &apptheory.Response{Status: 204}, nil
			})(ctx)
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.True(t, nextCalled)

			entries := observed.FilterMessage("optional authentication failed - header present but validation failed").AllUntimed()
			require.Len(t, entries, 1)
			assert.Equal(t, serviceName, entries[0].ContextMap()["service"])
		})
	}
}
