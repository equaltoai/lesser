// Package main provides a drift-checked OpenAPI generator for Lesser's REST surface.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type runOptions struct {
	SpecPath string
	Write    bool
	Check    bool
	Strict   bool
}

type routeDef struct {
	Method         string
	Path           string
	Lambda         string
	Handler        string
	Auth           authMode
	RateLimited    bool
	RequestSchema  string
	ResponseSchema string
	SuccessStatus  int
	SuccessCodes   []int
	QueryParams    []string
	Scopes         []string
	Sources        []string
}

type openAPISpec struct {
	OpenAPI    string               `yaml:"openapi"`
	Info       openAPIInfo          `yaml:"info"`
	Components openAPIComponents    `yaml:"components,omitempty"`
	Paths      map[string]*pathItem `yaml:"paths"`
}

type openAPIInfo struct {
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
}

type openAPIComponents struct {
	SecuritySchemes map[string]securityScheme `yaml:"securitySchemes,omitempty"`
	Schemas         map[string]any            `yaml:"schemas,omitempty"`
	Parameters      map[string]parameter      `yaml:"parameters,omitempty"`
	Responses       map[string]response       `yaml:"responses,omitempty"`
}

type securityScheme struct {
	Type         string      `yaml:"type"`
	Scheme       string      `yaml:"scheme,omitempty"`
	BearerFormat string      `yaml:"bearerFormat,omitempty"`
	Description  string      `yaml:"description,omitempty"`
	Flows        *oauthFlows `yaml:"flows,omitempty"`
}

type oauthFlows struct {
	AuthorizationCode *oauthFlow `yaml:"authorizationCode,omitempty"`
}

type oauthFlow struct {
	AuthorizationURL string            `yaml:"authorizationUrl"`
	TokenURL         string            `yaml:"tokenUrl"`
	Scopes           map[string]string `yaml:"scopes"`
}

type pathItem struct {
	Get    *operation `yaml:"get,omitempty"`
	Post   *operation `yaml:"post,omitempty"`
	Put    *operation `yaml:"put,omitempty"`
	Patch  *operation `yaml:"patch,omitempty"`
	Delete *operation `yaml:"delete,omitempty"`
}

type operation struct {
	OperationID string                `yaml:"operationId,omitempty"`
	Summary     string                `yaml:"summary,omitempty"`
	Description string                `yaml:"description,omitempty"`
	Tags        []string              `yaml:"tags,omitempty"`
	Security    []map[string][]string `yaml:"security,omitempty"`
	Parameters  []parameter           `yaml:"parameters,omitempty"`
	RequestBody *requestBody          `yaml:"requestBody,omitempty"`
	Responses   map[string]response   `yaml:"responses"`
	Extensions  map[string]any        `yaml:",inline,omitempty"`
}

type parameter struct {
	Ref         string    `yaml:"$ref,omitempty"`
	Name        string    `yaml:"name,omitempty"`
	In          string    `yaml:"in,omitempty"`
	Required    bool      `yaml:"required,omitempty"`
	Description string    `yaml:"description,omitempty"`
	Schema      schemaRef `yaml:"schema,omitempty"`
}

type schemaRef struct {
	Type                 string               `yaml:"type,omitempty"`
	Format               string               `yaml:"format,omitempty"`
	Ref                  string               `yaml:"$ref,omitempty"`
	Properties           map[string]schemaRef `yaml:"properties,omitempty"`
	Required             []string             `yaml:"required,omitempty"`
	AdditionalProperties any                  `yaml:"additionalProperties,omitempty"`
}

type requestBody struct {
	Required bool                 `yaml:"required,omitempty"`
	Content  map[string]mediaType `yaml:"content"`
}

type mediaType struct {
	Schema schemaRef `yaml:"schema"`
}

type responseHeader struct {
	Description string    `yaml:"description,omitempty"`
	Required    bool      `yaml:"required,omitempty"`
	Schema      schemaRef `yaml:"schema,omitempty"`
}

type response struct {
	Ref         string                    `yaml:"$ref,omitempty"`
	Description string                    `yaml:"description,omitempty"`
	Content     map[string]mediaType      `yaml:"content,omitempty"`
	Headers     map[string]responseHeader `yaml:"headers,omitempty"`
}

type authMode string

const (
	authModePublic         authMode = "public"
	authModeBearerRequired authMode = "bearer_required"
	authModeBearerOptional authMode = "bearer_optional"
	authModeSetupBearer    authMode = "setup_bearer"
)

const (
	methodGET    = "GET"
	methodPOST   = "POST"
	methodPUT    = "PUT"
	methodPATCH  = "PATCH"
	methodDELETE = "DELETE"

	lambdaAPI     = "api"
	lambdaSSE     = "sse"
	lambdaGraphQL = "graphql"
)

func main() {
	opts := parseOptions()

	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}

	if err := run(repoRoot, opts); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

func parseOptions() runOptions {
	var opts runOptions

	flag.StringVar(&opts.SpecPath, "spec", "docs/specs/openapi.yaml", "path to OpenAPI spec yaml")
	flag.BoolVar(&opts.Write, "write", false, "update the spec file in place")
	flag.BoolVar(&opts.Check, "check", false, "verify spec matches current routes")
	flag.BoolVar(&opts.Strict, "strict", false, "strict verification: ensure schemas, security, and inferred query params are complete and spec is up-to-date")
	flag.Parse()

	if !opts.Write && !opts.Check {
		opts.Check = true
	}
	if opts.Strict {
		opts.Check = true
	}

	return opts
}

func run(repoRoot string, opts runOptions) error {
	absSpec := filepath.Join(repoRoot, filepath.FromSlash(opts.SpecPath))

	var originalSpecBytes []byte
	if opts.Strict {
		data, err := os.ReadFile(absSpec) //nolint:gosec // local spec path
		if err != nil {
			return fmt.Errorf("read %s: %w", absSpec, err)
		}
		originalSpecBytes = data
	}

	spec, err := readOrInitSpec(absSpec, opts.Write)
	if err != nil {
		return err
	}

	currentRoutes, err := extractConfiguredRoutes(repoRoot)
	if err != nil {
		return err
	}
	currentRoutes = sanitizeRoutes(currentRoutes)
	sortRoutes(currentRoutes)

	if err := bindPayloadSchemas(repoRoot, spec, currentRoutes); err != nil {
		return err
	}

	missing, stale := syncSpec(spec, currentRoutes)

	if opts.Write {
		if err := writeSpec(absSpec, spec); err != nil {
			return err
		}
		fmt.Println("updated:", opts.SpecPath)
	}

	if opts.Check {
		if err := reportRouteDrift(opts.SpecPath, missing, stale); err != nil {
			return err
		}
		if opts.Strict {
			if err := verifyStrictOpenAPI(opts.SpecPath, spec, currentRoutes, originalSpecBytes); err != nil {
				return err
			}
		}
		fmt.Printf("ok: %s (%d paths)\n", opts.SpecPath, len(spec.Paths))
	}

	return nil
}

