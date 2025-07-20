package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
)

// wrapHandler converts a Lambda handler to a Lift handler
func wrapHandler(fn func(context.Context, events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error)) lift.HandlerFunc {
	return func(ctx *lift.Context) error {
		// Convert Lift context to Lambda request
		lambdaReq := liftToLambdaRequest(ctx)

		// Call the original handler
		resp, err := fn(ctx.Context, lambdaReq)
		if err != nil {
			return lift.NewError(500, "Internal server error", err)
		}

		// Convert Lambda response to Lift response
		return lambdaResponseToLift(ctx, resp)
	}
}

// wrapHandlerWithParam converts a Lambda handler with parameter to a Lift handler
func wrapHandlerWithParam(fn func(context.Context, events.APIGatewayV2HTTPRequest, string) (*events.APIGatewayV2HTTPResponse, error), paramName string) lift.HandlerFunc {
	return func(ctx *lift.Context) error {
		// Get parameter from path
		paramValue := ctx.PathParam(paramName)

		// Convert Lift context to Lambda request
		lambdaReq := liftToLambdaRequest(ctx)

		// Call the original handler
		resp, err := fn(ctx.Context, lambdaReq, paramValue)
		if err != nil {
			return lift.NewError(500, "Internal server error", err)
		}

		// Convert Lambda response to Lift response
		return lambdaResponseToLift(ctx, resp)
	}
}

// liftToLambdaRequest converts a Lift context to a Lambda request
func liftToLambdaRequest(ctx *lift.Context) events.APIGatewayV2HTTPRequest {
	// Convert headers
	headers := make(map[string]string)
	for k, v := range ctx.Request.Headers {
		headers[k] = v
	}

	// Convert query parameters
	queryParams := make(map[string]string)
	for k, v := range ctx.Request.QueryStringParameters {
		queryParams[k] = v
	}

	// Get body
	body := ctx.Request.Body
	isBase64Encoded := false

	// If body is base64 encoded, decode it
	if ctx.Request.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err == nil {
			body = string(decoded)
		}
	}

	return events.APIGatewayV2HTTPRequest{
		Headers:               headers,
		QueryStringParameters: queryParams,
		Body:                  body,
		IsBase64Encoded:       isBase64Encoded,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: ctx.Request.Method,
				Path:   ctx.Request.Path,
			},
		},
	}
}

// lambdaResponseToLift converts a Lambda response to a Lift response
func lambdaResponseToLift(ctx *lift.Context, resp *events.APIGatewayV2HTTPResponse) error {
	// Set headers
	for k, v := range resp.Headers {
		ctx.SetHeader(k, v)
	}

	// Set status code
	if resp.StatusCode != 0 {
		ctx.Response.StatusCode = resp.StatusCode
	}

	// Set body
	if resp.Body != "" {
		// Check if body is JSON
		var jsonBody interface{}
		if err := json.Unmarshal([]byte(resp.Body), &jsonBody); err == nil {
			// Body is JSON, return it as JSON
			return ctx.JSON(jsonBody)
		} else {
			// Body is not JSON, return it as text
			return ctx.String(resp.Body)
		}
	}

	return nil
}

// Helper function to read the request body
func readBody(ctx *lift.Context) (string, error) {
	if ctx.Request.Body == "" {
		return "", nil
	}

	// If body is base64 encoded, decode it
	if ctx.Request.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(ctx.Request.Body)
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}

	return ctx.Request.Body, nil
}

// Helper function to convert a reader to a string
func readerToString(reader io.Reader) (string, error) {
	if reader == nil {
		return "", nil
	}

	buf := new(strings.Builder)
	_, err := io.Copy(buf, reader)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
