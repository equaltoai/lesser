package main

import (
	"fmt"
	"strconv"
	"strings"
)

func bindPayloadSchemas(repoRoot string, spec *openAPISpec, routes []routeDef) error {
	if spec == nil {
		return nil
	}

	builder, err := ensureGeneratedSchemas(spec, repoRoot)
	if err != nil {
		return err
	}

	liftPkg := builder.pkgs[packagePathLift]
	if liftPkg == nil {
		return fmt.Errorf("bind schemas: missing lift package %q", packagePathLift)
	}

	payloads, err := inferLiftHandlerPayloads(liftPkg)
	if err != nil {
		return err
	}

	for i := range routes {
		if err := bindRoutePayloadSchemas(builder, payloads, &routes[i]); err != nil {
			return err
		}
	}

	return nil
}

func bindRoutePayloadSchemas(builder *schemaBuilder, payloads map[string]handlerPayloadInfo, route *routeDef) error {
	if builder == nil || route == nil {
		return nil
	}
	if route.Lambda != lambdaAPI {
		return nil
	}
	if route.Handler == "" {
		return nil
	}

	meta, ok := payloads[route.Handler]
	if !ok {
		return nil
	}

	if shouldBindRequestSchema(route.Method) && meta.Request != nil {
		key, err := builder.schemaKeyForPayloadType(meta.Request)
		if err != nil {
			return err
		}
		if key != "" {
			route.RequestSchema = key
		}
	}

	if meta.Response != nil {
		key, err := builder.schemaKeyForPayloadType(meta.Response)
		if err != nil {
			return err
		}
		if key != "" {
			route.ResponseSchema = key
		}
	}

	if meta.PrimaryCode != 0 {
		route.SuccessStatus = meta.PrimaryCode
	}
	if len(meta.SuccessCodes) > 0 {
		route.SuccessCodes = append([]int(nil), meta.SuccessCodes...)
	}
	if len(meta.QueryParams) > 0 {
		route.QueryParams = append([]string(nil), meta.QueryParams...)
	}

	return nil
}

func shouldBindRequestSchema(method string) bool {
	switch method {
	case methodPOST, methodPUT, methodPATCH:
		return true
	default:
		return false
	}
}

func applyPayloadSchemaRefs(op *operation, route routeDef) {
	if op == nil {
		return
	}

	if route.RequestSchema != "" && op.RequestBody != nil && op.RequestBody.Content != nil {
		body := op.RequestBody.Content["application/json"]
		if shouldReplacePlaceholderSchema(body.Schema) || isGenericSchemaRef(body.Schema) {
			body.Schema = schemaRef{Ref: "#/components/schemas/" + route.RequestSchema}
			op.RequestBody.Content["application/json"] = body
		}
	}

	if route.ResponseSchema != "" {
		if op.Responses == nil {
			op.Responses = map[string]response{}
		}

		code := strconv.Itoa(primarySuccessStatus(route))
		resp := op.Responses[code]
		if resp.Content == nil {
			resp.Content = map[string]mediaType{}
		}

		mt := resp.Content["application/json"]
		if shouldReplaceResponseSchema(mt.Schema) {
			mt.Schema = schemaRef{Ref: "#/components/schemas/" + route.ResponseSchema}
			resp.Content["application/json"] = mt
			op.Responses[code] = resp
		}
	}
}

func shouldReplaceResponseSchema(s schemaRef) bool {
	if s.Ref != "" {
		return isGenericSchemaRef(s)
	}
	if s.Type == "" && s.AdditionalProperties == nil {
		return true
	}
	return shouldReplacePlaceholderSchema(s)
}

func isGenericSchemaRef(s schemaRef) bool {
	return strings.TrimSpace(s.Ref) == "#/components/schemas/"+schemaAnyObject ||
		strings.TrimSpace(s.Ref) == "#/components/schemas/"+schemaAnyArray
}

func shouldReplacePlaceholderSchema(s schemaRef) bool {
	if s.Ref != "" {
		return false
	}
	if s.Type != "object" {
		return false
	}
	switch v := s.AdditionalProperties.(type) {
	case bool:
		return v
	default:
		return false
	}
}