func readOrInitSpec(path string, allowInit bool) (*openAPISpec, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is an operator-supplied local file
	if err != nil {
		if os.IsNotExist(err) && allowInit {
			return defaultSpec(), nil
		}
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: missing file; run `make generate-openapi`", path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var spec openAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if strings.TrimSpace(spec.OpenAPI) == "" {
		return nil, fmt.Errorf("invalid %s: missing openapi field", path)
	}
	if spec.Paths == nil {
		spec.Paths = map[string]*pathItem{}
	}

	ensureFoundationComponents(&spec)
	return &spec, nil
}

func defaultSpec() *openAPISpec {
	spec := &openAPISpec{
		OpenAPI: "3.1.0",
		Info: openAPIInfo{
			Title:       "Lesser REST API",
			Version:     "0.1.0",
			Description: "Auto-generated route skeleton; fill request/response schemas over time. Do not serve this file at runtime; use it for build-time client generation.",
		},
		Components: openAPIComponents{},
		Paths:      map[string]*pathItem{},
	}
	ensureFoundationComponents(spec)
	return spec
}

func ensureFoundationComponents(spec *openAPISpec) {
	if spec == nil {
		return
	}
	ensureSecuritySchemes(spec)
	if spec.Components.SecuritySchemes == nil {
		spec.Components.SecuritySchemes = map[string]securityScheme{}
	}
	if spec.Components.Schemas == nil {
		spec.Components.Schemas = map[string]any{}
	}
	if spec.Components.Parameters == nil {
		spec.Components.Parameters = map[string]parameter{}
	}
	if spec.Components.Responses == nil {
		spec.Components.Responses = map[string]response{}
	}

	if _, ok := spec.Components.SecuritySchemes["bearerAuth"]; !ok {
		spec.Components.SecuritySchemes["bearerAuth"] = securityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "OAuth access token (JWT): `Authorization: Bearer <access_token>`.",
		}
	}

	if _, ok := spec.Components.SecuritySchemes["setupBearer"]; !ok {
		spec.Components.SecuritySchemes["setupBearer"] = securityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "opaque",
			Description:  "Temporary setup session token: `Authorization: Bearer <setup_token>` (issued by `/setup/bootstrap/verify`).",
		}
	}

	ensureOAuth2Scheme(spec)
	ensureFoundationSchemas(spec)
	ensureFoundationParameters(spec)
	ensureFoundationResponses(spec)
}

func ensureSecuritySchemes(spec *openAPISpec) {
	if spec == nil {
		return
	}
	if spec.Components.SecuritySchemes == nil {
		spec.Components.SecuritySchemes = map[string]securityScheme{}
	}
}

func ensureOAuth2Scheme(spec *openAPISpec) {
	if spec == nil {
		return
	}

	const schemeName = "oauth2"

	desired := securityScheme{
		Type: "oauth2",
		Flows: &oauthFlows{
			AuthorizationCode: &oauthFlow{
				AuthorizationURL: "/oauth/authorize",
				TokenURL:         "/oauth/token",
				Scopes: map[string]string{
					"read":                "Read access",
					"write":               "Write access",
					"follow":              "Follow-related access",
					"push":                "Push notification access",
					"admin":               "Administrative access",
					"admin:read":          "Administrative read access",
					"admin:write":         "Administrative write access",
					"admin:accounts":      "Administrative accounts access",
					"admin:all":           "Full administrative access",
					"read:accounts":       "Read accounts",
					"write:accounts":      "Write accounts",
					"read:statuses":       "Read statuses",
					"write:statuses":      "Write statuses",
					"write:media":         "Write media (uploads)",
					"read:follows":        "Read follow relationships",
					"write:follows":       "Write follow relationships",
					"read:blocks":         "Read blocks",
					"write:blocks":        "Write blocks",
					"read:notifications":  "Read notifications",
					"write:notifications": "Write notifications",
					"read:filters":        "Read filters",
					"write:filters":       "Write filters",
					"moderation":          "Moderation actions (internal)",
					"debug":               "Debug endpoints (internal)",
				},
			},
		},
	}

	existing, ok := spec.Components.SecuritySchemes[schemeName]
	if !ok {
		spec.Components.SecuritySchemes[schemeName] = desired
		return
	}

	if existing.Type == "" {
		existing.Type = desired.Type
	}
	if existing.Flows == nil || existing.Flows.AuthorizationCode == nil {
		existing.Flows = desired.Flows
		spec.Components.SecuritySchemes[schemeName] = existing
		return
	}

	if existing.Flows.AuthorizationCode.AuthorizationURL == "" {
		existing.Flows.AuthorizationCode.AuthorizationURL = desired.Flows.AuthorizationCode.AuthorizationURL
	}
	if existing.Flows.AuthorizationCode.TokenURL == "" {
		existing.Flows.AuthorizationCode.TokenURL = desired.Flows.AuthorizationCode.TokenURL
	}
	if existing.Flows.AuthorizationCode.Scopes == nil {
		existing.Flows.AuthorizationCode.Scopes = map[string]string{}
	}
	for scope, desc := range desired.Flows.AuthorizationCode.Scopes {
		if _, ok := existing.Flows.AuthorizationCode.Scopes[scope]; ok {
			continue
		}
		existing.Flows.AuthorizationCode.Scopes[scope] = desc
	}

	spec.Components.SecuritySchemes[schemeName] = existing
}

func ensureFoundationSchemas(spec *openAPISpec) {
	if spec == nil {
		return
	}
	if spec.Components.Schemas == nil {
		spec.Components.Schemas = map[string]any{}
	}

	if _, ok := spec.Components.Schemas["Error"]; !ok {
		spec.Components.Schemas["Error"] = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{
					"type":        "string",
					"description": "Short error message.",
				},
				"error_description": map[string]any{
					"type":        "string",
					"description": "Additional error details (optional).",
				},
				"error_code": map[string]any{
					"type":        "string",
					"description": "Machine-readable error code (optional).",
				},
			},
			"required":             []string{"error"},
			"additionalProperties": false,
		}
	}

	if _, ok := spec.Components.Schemas["RFC3339DateTime"]; !ok {
		spec.Components.Schemas["RFC3339DateTime"] = map[string]any{
			"type":   "string",
			"format": "date-time",
		}
	}

	if _, ok := spec.Components.Schemas["URI"]; !ok {
		spec.Components.Schemas["URI"] = map[string]any{
			"type":   "string",
			"format": "uri",
		}
	}

	if _, ok := spec.Components.Schemas["SnowflakeID"]; !ok {
		spec.Components.Schemas["SnowflakeID"] = map[string]any{
			"type":        "string",
			"pattern":     "^[0-9]+$",
			"description": "Mastodon-compatible snowflake identifier (stringified uint).",
		}
	}

	if _, ok := spec.Components.Schemas["GraphQLRequest"]; !ok {
		spec.Components.Schemas["GraphQLRequest"] = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "GraphQL query document.",
				},
				"variables": map[string]any{
					"type":                 "object",
					"description":          "GraphQL variables map (JSON object).",
					"additionalProperties": true,
				},
				"operationName": map[string]any{
					"type":        "string",
					"description": "GraphQL operation name (optional).",
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		}
	}

	if _, ok := spec.Components.Schemas["GraphQLError"]; !ok {
		spec.Components.Schemas["GraphQLError"] = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Error message.",
				},
				"locations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"line":   map[string]any{"type": "integer", "format": "int32"},
							"column": map[string]any{"type": "integer", "format": "int32"},
						},
						"required":             []string{"line", "column"},
						"additionalProperties": false,
					},
				},
				"path": map[string]any{
					"type":        "array",
					"description": "Path of the field that experienced the error (optional).",
					"items":       map[string]any{},
				},
				"extensions": map[string]any{
					"type":                 "object",
					"description":          "Additional error metadata (optional).",
					"additionalProperties": true,
				},
			},
			"required":             []string{"message"},
			"additionalProperties": false,
		}
	}

	if _, ok := spec.Components.Schemas["GraphQLResponse"]; !ok {
		spec.Components.Schemas["GraphQLResponse"] = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"data": map[string]any{
					"type":                 "object",
					"description":          "GraphQL response data (JSON object).",
					"additionalProperties": true,
				},
				"errors": map[string]any{
					"type": "array",
					"items": map[string]any{
						"$ref": "#/components/schemas/GraphQLError",
					},
				},
			},
			"additionalProperties": false,
		}
	}
}

