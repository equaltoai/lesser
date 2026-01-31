package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func liftHandlerToAppTheory(handler lift.Handler, middleware []lift.Middleware) apptheory.Handler {
	return func(ctx *apptheory.Context) (*apptheory.Response, error) {
		if handler == nil {
			return nil, fmt.Errorf("nil lift handler")
		}

		liftCtx := newLiftContextFromAppTheory(ctx)
		final := handler
		for i := len(middleware) - 1; i >= 0; i-- {
			final = middleware[i](final)
		}

		if err := final.Handle(liftCtx); err != nil {
			return nil, err
		}

		return apptheoryResponseFromLiftResponse(liftCtx.Response)
	}
}

func newLiftContextFromAppTheory(ctx *apptheory.Context) *lift.Context {
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = ctx.Context()
	}

	req := &adapters.Request{
		TriggerType: adapters.TriggerAPIGatewayV2,
		Method:      "",
		Path:        "",
		Headers:     map[string]string{},
		QueryParams: map[string]string{},
		PathParams:  map[string]string{},
		Body:        nil,
	}

	if ctx != nil {
		req.Method = ctx.Request.Method
		req.Path = ctx.Request.Path
		req.Headers = firstHeaderValues(ctx.Request.Headers)
		req.QueryParams = firstHeaderValues(ctx.Request.Query)
		req.Body = append([]byte(nil), ctx.Request.Body...)

		if ctx.Params != nil {
			for k, v := range ctx.Params {
				req.PathParams[k] = v
			}
		}
	}

	liftReq := lift.NewRequestWithContext(baseCtx, req)
	liftCtx := lift.NewContext(baseCtx, liftReq)
	for k, v := range req.PathParams {
		liftCtx.SetParam(k, v)
	}

	return liftCtx
}

func firstHeaderValues(in map[string][]string) map[string]string {
	out := map[string]string{}
	for k, values := range in {
		if len(values) == 0 {
			continue
		}
		out[k] = values[0]
	}
	return out
}

func apptheoryResponseFromLiftResponse(resp *lift.Response) (*apptheory.Response, error) {
	if resp == nil {
		return nil, fmt.Errorf("nil lift response")
	}

	headers := map[string][]string{}
	for k, v := range resp.Headers {
		headers[k] = []string{v}
	}

	body, isBase64, err := liftBodyToBytes(resp.Body, resp.IsBase64Encoded)
	if err != nil {
		return nil, err
	}

	return &apptheory.Response{
		Status:   resp.StatusCode,
		Headers:  headers,
		Cookies:  nil,
		Body:     body,
		IsBase64: isBase64,
	}, nil
}

func liftBodyToBytes(body any, isBase64Encoded bool) ([]byte, bool, error) {
	if body == nil {
		return nil, false, nil
	}

	if isBase64Encoded {
		bodyStr, ok := body.(string)
		if !ok {
			return nil, false, fmt.Errorf("expected base64 body string, got %T", body)
		}
		decoded, err := base64.StdEncoding.DecodeString(bodyStr)
		if err != nil {
			return nil, false, fmt.Errorf("decode base64 body: %w", err)
		}
		return decoded, true, nil
	}

	switch v := body.(type) {
	case string:
		return []byte(v), false, nil
	case []byte:
		return append([]byte(nil), v...), false, nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, false, fmt.Errorf("marshal lift body: %w", err)
		}
		return encoded, false, nil
	}
}
