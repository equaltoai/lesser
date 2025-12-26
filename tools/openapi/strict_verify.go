package main

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type strictCheck struct {
	Method string
	Path   string

	AllowPlaceholderRequest  bool
	AllowPlaceholderResponse bool
	SkipSchemaChecks         bool
}

var strictExemptions = map[string]strictCheck{
	routeKey(methodGET, "/api/graphql"): {
		Method:           methodGET,
		Path:             "/api/graphql",
		SkipSchemaChecks: true,
	},
	routeKey(methodPOST, "/api/graphql"): {
		Method:                   methodPOST,
		Path:                     "/api/graphql",
		AllowPlaceholderRequest:  true,
		AllowPlaceholderResponse: true,
	},
}

func verifyStrictOpenAPI(specPath string, spec *openAPISpec, routes []routeDef, originalSpecBytes []byte) error {
	var problems []string

	if err := validateStrictGeneratedSpec(spec, routes); err != nil {
		problems = append(problems, err.Error())
	}

	expectedSpecBytes, err := marshalSpecBytes(spec)
	if err != nil {
		problems = append(problems, err.Error())
	} else if !bytes.Equal(bytes.TrimSpace(originalSpecBytes), bytes.TrimSpace(expectedSpecBytes)) {
		problems = append(problems, fmt.Sprintf("OpenAPI spec is out-of-date: run `make generate-openapi` (%s)", specPath))
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

func marshalSpecBytes(spec *openAPISpec) ([]byte, error) {
	if spec == nil {
		return nil, errors.New("marshal openapi: spec is nil")
	}
	ensureFoundationComponents(spec)
	out, err := yaml.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal openapi yaml: %w", err)
	}
	out = append(out, '\n')
	return out, nil
}

func validateStrictGeneratedSpec(spec *openAPISpec, routes []routeDef) error {
	if spec == nil {
		return errors.New("strict openapi: spec is nil")
	}

	routeByKey := map[string]routeDef{}
	for _, r := range routes {
		routeByKey[routeKey(r.Method, r.Path)] = r
	}

	var problems []string
	for key, route := range routeByKey {
		item := spec.Paths[route.Path]
		if item == nil {
			problems = append(problems, fmt.Sprintf("missing path item for %s", key))
			continue
		}

		op := getOperation(item, route.Method)
		if op == nil {
			problems = append(problems, fmt.Sprintf("missing operation for %s", key))
			continue
		}

		exemption, exempt := strictExemptions[key]

		if err := validateStrictOperationSecurity(op, route); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %s", key, err.Error()))
		}

		if err := validateStrictOperationQueryParams(op, route); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %s", key, err.Error()))
		}

		if route.Lambda != lambdaAPI {
			continue
		}

		if exempt && exemption.SkipSchemaChecks {
			continue
		}

		if err := validateStrictOperationRequestSchema(op, route, exemption); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %s", key, err.Error()))
		}
		if err := validateStrictOperationResponseSchema(op, route, exemption); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %s", key, err.Error()))
		}
	}

	sort.Strings(problems)
	if len(problems) > 0 {
		return errors.New("strict openapi validation failed:\n" + strings.Join(problems, "\n"))
	}

	return nil
}

func validateStrictOperationSecurity(op *operation, route routeDef) error {
	if op == nil {
		return errors.New("operation is nil")
	}

	switch route.Auth {
	case authModePublic:
		if len(op.Security) != 0 {
			return errors.New("expected public operation (no security requirements)")
		}
	case authModeBearerRequired:
		if !isBearerRequiredSecurity(op.Security) {
			return errors.New("expected bearerAuth required security")
		}
		if _, ok := op.Responses["401"]; !ok {
			return errors.New("missing 401 response for authenticated operation")
		}
		if _, ok := op.Responses["403"]; !ok {
			return errors.New("missing 403 response for authenticated operation")
		}
	case authModeBearerOptional:
		if !isBearerOptionalSecurity(op.Security) {
			return errors.New("expected bearerAuth optional security")
		}
	case authModeSetupBearer:
		if !isSetupBearerSecurity(op.Security) {
			return errors.New("expected setupBearer required security")
		}
		if _, ok := op.Responses["401"]; !ok {
			return errors.New("missing 401 response for authenticated operation")
		}
		if _, ok := op.Responses["403"]; !ok {
			return errors.New("missing 403 response for authenticated operation")
		}
	default:
		return fmt.Errorf("unknown auth mode %q", route.Auth)
	}

	if strings.HasPrefix(route.Path, "/api/v1/admin/") && !isBearerRequiredSecurity(op.Security) {
		return errors.New("/api/v1/admin/* must require bearerAuth")
	}

	return nil
}

func isBearerRequiredSecurity(security []map[string][]string) bool {
	if len(security) != 1 {
		return false
	}
	_, ok := security[0]["bearerAuth"]
	return ok
}

func isBearerOptionalSecurity(security []map[string][]string) bool {
	if len(security) != 2 {
		return false
	}
	if len(security[0]) != 0 {
		return false
	}
	_, ok := security[1]["bearerAuth"]
	return ok
}