func ensureFoundationParameters(spec *openAPISpec) {
	if spec == nil {
		return
	}
	if spec.Components.Parameters == nil {
		spec.Components.Parameters = map[string]parameter{}
	}

	addParam := func(name string, p parameter) {
		if _, ok := spec.Components.Parameters[name]; ok {
			return
		}
		spec.Components.Parameters[name] = p
	}

	addParam("Limit", parameter{
		Name:        "limit",
		In:          "query",
		Required:    false,
		Description: "Maximum number of items to return.",
		Schema: schemaRef{
			Type:   "integer",
			Format: "int32",
		},
	})
	addParam("MaxID", parameter{
		Name:        "max_id",
		In:          "query",
		Required:    false,
		Description: "Return results with an ID less than this value.",
		Schema:      schemaRef{Type: "string"},
	})
	addParam("SinceID", parameter{
		Name:        "since_id",
		In:          "query",
		Required:    false,
		Description: "Return results with an ID greater than this value.",
		Schema:      schemaRef{Type: "string"},
	})
	addParam("MinID", parameter{
		Name:        "min_id",
		In:          "query",
		Required:    false,
		Description: "Return results with an ID greater than or equal to this value.",
		Schema:      schemaRef{Type: "string"},
	})
	addParam("Page", parameter{
		Name:        "page",
		In:          "query",
		Required:    false,
		Description: "Page number for page-based pagination.",
		Schema: schemaRef{
			Type:   "integer",
			Format: "int32",
		},
	})
	addParam("Offset", parameter{
		Name:        "offset",
		In:          "query",
		Required:    false,
		Description: "Offset for offset-based pagination.",
		Schema: schemaRef{
			Type:   "integer",
			Format: "int32",
		},
	})
	addParam("Cursor", parameter{
		Name:        "cursor",
		In:          "query",
		Required:    false,
		Description: "Cursor for cursor-based pagination.",
		Schema:      schemaRef{Type: "string"},
	})
}

func ensureFoundationResponses(spec *openAPISpec) {
	if spec == nil {
		return
	}
	if spec.Components.Responses == nil {
		spec.Components.Responses = map[string]response{}
	}

	errorSchema := schemaRef{Ref: "#/components/schemas/Error"}
	errorContent := map[string]mediaType{
		"application/json": {Schema: errorSchema},
	}

	addResponse := func(name, desc string, content map[string]mediaType) {
		if _, ok := spec.Components.Responses[name]; ok {
			return
		}
		spec.Components.Responses[name] = response{
			Description: desc,
			Content:     content,
		}
	}

	addResponse("BadRequest", "Bad Request", errorContent)
	addResponse("Unauthorized", "Unauthorized", errorContent)
	addResponse("Forbidden", "Forbidden", errorContent)
	addResponse("NotFound", "Not Found", errorContent)
	addResponse("Conflict", "Conflict", errorContent)
	addResponse("UnprocessableEntity", "Unprocessable Entity", errorContent)
	desiredTooMany := response{
		Description: "Too Many Requests",
		Content:     errorContent,
		Headers: map[string]responseHeader{
			"Retry-After": {
				Description: "Number of seconds to wait before retrying.",
				Schema:      schemaRef{Type: "integer", Format: "int32"},
			},
			"X-RateLimit-Limit": {
				Description: "Request limit per window.",
				Schema:      schemaRef{Type: "integer", Format: "int32"},
			},
			"X-RateLimit-Remaining": {
				Description: "Requests remaining in the current window.",
				Schema:      schemaRef{Type: "integer", Format: "int32"},
			},
			"X-RateLimit-Reset": {
				Description: "Unix timestamp (seconds) when the current window resets.",
				Schema:      schemaRef{Type: "integer", Format: "int64"},
			},
		},
	}
	if existing, ok := spec.Components.Responses["TooManyRequests"]; !ok {
		spec.Components.Responses["TooManyRequests"] = desiredTooMany
	} else {
		if existing.Description == "" {
			existing.Description = desiredTooMany.Description
		}
		if existing.Content == nil {
			existing.Content = desiredTooMany.Content
		}
		if existing.Headers == nil {
			existing.Headers = map[string]responseHeader{}
		}
		for name, hdr := range desiredTooMany.Headers {
			if _, ok := existing.Headers[name]; ok {
				continue
			}
			existing.Headers[name] = hdr
		}
		spec.Components.Responses["TooManyRequests"] = existing
	}
	addResponse("InternalServerError", "Internal Server Error", errorContent)
	addResponse("ServiceUnavailable", "Service Unavailable", errorContent)
}

func syncSpec(spec *openAPISpec, routes []routeDef) (missing []routeDef, stale []routeDef) {
	configured := make(map[string]routeDef, len(routes))
	for _, r := range routes {
		configured[routeKey(r.Method, r.Path)] = r
	}

	// Add missing routes.
	for _, r := range routes {
		item := spec.Paths[r.Path]
		if item == nil {
			item = &pathItem{}
			spec.Paths[r.Path] = item
		}
		if getOperation(item, r.Method) == nil {
			missing = append(missing, r)
			setOperation(item, r.Method, newOperation(r))
		} else {
			// Ensure basic invariants for existing operations.
			op := getOperation(item, r.Method)
			ensureOperationDefaults(op, r)
		}
	}

	// Remove stale routes.
	for path, item := range spec.Paths {
		if item == nil {
			continue
		}

		for _, method := range []string{methodGET, methodPOST, methodPUT, methodPATCH, methodDELETE} {
			op := getOperation(item, method)
			if op == nil {
				continue
			}
			if _, ok := configured[routeKey(method, path)]; !ok {
				stale = append(stale, routeDef{Method: method, Path: path})
				setOperation(item, method, nil)
				continue
			}
			ensureOperationDefaults(op, configured[routeKey(method, path)])
		}

		if getOperation(item, methodGET) == nil &&
			getOperation(item, methodPOST) == nil &&
			getOperation(item, methodPUT) == nil &&
			getOperation(item, methodPATCH) == nil &&
			getOperation(item, methodDELETE) == nil {
			delete(spec.Paths, path)
		}
	}

	sortRoutes(missing)
	sortRoutes(stale)
	return missing, stale
}

