package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
)

func TestWrapHandler(t *testing.T) {
	t.Run("text response", func(t *testing.T) {
		handler := wrapHandler(func(_ context.Context, _ events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: 201,
				Headers:    map[string]string{"X-Test": "ok"},
				Body:       "hello",
			}, nil
		})

		req := lift.NewRequest(&adapters.Request{
			Method: "GET",
			Path:   "/test",
		})
		ctx := lift.NewContext(context.Background(), req)

		require.NoError(t, handler.Handle(ctx))
		require.Equal(t, 201, ctx.Response.StatusCode)
		require.Equal(t, "ok", ctx.Response.Headers["X-Test"])
		require.Equal(t, "hello", ctx.Response.Body)
	})

	t.Run("base64 response", func(t *testing.T) {
		handler := wrapHandler(func(_ context.Context, _ events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
			return &events.APIGatewayV2HTTPResponse{
				StatusCode:      200,
				IsBase64Encoded: true,
				Body:            "raw",
			}, nil
		})

		req := lift.NewRequest(&adapters.Request{
			Method: "GET",
			Path:   "/binary",
		})
		ctx := lift.NewContext(context.Background(), req)

		require.NoError(t, handler.Handle(ctx))
		require.True(t, ctx.Response.IsBase64Encoded)
		require.Equal(t, "cmF3", ctx.Response.Body)
	})
}

func TestWrapHandlerWithParam(t *testing.T) {
	handler := wrapHandlerWithParam(func(_ context.Context, req events.APIGatewayV2HTTPRequest, param string) (*events.APIGatewayV2HTTPResponse, error) {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 200,
			Body:       req.PathParameters["id"] + ":" + param,
		}, nil
	}, "id")

	req := lift.NewRequest(&adapters.Request{
		Method: "GET",
		Path:   "/test/123",
	})
	ctx := lift.NewContext(context.Background(), req)
	ctx.SetParam("id", "123")

	require.NoError(t, handler.Handle(ctx))
	require.Equal(t, 200, ctx.Response.StatusCode)
	require.Equal(t, "123:123", ctx.Response.Body)
}
