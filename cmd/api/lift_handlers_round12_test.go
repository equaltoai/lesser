package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
)

func round12NewLiftContext(method, path string, headers, query map[string]string, body []byte) *lift.Context {
	req := lift.NewRequest(&adapters.Request{
		Method:      method,
		Path:        path,
		Headers:     headers,
		QueryParams: query,
		Body:        body,
	})
	return lift.NewContext(context.Background(), req)
}

func TestWrapHandlerRound12(t *testing.T) {
	t.Run("wrapHandler maps response fields", func(t *testing.T) {
		h := wrapHandler(func(context.Context, events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: 201,
				Headers:    map[string]string{"X-Test": "1"},
				Body:       "hello",
			}, nil
		})

		ctx := round12NewLiftContext("GET", "/test", map[string]string{"Host": "example.com"}, map[string]string{"q": "1"}, []byte("body"))
		require.NoError(t, h.Handle(ctx))
		require.Equal(t, 201, ctx.Response.StatusCode)
		require.Equal(t, "1", ctx.Response.Headers["X-Test"])
		require.Equal(t, "hello", ctx.Response.Body)
	})

	t.Run("wrapHandler passes through handler errors", func(t *testing.T) {
		h := wrapHandler(func(context.Context, events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
			return nil, errors.New("boom")
		})
		ctx := round12NewLiftContext("GET", "/test", nil, nil, nil)
		require.Error(t, h.Handle(ctx))
	})

	t.Run("wrapHandler handles base64 responses", func(t *testing.T) {
		h := wrapHandler(func(context.Context, events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
			return &events.APIGatewayV2HTTPResponse{
				StatusCode:      200,
				IsBase64Encoded: true,
				Body:            "abc",
			}, nil
		})
		ctx := round12NewLiftContext("GET", "/test", nil, nil, nil)
		require.NoError(t, h.Handle(ctx))
		require.True(t, ctx.Response.IsBase64Encoded)
		require.Equal(t, "YWJj", ctx.Response.Body)
	})
}

func TestWrapHandlerWithParamRound12(t *testing.T) {
	t.Run("wrapHandlerWithParam maps path parameter", func(t *testing.T) {
		h := wrapHandlerWithParam(func(_ context.Context, _ events.APIGatewayV2HTTPRequest, param string) (*events.APIGatewayV2HTTPResponse, error) {
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: 200,
				Body:       param,
			}, nil
		}, "id")

		ctx := round12NewLiftContext("GET", "/items/123", nil, nil, nil)
		ctx.SetParam("id", "123")
		require.NoError(t, h.Handle(ctx))
		require.Equal(t, 200, ctx.Response.StatusCode)
		require.Equal(t, "123", ctx.Response.Body)
	})

	t.Run("wrapHandlerWithParam passes through handler errors", func(t *testing.T) {
		h := wrapHandlerWithParam(func(context.Context, events.APIGatewayV2HTTPRequest, string) (*events.APIGatewayV2HTTPResponse, error) {
			return nil, errors.New("boom")
		}, "id")
		ctx := round12NewLiftContext("GET", "/items/123", nil, nil, nil)
		ctx.SetParam("id", "123")
		require.Error(t, h.Handle(ctx))
	})

	t.Run("wrapHandlerWithParam handles base64 responses", func(t *testing.T) {
		h := wrapHandlerWithParam(func(context.Context, events.APIGatewayV2HTTPRequest, string) (*events.APIGatewayV2HTTPResponse, error) {
			return &events.APIGatewayV2HTTPResponse{
				StatusCode:      200,
				IsBase64Encoded: true,
				Body:            "abc",
			}, nil
		}, "id")
		ctx := round12NewLiftContext("GET", "/items/123", nil, nil, nil)
		ctx.SetParam("id", "123")
		require.NoError(t, h.Handle(ctx))
		require.True(t, ctx.Response.IsBase64Encoded)
		require.Equal(t, "YWJj", ctx.Response.Body)
	})
}