func ensureOperationDefaults(op *operation, route routeDef) {
	if op == nil {
		return
	}

	ensureGeneratedExtensions(op, route)
	applyAuthDefaults(op, route)

	if strings.TrimSpace(op.OperationID) == "" {
		op.OperationID = buildOperationID(route.Method, route.Path)
	}
	if len(op.Responses) == 0 {
		op.Responses = map[string]response{"200": {Description: "OK"}}
	}
	ensureStandardResponses(op, route)
	ensurePrimarySuccessResponse(op, route)

	// Ensure path parameters exist.
	params := extractPathParams(route.Path)
	if len(params) > 0 {
		existing := make(map[string]parameter, len(op.Parameters))
		for _, p := range op.Parameters {
			if strings.EqualFold(p.In, "path") && strings.TrimSpace(p.Name) != "" {
				existing[p.Name] = p
			}
		}
		for _, name := range params {
			if _, ok := existing[name]; ok {
				continue
			}
			op.Parameters = append(op.Parameters, parameter{
				Name:     name,
				In:       "path",
				Required: true,
				Schema:   schemaRef{Type: "string"},
			})
		}
		sort.Slice(op.Parameters, func(i, j int) bool {
			if op.Parameters[i].In == op.Parameters[j].In {
				return op.Parameters[i].Name < op.Parameters[j].Name
			}
			return op.Parameters[i].In < op.Parameters[j].In
		})
	}

	// Ensure a placeholder request body for write methods (optional by default).
	if route.Method == methodPOST || route.Method == methodPUT || route.Method == methodPATCH {
		if op.RequestBody == nil {
			shouldCreatePlaceholder := strings.TrimSpace(route.RequestSchema) != "" || strings.TrimSpace(route.Path) == pathGraphQL
			if shouldCreatePlaceholder {
				op.RequestBody = &requestBody{
					Required: false,
					Content: map[string]mediaType{
						"application/json": {Schema: schemaRef{Type: "object", AdditionalProperties: true}},
					},
				}
			}
		}
	}

	applyOperationOverrides(op, route)
	ensureInferredQueryParams(op, route)
	applyPayloadSchemaRefs(op, route)
	cleanupPlaceholderRequestBody(op, route)
	ensureStandardResponseHeaders(op, route)
}

func newOperation(route routeDef) *operation {
	op := &operation{
		OperationID: buildOperationID(route.Method, route.Path),
		Tags:        deriveTags(route.Path),
		Responses:   map[string]response{"200": {Description: "OK"}},
	}

	ensureOperationDefaults(op, route)
	return op
}

func deriveTags(path string) []string {
	trimmed := strings.TrimPrefix(path, "/")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return []string{"root"}
	}

	parts := strings.Split(trimmed, "/")
	switch parts[0] {
	case "api":
		if len(parts) == 2 && parts[1] == lambdaGraphQL {
			return []string{lambdaGraphQL}
		}
		if len(parts) >= 3 && parts[1] != "" {
			if parts[2] == "admin" {
				return []string{"admin"}
			}
			return []string{parts[2]}
		}
		return []string{"api"}
	case "oauth":
		return []string{"oauth"}
	case "auth":
		return []string{"auth"}
	case "setup":
		return []string{"setup"}
	case ".well-known":
		return []string{"well-known"}
	case "nodeinfo":
		return []string{"nodeinfo"}
	case "users":
		return []string{"activitypub"}
	case "health":
		return []string{"health"}
	case "embed":
		return []string{"embed"}
	default:
		return []string{parts[0]}
	}
}

func buildOperationID(method, path string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	path = strings.TrimSpace(path)

	// Convert "/api/v1/accounts/{id}" -> "api_v1_accounts_by_id"
	normalized := strings.TrimPrefix(path, "/")
	normalized = strings.ReplaceAll(normalized, "/", "_")
	normalized = strings.ReplaceAll(normalized, "{", "by_")
	normalized = strings.ReplaceAll(normalized, "}", "")
	normalized = regexp.MustCompile(`[^a-zA-Z0-9_]+`).ReplaceAllString(normalized, "_")
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		normalized = "root"
	}
	return method + "_" + normalized
}

func extractPathParams(path string) []string {
	re := regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, m := range matches {
		if len(m) != 2 {
			continue
		}
		name := m[1]
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func getOperation(item *pathItem, method string) *operation {
	if item == nil {
		return nil
	}
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case methodGET:
		return item.Get
	case methodPOST:
		return item.Post
	case methodPUT:
		return item.Put
	case methodPATCH:
		return item.Patch
	case methodDELETE:
		return item.Delete
	default:
		return nil
	}
}

func setOperation(item *pathItem, method string, op *operation) {
	if item == nil {
		return
	}
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case methodGET:
		item.Get = op
	case methodPOST:
		item.Post = op
	case methodPUT:
		item.Put = op
	case methodPATCH:
		item.Patch = op
	case methodDELETE:
		item.Delete = op
	}
}

func writeSpec(path string, spec *openAPISpec) error {
	if spec == nil {
		return errors.New("spec is nil")
	}
	ensureFoundationComponents(spec)

	out, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	out = append(out, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	return os.Rename(tmp, path)
}

func reportRouteDrift(specPath string, missing []routeDef, stale []routeDef) error {
	var problems []string
	if len(missing) > 0 {
		var lines []string
		for _, r := range missing {
			lines = append(lines, fmt.Sprintf("  - %s %s", r.Method, r.Path))
		}
		problems = append(problems, fmt.Sprintf("missing routes in %s:\n%s", specPath, strings.Join(lines, "\n")))
	}
	if len(stale) > 0 {
		var lines []string
		for _, r := range stale {
			lines = append(lines, fmt.Sprintf("  - %s %s", r.Method, r.Path))
		}
		problems = append(problems, fmt.Sprintf("stale routes in %s (no longer configured):\n%s", specPath, strings.Join(lines, "\n")))
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n\n"))
	}
	return nil
}

func findRepoRoot() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := start
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "cmd", "api", "routes_lift.go")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("unable to locate repo root (expected go.mod and cmd/api/routes_lift.go)")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

type routeAgg struct {
	Method      string
	Path        string
	Lambda      string
	Handler     string
	LambdaScore int
	Auth        authMode
	AuthScore   int
	RateLimited bool
	Sources     map[string]struct{}
}

