// Package main provides handler wrappers and utilities for bridging between
// AWS Lambda event formats and the Lift framework, enabling seamless migration
// from traditional Lambda handlers to Lift's more ergonomic request/response model.
package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
)

// wrapHandler wraps a legacy handler function to work with Lift
//
//nolint:unused // Used in tests (infrastructure_test.go, performance_test.go)
func wrapHandler(fn func(context.Context, events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error)) lift.Handler {
	return lift.HandlerFunc(func(ctx *lift.Context) error {
		// Convert Lift request to Lambda event
		lambdaReq := events.APIGatewayV2HTTPRequest{
			Headers:               ctx.Request.Headers,
			QueryStringParameters: ctx.Request.QueryParams,
			PathParameters:        make(map[string]string),
			Body:                  string(ctx.Request.Body),
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method: ctx.Request.Method,
					Path:   ctx.Request.Path,
				},
			},
		}

		// Call the legacy handler
		resp, err := fn(ctx.Context, lambdaReq)
		if err != nil {
			return err
		}

		// Set response status
		ctx.Status(resp.StatusCode)

		// Set response headers
		for k, v := range resp.Headers {
			ctx.Response.Header(k, v)
		}

		// Set response body
		if resp.IsBase64Encoded {
			// Handle base64 encoded responses
			return ctx.Response.Binary([]byte(resp.Body))
		}

		// Return the body as text
		return ctx.Text(resp.Body)
	})
}

// wrapHandlerWithParam wraps a legacy handler that expects a path parameter
//
//nolint:unused // Used in tests (infrastructure_test.go)
func wrapHandlerWithParam(fn func(context.Context, events.APIGatewayV2HTTPRequest, string) (*events.APIGatewayV2HTTPResponse, error), param string) lift.Handler {
	return lift.HandlerFunc(func(ctx *lift.Context) error {
		// Get the parameter value
		paramValue := ctx.PathParam(param)

		// Convert Lift request to Lambda event
		pathParams := make(map[string]string)
		pathParams[param] = paramValue

		lambdaReq := events.APIGatewayV2HTTPRequest{
			Headers:               ctx.Request.Headers,
			QueryStringParameters: ctx.Request.QueryParams,
			PathParameters:        pathParams,
			Body:                  string(ctx.Request.Body),
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method: ctx.Request.Method,
					Path:   ctx.Request.Path,
				},
			},
		}

		// Call the legacy handler
		resp, err := fn(ctx.Context, lambdaReq, paramValue)
		if err != nil {
			return err
		}

		// Set response status
		ctx.Status(resp.StatusCode)

		// Set response headers
		for k, v := range resp.Headers {
			ctx.Response.Header(k, v)
		}

		// Set response body
		if resp.IsBase64Encoded {
			// Handle base64 encoded responses
			return ctx.Response.Binary([]byte(resp.Body))
		}

		// Return the body as text
		return ctx.Text(resp.Body)
	})
}