func isSetupBearerSecurity(security []map[string][]string) bool {
	if len(security) != 1 {
		return false
	}
	_, ok := security[0]["setupBearer"]
	return ok
}

func validateStrictOperationQueryParams(op *operation, route routeDef) error {
	if op == nil {
		return errors.New("operation is nil")
	}
	if len(route.QueryParams) == 0 {
		return nil
	}

	existing := existingQueryParams(op.Parameters)
	var missing []string

	for _, param := range route.QueryParams {
		name := strings.ToLower(strings.TrimSpace(param))
		if name == "" {
			continue
		}
		if _, ok := existing[name]; ok {
			continue
		}
		missing = append(missing, name)
	}

	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("missing query parameters: %s", strings.Join(missing, ", "))
	}

	// Enforce component parameter refs for known pagination query params.
	var componentRefProblems []string
	for _, name := range route.QueryParams {
		q := strings.ToLower(strings.TrimSpace(name))
		if q == "" {
			continue
		}
		ref := componentParameterRefForQueryParam(q)
		if ref == "" {
			continue
		}
		if !operationHasParameterRef(op, ref) {
			componentRefProblems = append(componentRefProblems, q)
		}
	}

	sort.Strings(componentRefProblems)
	if len(componentRefProblems) > 0 {
		return fmt.Errorf("expected component parameter refs for: %s", strings.Join(componentRefProblems, ", "))
	}

	return nil
}

func operationHasParameterRef(op *operation, ref string) bool {
	if op == nil || ref == "" {
		return false
	}
	ref = strings.TrimSpace(ref)
	for _, p := range op.Parameters {
		if strings.TrimSpace(p.Ref) == ref {
			return true
		}
	}
	return false
}

func validateStrictOperationRequestSchema(op *operation, route routeDef, exemption strictCheck) error {
	if op == nil {
		return errors.New("operation is nil")
	}

	if exemption.AllowPlaceholderRequest {
		return nil
	}

	if op.RequestBody != nil && op.RequestBody.Content != nil {
		for _, mt := range op.RequestBody.Content {
			if isPlaceholderSchema(mt.Schema) {
				return errors.New("request schema is still a placeholder")
			}
		}
	}

	if route.Method != methodPOST && route.Method != methodPUT && route.Method != methodPATCH {
		return nil
	}

	if route.RequestSchema == "" || route.RequestSchema == schemaAnyObject || route.RequestSchema == schemaAnyArray {
		return nil
	}

	if op.RequestBody == nil || op.RequestBody.Content == nil {
		return errors.New("missing requestBody")
	}

	mt, ok := op.RequestBody.Content["application/json"]
	if !ok {
		return errors.New("missing application/json request schema")
	}

	if isPlaceholderSchema(mt.Schema) {
		return errors.New("request schema is still a placeholder")
	}

	expectedRef := "#/components/schemas/" + route.RequestSchema
	if strings.TrimSpace(mt.Schema.Ref) != expectedRef {
		return fmt.Errorf("unexpected request schema ref: got %q want %q", strings.TrimSpace(mt.Schema.Ref), expectedRef)
	}

	return nil
}

func validateStrictOperationResponseSchema(op *operation, route routeDef, exemption strictCheck) error {
	if op == nil {
		return errors.New("operation is nil")
	}

	if exemption.AllowPlaceholderResponse {
		return nil
	}

	primary := primarySuccessStatus(route)
	code := strconv.Itoa(primary)

	resp, ok := op.Responses[code]
	if !ok {
		if route.ResponseSchema == "" || route.ResponseSchema == schemaAnyObject || route.ResponseSchema == schemaAnyArray {
			return nil
		}
		return fmt.Errorf("missing primary success response %s", code)
	}

	if primary == 204 {
		if resp.Content != nil {
			return errors.New("204 response must not define content")
		}
		return nil
	}

	if resp.Content == nil {
		if route.ResponseSchema == "" || route.ResponseSchema == schemaAnyObject || route.ResponseSchema == schemaAnyArray {
			return nil
		}
		return errors.New("missing response content")
	}

	for _, mt := range resp.Content {
		if isPlaceholderSchema(mt.Schema) {
			return errors.New("response schema is still a placeholder")
		}
	}

	if route.ResponseSchema == "" || route.ResponseSchema == schemaAnyObject || route.ResponseSchema == schemaAnyArray {
		return nil
	}
	mt, ok := resp.Content["application/json"]
	if !ok {
		return errors.New("missing application/json response schema")
	}

	expectedRef := "#/components/schemas/" + route.ResponseSchema
	if strings.TrimSpace(mt.Schema.Ref) != expectedRef {
		return fmt.Errorf("unexpected response schema ref: got %q want %q", strings.TrimSpace(mt.Schema.Ref), expectedRef)
	}

	return nil
}

func isPlaceholderSchema(schema schemaRef) bool {
	if isGenericSchemaRef(schema) {
		return true
	}
	if schema.Ref != "" {
		return false
	}
	if schema.Type != "object" {
		return false
	}
	if len(schema.Properties) != 0 || len(schema.Required) != 0 {
		return false
	}
	v, ok := schema.AdditionalProperties.(bool)
	return ok && v
}