func extractConfiguredRoutes(repoRoot string) ([]routeDef, error) {
	routesByKey := make(map[string]*routeAgg)

	apiLiftRoutes, err := extractAPILiftRoutes(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, r := range apiLiftRoutes {
		addRouteAgg(routesByKey, r, filepath.ToSlash("cmd/api/routes_lift.go"))
	}

	// API (health, etc) routes.
	apiRoutes, err := extractRoutesFromSourceFile(repoRoot, "cmd/api/main.go", lambdaAPI, authModePublic)
	if err != nil {
		return nil, err
	}
	for _, r := range apiRoutes {
		addRouteAgg(routesByKey, r, filepath.ToSlash("cmd/api/main.go"))
	}

	// SSE streaming routes.
	sseRoutes, err := extractRoutesFromSourceFile(repoRoot, "cmd/sse/main.go", lambdaSSE, authModePublic)
	if err != nil {
		return nil, err
	}
	for _, r := range sseRoutes {
		addRouteAgg(routesByKey, r, filepath.ToSlash("cmd/sse/main.go"))
	}

	// Inventory-driven HTTP lambdas (federation + webfinger + GraphQL gateway).
	inventoryRoutes, err := extractInventoryHTTPRoutes(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, r := range inventoryRoutes {
		source := filepath.ToSlash("infra/cdk/inventory/lambdas.go") + ":" + strings.TrimSpace(r.Lambda)
		r.Auth = authModePublic
		if strings.EqualFold(r.Lambda, lambdaGraphQL) {
			r.Auth = authModeBearerOptional
		}
		addRouteAgg(routesByKey, r, source)
	}

	routes := finalizeRouteAgg(routesByKey)
	sortRoutes(routes)
	return routes, nil
}

func addRouteAgg(routesByKey map[string]*routeAgg, r routeDef, source string) {
	if routesByKey == nil {
		return
	}

	method := strings.ToUpper(strings.TrimSpace(r.Method))
	path := normalizePath(r.Path)
	lambda := strings.TrimSpace(r.Lambda)
	handler := strings.TrimSpace(r.Handler)
	auth := r.Auth
	rateLimited := r.RateLimited

	if method == "" || path == "" {
		return
	}
	if method != methodGET && method != methodPOST && method != methodPUT && method != methodPATCH && method != methodDELETE {
		return
	}

	key := routeKey(method, path)
	entry := routesByKey[key]
	if entry == nil {
		entry = &routeAgg{
			Method:      method,
			Path:        path,
			Lambda:      lambda,
			Handler:     handler,
			LambdaScore: lambdaPriority(lambda),
			Auth:        auth,
			AuthScore:   authPriority(auth),
			RateLimited: rateLimited,
			Sources:     map[string]struct{}{},
		}
		routesByKey[key] = entry
	}

	if source != "" {
		entry.Sources[source] = struct{}{}
	}
	if score := lambdaPriority(lambda); score > entry.LambdaScore {
		entry.Lambda = lambda
		entry.LambdaScore = score
		if handler != "" {
			entry.Handler = handler
		}
		entry.Auth = auth
		entry.AuthScore = authPriority(auth)
		entry.RateLimited = rateLimited
	}
	if entry.Handler == "" && handler != "" {
		entry.Handler = handler
	}
	if score := authPriority(auth); score > entry.AuthScore {
		entry.Auth = auth
		entry.AuthScore = score
	}
	if rateLimited {
		entry.RateLimited = true
	}
}

func finalizeRouteAgg(routesByKey map[string]*routeAgg) []routeDef {
	var routes []routeDef
	for _, entry := range routesByKey {
		sources := make([]string, 0, len(entry.Sources))
		for s := range entry.Sources {
			sources = append(sources, s)
		}
		sort.Strings(sources)

		routes = append(routes, routeDef{
			Method:      entry.Method,
			Path:        entry.Path,
			Lambda:      strings.TrimSpace(entry.Lambda),
			Handler:     strings.TrimSpace(entry.Handler),
			Auth:        entry.Auth,
			RateLimited: entry.RateLimited,
			Sources:     sources,
		})
	}
	return routes
}

func extractRoutesFromSourceFile(repoRoot, relPath, lambda string, auth authMode) ([]routeDef, error) {
	absPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	data, err := os.ReadFile(absPath) //nolint:gosec // local source file path
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", absPath, err)
	}

	callRE := regexp.MustCompile(`\bapp\.(GET|POST|PUT|PATCH|DELETE)\("([^"]+)"`)
	handleRE := regexp.MustCompile(`\bapp\.Handle\("([^"]+)",\s*"([^"]+)"`)

	var routes []routeDef
	for _, line := range strings.Split(string(data), "\n") {
		if m := callRE.FindStringSubmatch(line); len(m) == 3 {
			routes = append(routes, routeDef{Method: m[1], Path: m[2], Lambda: lambda, Auth: auth})
		}
		if m := handleRE.FindStringSubmatch(line); len(m) == 3 {
			routes = append(routes, routeDef{Method: m[1], Path: m[2], Lambda: lambda, Auth: auth})
		}
	}
	return routes, nil
}

func sanitizeRoutes(routes []routeDef) []routeDef {
	var out []routeDef
	for _, r := range routes {
		r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
		r.Path = normalizePath(r.Path)
		r.Lambda = strings.TrimSpace(r.Lambda)
		if r.Auth == "" {
			r.Auth = authModePublic
		}
		r.Auth = applyAuthOverrides(r.Method, r.Path, "", r.Lambda, r.Auth)
		if r.Method == "" || r.Path == "" {
			continue
		}
		if r.Method != methodGET && r.Method != methodPOST && r.Method != methodPUT && r.Method != methodPATCH && r.Method != methodDELETE {
			continue
		}
		out = append(out, r)
	}
	return out
}

func sortRoutes(routes []routeDef) {
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
}

func routeKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	parts := strings.Split(path, "/")
	for i, part := range parts {
		if len(part) > 1 && strings.HasPrefix(part, ":") {
			name := strings.TrimSpace(strings.TrimPrefix(part, ":"))
			if name != "" {
				parts[i] = "{" + name + "}"
			}
		}
	}

	normalized := strings.Join(parts, "/")
	for strings.Contains(normalized, "//") {
		normalized = strings.ReplaceAll(normalized, "//", "/")
	}
	if len(normalized) > 1 {
		normalized = strings.TrimSuffix(normalized, "/")
	}
	return normalized
}

func lambdaPriority(lambda string) int {
	lambda = strings.TrimSpace(lambda)
	switch lambda {
	case "":
		return 0
	case lambdaAPI:
		return 10
	case lambdaSSE:
		return 20
	case lambdaGraphQL:
		return 20
	default:
		return 30
	}
}

func authPriority(mode authMode) int {
	switch mode {
	case authModeSetupBearer:
		return 40
	case authModeBearerRequired:
		return 30
	case authModeBearerOptional:
		return 20
	case authModePublic:
		return 10
	default:
		return 0
	}
}

func ensureGeneratedExtensions(op *operation, route routeDef) {
	if op == nil {
		return
	}
	if op.Extensions == nil {
		op.Extensions = map[string]any{}
	}

	lambda := strings.TrimSpace(route.Lambda)
	if lambda == "" {
		delete(op.Extensions, "x-lesser-lambda")
	} else {
		op.Extensions["x-lesser-lambda"] = lambda
	}

	if len(route.Sources) == 0 {
		delete(op.Extensions, "x-lesser-routeSources")
	} else {
		sources := append([]string(nil), route.Sources...)
		sort.Strings(sources)
		op.Extensions["x-lesser-routeSources"] = sources
	}

	handler := strings.TrimSpace(route.Handler)
	if handler == "" {
		delete(op.Extensions, "x-lesser-handler")
	} else {
		op.Extensions["x-lesser-handler"] = handler
	}

	if route.Auth == authModeBearerRequired || route.Auth == authModeBearerOptional {
		if len(route.Scopes) == 0 {
			delete(op.Extensions, "x-oauth-scopes")
		} else {
			scopes := append([]string(nil), route.Scopes...)
			sort.Strings(scopes)
			op.Extensions["x-oauth-scopes"] = scopes
		}
	} else {
		delete(op.Extensions, "x-oauth-scopes")
	}
}

func applyAuthDefaults(op *operation, route routeDef) {
	if op == nil {
		return
	}

	switch route.Auth {
	case authModeBearerRequired:
		op.Security = []map[string][]string{{"bearerAuth": {}}}
	case authModeBearerOptional:
		op.Security = []map[string][]string{{}, {"bearerAuth": {}}}
	case authModeSetupBearer:
		op.Security = []map[string][]string{{"setupBearer": {}}}
	case authModePublic:
		op.Security = nil
	default:
		op.Security = nil
	}
}

func ensureStandardResponses(op *operation, route routeDef) {
	if op == nil {
		return
	}
	if op.Responses == nil {
		op.Responses = map[string]response{}
	}

	ensureResponseRef(op.Responses, "500", "InternalServerError")

	if route.RateLimited {
		ensureResponseRef(op.Responses, "429", "TooManyRequests")
	}

	if route.Auth == authModeBearerRequired || route.Auth == authModeBearerOptional || route.Auth == authModeSetupBearer {
		ensureResponseRef(op.Responses, "401", "Unauthorized")
		ensureResponseRef(op.Responses, "403", "Forbidden")
	}

	ensureResponseRef(op.Responses, "400", "BadRequest")

	if len(extractPathParams(route.Path)) > 0 {
		ensureResponseRef(op.Responses, "404", "NotFound")
	}

	if route.Method == methodPOST || route.Method == methodPUT || route.Method == methodPATCH {
		ensureResponseRef(op.Responses, "422", "UnprocessableEntity")
	}

	if strings.HasPrefix(route.Path, "/health") || route.Lambda == "sse" {
		ensureResponseRef(op.Responses, "503", "ServiceUnavailable")
	}
}

func ensureResponseRef(responses map[string]response, statusCode string, componentName string) {
	if responses == nil {
		return
	}
	statusCode = strings.TrimSpace(statusCode)
	componentName = strings.TrimSpace(componentName)
	if statusCode == "" || componentName == "" {
		return
	}
	if _, ok := responses[statusCode]; ok {
		return
	}
	responses[statusCode] = response{Ref: "#/components/responses/" + componentName}
}

func extractInventoryHTTPRoutes(repoRoot string) ([]routeDef, error) {
	path := filepath.Join(repoRoot, "infra", "cdk", "inventory", "lambdas.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	inventoryLit, err := findInventoryComposite(file)
	if err != nil {
		return nil, fmt.Errorf("extract inventory routes: %w", err)
	}

	lambdasLit, err := findCompositeField(inventoryLit, "Lambdas")
	if err != nil {
		return nil, fmt.Errorf("extract inventory routes: %w", err)
	}

	var routes []routeDef
	for _, elt := range lambdasLit.Elts {
		lambdaLit, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}

		name, ok := findStringField(lambdaLit, "Name")
		if !ok {
			return nil, fmt.Errorf("extract inventory routes: lambda spec missing Name in %s", path)
		}

		httpRoutesLit, err := findCompositeFieldOptional(lambdaLit, "HTTPRoutes")
		if err != nil {
			return nil, fmt.Errorf("extract inventory routes: %w", err)
		}
		if httpRoutesLit == nil {
			continue
		}

		for _, routeElt := range httpRoutesLit.Elts {
			routeLit, ok := routeElt.(*ast.CompositeLit)
			if !ok {
				continue
			}
			method, ok := findStringField(routeLit, "Method")
			if !ok {
				return nil, fmt.Errorf("extract inventory routes: HTTPRoute missing Method for lambda %q", name)
			}
			routePath, ok := findStringField(routeLit, "Path")
			if !ok {
				return nil, fmt.Errorf("extract inventory routes: HTTPRoute missing Path for lambda %q", name)
			}

			method = strings.ToUpper(strings.TrimSpace(method))
			routePath = normalizePath(routePath)
			if method == "ANY" {
				continue
			}
			if strings.Contains(routePath, "{proxy+}") {
				continue
			}

			routes = append(routes, routeDef{
				Method: method,
				Path:   routePath,
				Lambda: name,
			})
		}
	}

	sortRoutes(routes)
	return routes, nil
}

type apiRouteMeta struct {
	Method      string
	Path        string
	Handler     string
	RateLimited bool
}

func extractAPILiftRoutes(repoRoot string) ([]routeDef, error) {
	handlerAuth, err := classifyAPILiftHandlerAuthModes(repoRoot)
	if err != nil {
		return nil, err
	}

	metas, err := extractAPIRouteMetadata(repoRoot)
	if err != nil {
		return nil, err
	}

	var routes []routeDef
	for _, meta := range metas {
		auth := handlerAuth[meta.Handler]
		if auth == "" {
			auth = authModePublic
		}
		auth = applyAuthOverrides(meta.Method, meta.Path, meta.Handler, "api", auth)

		routes = append(routes, routeDef{
			Method:      meta.Method,
			Path:        meta.Path,
			Lambda:      lambdaAPI,
			Handler:     meta.Handler,
			Auth:        auth,
			RateLimited: meta.RateLimited,
		})
	}

	sortRoutes(routes)
	return routes, nil
}

func extractAPIRouteMetadata(repoRoot string) ([]apiRouteMeta, error) {
	path := filepath.Join(repoRoot, "cmd", "api", "routes_lift.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var routes []apiRouteMeta

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		meta, ok := extractAPIRouteMeta(call)
		if ok {
			routes = append(routes, meta)
		}
		return true
	})

	return routes, nil
}

func extractAPIRouteMeta(call *ast.CallExpr) (apiRouteMeta, bool) {
	if call == nil {
		return apiRouteMeta{}, false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return apiRouteMeta{}, false
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok || recv.Name != "app" {
		return apiRouteMeta{}, false
	}

	method := sel.Sel.Name
	switch method {
	case methodGET, methodPOST, methodPUT, methodPATCH, methodDELETE:
		return extractAPIRouteMetaFromArgs(method, call.Args, 1)
	case "Handle":
		return extractAPIRouteMetaFromHandleArgs(call.Args)
	default:
		return apiRouteMeta{}, false
	}
}

func extractAPIRouteMetaFromArgs(method string, args []ast.Expr, handlerIndex int) (apiRouteMeta, bool) {
	if len(args) <= handlerIndex {
		return apiRouteMeta{}, false
	}
	routePath, ok := evalStringLiteral(args[0])
	if !ok {
		return apiRouteMeta{}, false
	}
	handlerExpr := args[handlerIndex]
	handlerName, _ := extractHandlerName(handlerExpr)
	return apiRouteMeta{
		Method:      method,
		Path:        routePath,
		Handler:     handlerName,
		RateLimited: isRateLimitedHandler(handlerExpr),
	}, true
}

func extractAPIRouteMetaFromHandleArgs(args []ast.Expr) (apiRouteMeta, bool) {
	if len(args) < 3 {
		return apiRouteMeta{}, false
	}
	methodStr, ok := evalStringLiteral(args[0])
	if !ok {
		return apiRouteMeta{}, false
	}
	methodStr = strings.ToUpper(strings.TrimSpace(methodStr))
	if methodStr != methodGET && methodStr != methodPOST && methodStr != methodPUT && methodStr != methodPATCH && methodStr != methodDELETE {
		return apiRouteMeta{}, false
	}
	routePath, ok := evalStringLiteral(args[1])
	if !ok {
		return apiRouteMeta{}, false
	}
	handlerExpr := args[2]
	handlerName, _ := extractHandlerName(handlerExpr)
	return apiRouteMeta{
		Method:      methodStr,
		Path:        routePath,
		Handler:     handlerName,
		RateLimited: isRateLimitedHandler(handlerExpr),
	}, true
}

type handlerAnalysis struct {
	Base    authMode
	Callees map[string]struct{}
}

func classifyAPILiftHandlerAuthModes(repoRoot string) (map[string]authMode, error) {
	analyses, err := loadHandlerAnalyses(repoRoot)
	if err != nil {
		return nil, err
	}
	return propagateAuthModes(analyses), nil
}

func loadHandlerAnalyses(repoRoot string) (map[string]handlerAnalysis, error) {
	dir := filepath.Join(repoRoot, "cmd", "api", "lift")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	fset := token.NewFileSet()
	analyses := map[string]handlerAnalysis{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		fileAnalyses, err := loadHandlerAnalysesFromFile(fset, filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		for name, analysis := range fileAnalyses {
			analyses[name] = mergeHandlerAnalysis(analyses[name], analysis)
		}
	}
	return analyses, nil
}

func loadHandlerAnalysesFromFile(fset *token.FileSet, path string) (map[string]handlerAnalysis, error) {
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	analyses := map[string]handlerAnalysis{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Body == nil {
			continue
		}
		if !isHandlerReceiver(fn.Recv) {
			continue
		}

		name := fn.Name.Name
		mode, callees := analyzeHandlerAuth(fn)
		analyses[name] = handlerAnalysis{Base: mode, Callees: callees}
	}

	return analyses, nil
}

func mergeHandlerAnalysis(existing handlerAnalysis, next handlerAnalysis) handlerAnalysis {
	if authPriority(next.Base) > authPriority(existing.Base) {
		existing.Base = next.Base
	}
	if existing.Callees == nil {
		existing.Callees = map[string]struct{}{}
	}
	for callee := range next.Callees {
		existing.Callees[callee] = struct{}{}
	}
	return existing
}

func propagateAuthModes(analyses map[string]handlerAnalysis) map[string]authMode {
	modes := map[string]authMode{}
	for name, analysis := range analyses {
		modes[name] = analysis.Base
	}

	changed := true
	for changed {
		changed = false
		for name, analysis := range analyses {
			current := modes[name]
			best := current
			for callee := range analysis.Callees {
				calleeMode, ok := modes[callee]
				if !ok {
					continue
				}
				if authPriority(calleeMode) > authPriority(best) {
					best = calleeMode
				}
			}
			if best != current {
				modes[name] = best
				changed = true
			}
		}
	}

	return modes
}

func isHandlerReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}
	field := recv.List[0]
	switch t := field.Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name == "Handler"
		}
	case *ast.Ident:
		return t.Name == "Handler"
	}
	return false
}

type handlerAuthAnalyzer struct {
	hasRequiredAuth      bool
	hasOptionalAuth      bool
	requiredFromAuthVars bool
	bearerErrVars        map[string]struct{}
	authUserVars         map[string]struct{}
	callees              map[string]struct{}
}

func newHandlerAuthAnalyzer() *handlerAuthAnalyzer {
	return &handlerAuthAnalyzer{
		bearerErrVars: map[string]struct{}{},
		authUserVars:  map[string]struct{}{},
		callees:       map[string]struct{}{},
	}
}

func analyzeHandlerAuth(fn *ast.FuncDecl) (authMode, map[string]struct{}) {
	if fn == nil || fn.Body == nil {
		return authModePublic, nil
	}

	analyzer := newHandlerAuthAnalyzer()
	ast.Inspect(fn.Body, analyzer.visit)
	return analyzer.result()
}

func (a *handlerAuthAnalyzer) visit(n ast.Node) bool {
	switch v := n.(type) {
	case *ast.AssignStmt:
		a.handleAssign(v)
	case *ast.CallExpr:
		a.handleCall(v)
	case *ast.IfStmt:
		a.handleIf(v)
	}
	return true
}

func (a *handlerAuthAnalyzer) result() (authMode, map[string]struct{}) {
	if a.hasRequiredAuth || a.requiredFromAuthVars {
		return authModeBearerRequired, a.callees
	}
	if a.hasOptionalAuth {
		return authModeBearerOptional, a.callees
	}
	return authModePublic, a.callees
}

func (a *handlerAuthAnalyzer) handleAssign(stmt *ast.AssignStmt) {
	if stmt == nil {
		return
	}
	a.trackExtractBearerTokenErr(stmt)
	a.trackAuthenticatedUserLiftAssign(stmt)
}

func (a *handlerAuthAnalyzer) trackExtractBearerTokenErr(stmt *ast.AssignStmt) {
	if stmt == nil || len(stmt.Lhs) < 2 || len(stmt.Rhs) != 1 {
		return
	}
	if !isCallToAuthExtractBearerToken(stmt.Rhs[0]) {
		return
	}
	ident, ok := stmt.Lhs[len(stmt.Lhs)-1].(*ast.Ident)
	if ok && ident.Name != "" && ident.Name != "_" {
		a.bearerErrVars[ident.Name] = struct{}{}
	}
}

func (a *handlerAuthAnalyzer) trackAuthenticatedUserLiftAssign(stmt *ast.AssignStmt) {
	if stmt == nil {
		return
	}

	for i, rhs := range stmt.Rhs {
		call, ok := rhs.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			continue
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok || recv.Name != "h" || sel.Sel.Name == "" {
			continue
		}
		a.callees[sel.Sel.Name] = struct{}{}
		if sel.Sel.Name != "getAuthenticatedUserLift" || i >= len(stmt.Lhs) {
			continue
		}
		if ident, ok := stmt.Lhs[i].(*ast.Ident); ok && ident.Name != "" {
			a.authUserVars[ident.Name] = struct{}{}
		}
	}
}

func (a *handlerAuthAnalyzer) handleCall(call *ast.CallExpr) {
	if call == nil {
		return
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok || recv.Name != "h" || sel.Sel.Name == "" {
		return
	}

	a.callees[sel.Sel.Name] = struct{}{}
	switch sel.Sel.Name {
	case "authenticateUser", "authenticateUserWithWriteScope", "authenticateAdminRequest", "authenticateWithScope":
		a.hasRequiredAuth = true
	case "authenticateUserOptional", "getOptionalAuthenticatedUser", "getAuthenticatedUserLift":
		a.hasOptionalAuth = true
	}
}

func (a *handlerAuthAnalyzer) handleIf(stmt *ast.IfStmt) {
	if stmt == nil {
		return
	}
	a.detectBearerAuthRequirement(stmt)
	a.detectAuthUserRequired(stmt)
}

func (a *handlerAuthAnalyzer) detectBearerAuthRequirement(stmt *ast.IfStmt) {
	if a.hasRequiredAuth {
		return
	}
	if stmt == nil {
		return
	}

	cond, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.NEQ {
		return
	}
	errIdent, ok := cond.X.(*ast.Ident)
	if !ok || errIdent.Name == "" {
		return
	}
	if _, ok := a.bearerErrVars[errIdent.Name]; !ok {
		return
	}
	if !isNilIdent(cond.Y) {
		return
	}
	if ifBodyReturnsUnauthorized(stmt.Body) {
		a.hasRequiredAuth = true
	}
}

func (a *handlerAuthAnalyzer) detectAuthUserRequired(stmt *ast.IfStmt) {
	if a.requiredFromAuthVars || len(a.authUserVars) == 0 || stmt == nil {
		return
	}

	if a.detectRequiredParamAuthUser(stmt) {
		a.requiredFromAuthVars = true
		return
	}
	if a.detectEmptyAuthUser(stmt) {
		a.requiredFromAuthVars = true
	}
}

func (a *handlerAuthAnalyzer) detectRequiredParamAuthUser(stmt *ast.IfStmt) bool {
	init, ok := stmt.Init.(*ast.AssignStmt)
	if !ok || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return false
	}
	errIdent, ok := init.Lhs[0].(*ast.Ident)
	if !ok || errIdent.Name != "err" {
		return false
	}
	call, ok := init.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok || recv.Name != "common" || sel.Sel.Name != "ValidateRequiredParam" || len(call.Args) < 2 {
		return false
	}
	argIdent, ok := call.Args[1].(*ast.Ident)
	if !ok {
		return false
	}
	if _, ok := a.authUserVars[argIdent.Name]; !ok {
		return false
	}
	return ifBodyReturnsUnauthorized(stmt.Body)
}

func (a *handlerAuthAnalyzer) detectEmptyAuthUser(stmt *ast.IfStmt) bool {
	cond, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.EQL {
		return false
	}
	ident, ok := cond.X.(*ast.Ident)
	if !ok {
		return false
	}
	if _, ok := a.authUserVars[ident.Name]; !ok {
		return false
	}
	if !isEmptyStringLiteral(cond.Y) {
		return false
	}
	return ifBodyReturnsUnauthorized(stmt.Body)
}

func isCallToAuthExtractBearerToken(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "ExtractBearerToken" {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "auth"
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

func ifBodyReturnsUnauthorized(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}

	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return true
		}

		if recv, ok := sel.X.(*ast.Ident); ok {
			if recv.Name == "common" && sel.Sel.Name == "RespondUnauthorized" {
				found = true
				return false
			}
			if recv.Name == "ctx" && sel.Sel.Name == "Status" && len(call.Args) >= 1 {
				if isHTTPStatusUnauthorized(call.Args[0]) {
					found = true
					return false
				}
			}
		}

		return true
	})
	return found
}

func isHTTPStatusUnauthorized(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.INT && strings.TrimSpace(v.Value) == "401" {
			return true
		}
	case *ast.SelectorExpr:
		if v.Sel == nil {
			return false
		}
		if ident, ok := v.X.(*ast.Ident); ok && ident.Name == "http" && v.Sel.Name == "StatusUnauthorized" {
			return true
		}
	}
	return false
}

func isEmptyStringLiteral(expr ast.Expr) bool {
	value, ok := evalStringLiteral(expr)
	return ok && value == ""
}

func evalStringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	parsed, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return parsed, true
}

func extractHandlerName(expr ast.Expr) (string, bool) {
	switch v := expr.(type) {
	case *ast.FuncLit:
		return "inline", true
	case *ast.Ident:
		return v.Name, true
	case *ast.SelectorExpr:
		return v.Sel.Name, true
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok && sel.Sel != nil {
			if recv, ok := sel.X.(*ast.Ident); ok {
				if recv.Name == "ratelimit" && sel.Sel.Name == "ApplyRateLimit" && len(v.Args) > 0 {
					return extractHandlerName(v.Args[0])
				}
				if recv.Name == "lift" && sel.Sel.Name == "HandlerFunc" && len(v.Args) > 0 {
					return extractHandlerName(v.Args[0])
				}
			}
		}
	}
	return "", false
}

func isRateLimitedHandler(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "ratelimit" && sel.Sel.Name == "ApplyRateLimit"
}

func applyAuthOverrides(method, path, handler, lambda string, current authMode) authMode {
	normPath := normalizePath(path)

	switch normPath {
	case "/setup/admin":
		return authModeSetupBearer
	case "/setup/finalize":
		return authModeBearerRequired
	}

	if strings.HasPrefix(normPath, "/api/v1/admin/") {
		return authModeBearerRequired
	}

	if strings.HasPrefix(normPath, "/api/v1/auth/webauthn/register") ||
		strings.HasPrefix(normPath, "/api/v1/auth/webauthn/credentials") {
		return authModeBearerRequired
	}

	if lambda == lambdaSSE {
		if normPath == pathStreamingRoot || normPath == pathStreamingHealth {
			return authModePublic
		}
		return authModeBearerRequired
	}

	if lambda == lambdaGraphQL && strings.HasPrefix(normPath, pathGraphQL) {
		return authModeBearerOptional
	}

	if strings.HasPrefix(normPath, "/auth/wallet/") {
		switch normPath {
		case "/auth/wallet/unlink/{address}", "/auth/wallet/list":
			return authModeBearerRequired
		case "/auth/wallet/link":
			return authModeBearerOptional
		default:
			return authModePublic
		}
	}

	_ = method
	_ = handler
	return current
}

func findInventoryComposite(file *ast.File) (*ast.CompositeLit, error) {
	if file == nil {
		return nil, errors.New("inventory file is nil")
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valSpec.Names {
				if name == nil || name.Name != "LambdaInventory" {
					continue
				}
				if i >= len(valSpec.Values) {
					continue
				}
				if lit := unwrapComposite(valSpec.Values[i]); lit != nil {
					return lit, nil
				}
			}
		}
	}
	return nil, errors.New("LambdaInventory variable not found")
}

func unwrapComposite(expr ast.Expr) *ast.CompositeLit {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		return v
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			if lit, ok := v.X.(*ast.CompositeLit); ok {
				return lit
			}
		}
	}
	return nil
}

func findCompositeField(lit *ast.CompositeLit, field string) (*ast.CompositeLit, error) {
	value, ok := findKeyValueExpr(lit, field)
	if !ok {
		return nil, fmt.Errorf("missing %s field", field)
	}
	comp := unwrapComposite(value)
	if comp == nil {
		return nil, fmt.Errorf("%s field is not a composite literal", field)
	}
	return comp, nil
}

func findCompositeFieldOptional(lit *ast.CompositeLit, field string) (*ast.CompositeLit, error) {
	value, ok := findKeyValueExpr(lit, field)
	if !ok {
		return nil, nil
	}
	comp := unwrapComposite(value)
	if comp == nil {
		return nil, fmt.Errorf("%s field is not a composite literal", field)
	}
	return comp, nil
}

func findKeyValueExpr(lit *ast.CompositeLit, field string) (ast.Expr, bool) {
	if lit == nil {
		return nil, false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok || keyIdent.Name != field {
			continue
		}
		return kv.Value, true
	}
	return nil, false
}

func findStringField(lit *ast.CompositeLit, field string) (string, bool) {
	value, ok := findKeyValueExpr(lit, field)
	if !ok {
		return "", false
	}

	switch v := value.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		parsed, err := strconv.Unquote(v.Value)
		if err != nil {
			return "", false
		}
		return parsed, true
	}
	return "", false
}
