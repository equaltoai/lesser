//nolint:goconst // OpenAPI operation override literals are clearer inline.
package main

import "strings"

func applyOperationOverrides(op *operation, route routeDef) {
	if op == nil {
		return
	}

	applySSEOverrides(op, route)
	applyGraphQLOverrides(op, route)
	applyOAuthOverrides(op, route)
	applyAppRegistrationOverrides(op, route)
	applyMediaOverrides(op, route)
	applySoulOverrides(op, route)
	applySkillOverrides(op, route)
	applyStatusOverrides(op, route)
	applyQuoteOverrides(op, route)
	applyActAsOverrides(op, route)
}

// actAsEnabledRoutes are the REST endpoints that honor the X-Lesser-Act-As
// share-grant act-as indicator (docs/contracts/agent-share-act-as.md). The
// header is ignored everywhere else.
var actAsEnabledRoutes = map[string]map[string]bool{
	"/api/v1/statuses":                    {"POST": true},
	"/api/v1/statuses/{id}/favourite":     {"POST": true},
	"/api/v1/statuses/{id}/unfavourite":   {"POST": true},
	"/api/v1/statuses/{id}/reblog":        {"POST": true},
	"/api/v1/statuses/{id}/unreblog":      {"POST": true},
	"/api/v1/notifications":               {"GET": true},
	"/api/v1/notifications/{id}":          {"GET": true},
	"/api/v1/notifications/clear":         {"POST": true},
	"/api/v1/notifications/{id}/dismiss":  {"POST": true},
	"/api/v1/timelines/home":              {"GET": true},
	"/api/v1/accounts/verify_credentials": {"GET": true},
	"/api/v1/conversations":               {"GET": true},
	"/api/v1/conversations/{id}":          {"GET": true},
	"/api/v1/conversations/lookup":        {"GET": true},
}

func applyActAsOverrides(op *operation, route routeDef) {
	methods, ok := actAsEnabledRoutes[route.Path]
	if !ok || !methods[strings.ToUpper(route.Method)] {
		return
	}
	ensureQueryParam(op, parameter{
		Name:        "X-Lesser-Act-As",
		In:          "header",
		Description: "Optional share-grant act-as indicator: the plain local username of a shared agent. When present and an active (agent, caller) share grant exists, the request acts agent-scoped with the real caller recorded as actedBy attribution (docs/contracts/agent-share-act-as.md). Malformed indicators fail 400, missing/inactive grants fail 403, and grant-check errors fail closed 500.",
		Schema:      schemaRef{Type: "string"},
	})
}

func applyQuoteOverrides(op *operation, route routeDef) {
	if op.Responses == nil {
		op.Responses = map[string]response{}
	}

	switch {
	case route.Method == methodPOST && route.Path == "/api/v1/statuses/{id}/quote":
		delete(op.Responses, "501")
		ensureJSONResponseSchema(op, "200", "QuoteStatusSummary")
		ensureResponseRef(op.Responses, "404", "NotFound")
		ensureResponseRef(op.Responses, "422", "UnprocessableEntity")
		ensureResponseRef(op.Responses, "500", "InternalServerError")
	case route.Method == methodGET && route.Path == "/api/v1/statuses/{id}/quotes":
		delete(op.Responses, "501")
		ensureJSONResponseSchema(op, "200", "QuoteStatusSummaryList")
		ensureResponseRef(op.Responses, "404", "NotFound")
		ensureResponseRef(op.Responses, "422", "UnprocessableEntity")
		ensureResponseRef(op.Responses, "500", "InternalServerError")
		replaceQueryParam(op, parameter{
			Name:        "offset",
			In:          "query",
			Required:    false,
			Description: "Visible-quote offset. Must not exceed (4 × the requested limit) - 1; larger offsets return 422 rather than a silently truncated empty page.",
			Schema: schemaRef{
				Type:    "integer",
				Format:  "int32",
				Minimum: intPtr(0),
			},
		})
	case route.Method == methodGET && route.Path == "/api/v1/accounts/{id}/quote_permissions":
		delete(op.Responses, "501")
		ensureJSONResponseSchema(op, "200", "QuotePermissionsResponse")
		ensureResponseRef(op.Responses, "404", "NotFound")
		ensureResponseRef(op.Responses, "500", "InternalServerError")
	case route.Method == methodPUT && route.Path == "/api/v1/accounts/quote_permissions":
		delete(op.Responses, "501")
		ensureJSONResponseSchema(op, "200", "QuotePermissionsResponse")
		ensureResponseRef(op.Responses, "500", "InternalServerError")
	}
}

func applySkillOverrides(op *operation, route routeDef) {
	if op == nil {
		return
	}
	if route.Path == "/api/v1/skills/catalog" && route.Method == methodGET {
		ensureQueryParam(op, parameter{
			Name:        "exposure",
			In:          "query",
			Required:    false,
			Description: "Optional approved catalog exposure filter: public, instance, or private. Visibility remains fail-closed for the caller.",
			Schema:      schemaRef{Type: "string"},
		})
		ensureQueryParam(op, parameter{
			Name:        "limit",
			In:          "query",
			Required:    false,
			Description: "Maximum catalog entries to return. Defaults to 25 and is capped at 100.",
			Schema: schemaRef{
				Type:    "integer",
				Default: 25,
				Minimum: intPtr(1),
				Maximum: intPtr(100),
			},
		})
		ensureQueryParam(op, parameter{
			Name:        "cursor",
			In:          "query",
			Required:    false,
			Description: "Opaque pagination cursor returned by a previous catalog response.",
			Schema:      schemaRef{Type: "string"},
		})
	}
	if route.Path == "/api/v1/skills/{skillId}/revisions/{revisionNumber}/bundle" && route.Method == methodGET {
		ensureQueryParam(op, parameter{
			Name:        "include_content",
			In:          "query",
			Required:    false,
			Description: "When true, include inline file content that is present in the approved canonical revision manifest. Lesser never writes files into the caller workspace.",
			Schema: schemaRef{
				Type:    "boolean",
				Default: false,
			},
		})
		if op.Responses == nil {
			op.Responses = map[string]response{}
		}
		ensureResponseRef(op.Responses, "422", "UnprocessableEntity")
	}
	if route.Path == "/api/v1/admin/skills/{skillId}/proposals/{proposalId}/promote" && route.Method == methodPOST {
		if op.Responses == nil {
			op.Responses = map[string]response{}
		}
		ensureResponseRef(op.Responses, "409", "Conflict")
	}
}

func applySoulOverrides(op *operation, route routeDef) {
	if op == nil {
		return
	}

	switch route.Path {
	case "/api/v1/souls/bound/me":
		if route.Method == methodGET {
			applyBoundSoulMeOverride(op)
		}
	case "/api/v1/souls/bound/me/mint-conversations":
		if route.Method == methodGET {
			applyBoundSoulMintConversationsOverride(op)
		}
	case "/api/v1/souls/bound/me/mint-conversations/{conversationId}":
		if route.Method == methodGET {
			applyBoundSoulMintConversationOverride(op)
		}
	case "/api/v1/souls/bindings":
		if route.Method == methodPOST {
			applySoulBindingCreateOverride(op)
		}
	case "/api/v1/souls/bindings/{agentId}":
		if route.Method == methodGET {
			applySoulBindingGetOverride(op)
		}
	}
}

func applyBoundSoulMeOverride(op *operation) {
	op.Description = "Return the active soul identity bound to the authenticated local runtime-agent principal. The endpoint is fail-closed: unbound, inactive, missing host identity, or wrong-domain identities return 404."
	if op.Responses == nil {
		op.Responses = map[string]response{}
	}
	ensureResponseRef(op.Responses, "404", "NotFound")
	ensureResponseRef(op.Responses, "422", "UnprocessableEntity")
}

func applyBoundSoulMintConversationsOverride(op *operation) {
	op.Description = "Return compact private mint-conversation metadata for the authenticated local principal's bound soul. Lesser derives the Host agent ID from the existing bound-self authority and calls lesser-host with instance trust only; caller bearer tokens are never forwarded upstream."
	ensureQueryParam(op, parameter{
		Name:        "limit",
		In:          "query",
		Required:    false,
		Description: "Maximum compact conversations to return. Defaults to 20 and is capped at 50.",
		Schema: schemaRef{
			Type:    "integer",
			Default: 20,
			Minimum: intPtr(1),
			Maximum: intPtr(50),
		},
	})
	ensureSoulMintConversationErrorResponses(op)
}

func applyBoundSoulMintConversationOverride(op *operation) {
	op.Description = "Return one bounded private mint-conversation record for the authenticated local principal's bound soul. Lesser validates the opaque conversation ID, derives the Host agent ID from bound self, and calls lesser-host with instance trust only."
	ensureQueryParam(op, parameter{
		Name:        "conversationId",
		In:          "path",
		Required:    true,
		Description: "Opaque safe mint-conversation identifier.",
		Schema: schemaRef{
			Type:      "string",
			Pattern:   "^[A-Za-z0-9._:-]{1,128}$",
			MinLength: intPtr(1),
			MaxLength: intPtr(128),
		},
	})
	ensureSoulMintConversationErrorResponses(op)
}

func ensureSoulMintConversationErrorResponses(op *operation) {
	if op.Responses == nil {
		op.Responses = map[string]response{}
	}
	ensureResponseRef(op.Responses, "404", "NotFound")
	ensureResponseRef(op.Responses, "409", "Conflict")
	ensureResponseRef(op.Responses, "422", "UnprocessableEntity")
	ensureResponseRef(op.Responses, "429", "TooManyRequests")
	ensureResponseRef(op.Responses, "503", "ServiceUnavailable")
	if _, ok := op.Responses["413"]; !ok {
		op.Responses["413"] = response{
			Description: "Private conversation response too large",
			Content: map[string]mediaType{
				"application/json": {Schema: schemaRef{Ref: "#/components/schemas/Error"}},
			},
		}
	}
}

func applySoulBindingCreateOverride(op *operation) {
	op.Description = "Create or confirm a Lesser-local hosted soul/body binding for body/Ptah. The endpoint is server-to-server only: it requires the dedicated soul-binding integration bearer, rejects user OAuth tokens, refetches Host source truth, and writes only through Lesser's SOUL_BODY_BINDING repository path."
	ensureQueryParam(op, parameter{
		Name:        "Idempotency-Key",
		In:          "header",
		Required:    true,
		Description: "Caller-scoped idempotency key for this binding attempt. Reusing the same key with a different canonical payload returns SOUL_BINDING_IDEMPOTENCY_MISMATCH.",
		Schema: schemaRef{
			Type:      "string",
			MinLength: intPtr(1),
			MaxLength: intPtr(200),
		},
	})
	ensureSoulBindingErrorResponses(op)
	ensureJSONResponseSchema(op, "200", "SoulBindingResponse")
}

func applySoulBindingGetOverride(op *operation) {
	op.Description = "Return the authenticated body/Ptah status projection for a Lesser-local hosted soul/body binding. Lesser fails closed if the local binding exists but Host source truth cannot currently prove the hosted active identity."
	ensureQueryParam(op, parameter{
		Name:        "actor_username",
		In:          "query",
		Required:    false,
		Description: "Optional local actor username assertion. A mismatch with the stored binding returns SOUL_BINDING_ACTOR_MISMATCH.",
		Schema: schemaRef{
			Type:      "string",
			MinLength: intPtr(1),
			MaxLength: intPtr(64),
		},
	})
	ensureSoulBindingErrorResponses(op)
	ensureJSONResponseSchema(op, "200", "SoulBindingResponse")
}

func ensureSoulBindingErrorResponses(op *operation) {
	if op.Responses == nil {
		op.Responses = map[string]response{}
	}
	ensureResponseRef(op.Responses, "404", "NotFound")
	ensureResponseRef(op.Responses, "409", "Conflict")
	ensureResponseRef(op.Responses, "422", "UnprocessableEntity")
	ensureResponseRef(op.Responses, "503", "ServiceUnavailable")
}

func applyGraphQLOverrides(op *operation, route routeDef) {
	if route.Path != pathGraphQL {
		return
	}

	if op.Responses == nil {
		op.Responses = map[string]response{}
	}

	switch route.Method {
	case methodGET:
		ensureQueryParam(op, parameter{
			Name:        "query",
			In:          "query",
			Required:    true,
			Description: "GraphQL query document.",
			Schema:      schemaRef{Type: "string"},
		})
		ensureQueryParam(op, parameter{
			Name:        "variables",
			In:          "query",
			Required:    false,
			Description: "GraphQL variables encoded as a JSON string.",
			Schema:      schemaRef{Type: "string"},
		})
		ensureQueryParam(op, parameter{
			Name:        "operationName",
			In:          "query",
			Required:    false,
			Description: "GraphQL operation name (optional).",
			Schema:      schemaRef{Type: "string"},
		})

		ensureJSONResponseSchema(op, "200", "GraphQLResponse")
	case methodPOST:
		op.RequestBody = &requestBody{
			Required: true,
			Content: map[string]mediaType{
				"application/json": {Schema: schemaRef{Ref: "#/components/schemas/GraphQLRequest"}},
			},
		}
		ensureJSONResponseSchema(op, "200", "GraphQLResponse")
	}
}

func applyOAuthOverrides(op *operation, route routeDef) {
	switch {
	case route.Method == methodPOST && route.Path == "/oauth/token":
		applyOAuthTokenOverrides(op)
	case route.Method == methodPOST && route.Path == "/oauth/device/code":
		applyOAuthDeviceCodeOverrides(op)
	case route.Method == methodPOST && route.Path == "/oauth/device/verify":
		applyOAuthDeviceVerifyOverrides(op)
	case route.Method == methodPOST && route.Path == "/oauth/device/consent":
		applyOAuthDeviceConsentOverrides(op)
	case route.Method == methodPOST && route.Path == "/oauth/consent":
		applyOAuthConsentOverrides(op)
	case route.Method == methodPOST && route.Path == "/oauth/register":
		applyOAuthDynamicRegistrationOverrides(op)
	case route.Method == methodGET && route.Path == "/oauth/authorize":
		applyOAuthAuthorizeOverrides(op)
	case route.Method == methodGET && route.Path == "/oauth/login":
		applyOAuthLoginOverrides(op)
	}
}

func applyStatusOverrides(op *operation, route routeDef) {
	if op == nil {
		return
	}

	if route.Method != methodPOST || route.Path != "/api/v1/statuses" {
		return
	}

	op.Description = "Create a status. `in_reply_to_id` accepts local status IDs and canonical remote status URLs. When a canonical remote URL is not yet materialized locally, Lesser performs request-scoped remote parent acquisition on the write path only. Direct replies continue through the conversations service. Self-directed direct posts are intentionally rejected with `DIRECT_SELF_POST_NOT_ALLOWED`. Failure classes distinguish invalid references (400), acquisition timeouts (408), fetched-but-unusable parents (422), and upstream unavailability (503)."
	if op.Responses == nil {
		op.Responses = map[string]response{}
	}

	ensureResponseRef(op.Responses, "408", "RequestTimeout")
	ensureResponseRef(op.Responses, "503", "ServiceUnavailable")
}

func applyOAuthTokenOverrides(op *operation) {
	if op == nil {
		return
	}

	op.RequestBody = &requestBody{
		Required: true,
		Content: map[string]mediaType{
			"application/x-www-form-urlencoded": {
				Schema: schemaRef{Ref: "#/components/schemas/OAuthTokenRequest"},
			},
		},
	}

	ensureJSONResponseSchema(op, "200", "OAuthTokenResponse")
}

func applyOAuthDeviceCodeOverrides(op *operation) {
	if op == nil {
		return
	}

	op.RequestBody = &requestBody{
		Required: true,
		Content: map[string]mediaType{
			"application/x-www-form-urlencoded": {
				Schema: schemaRef{Ref: "#/components/schemas/OAuthDeviceCodeRequest"},
			},
		},
	}

	ensureJSONResponseSchema(op, "200", "OAuthDeviceCodeResponse")
}

func applyOAuthDeviceVerifyOverrides(op *operation) {
	if op == nil {
		return
	}

	op.RequestBody = &requestBody{
		Required: true,
		Content: map[string]mediaType{
			"application/x-www-form-urlencoded": {
				Schema: schemaRef{Ref: "#/components/schemas/OAuthDeviceVerifyRequest"},
			},
		},
	}

	ensureJSONResponseSchema(op, "200", "OAuthDeviceVerifyResponse")
}

func applyOAuthDeviceConsentOverrides(op *operation) {
	if op == nil {
		return
	}

	op.RequestBody = &requestBody{
		Required: true,
		Content: map[string]mediaType{
			"application/x-www-form-urlencoded": {
				Schema: schemaRef{Ref: "#/components/schemas/OAuthDeviceConsentRequest"},
			},
		},
	}

	ensureJSONResponseSchema(op, "200", "OAuthDeviceConsentResponse")
}

func applyOAuthConsentOverrides(op *operation) {
	if op == nil {
		return
	}

	op.RequestBody = &requestBody{
		Required: true,
		Content: map[string]mediaType{
			"application/x-www-form-urlencoded": {
				Schema: schemaRef{Ref: "#/components/schemas/OAuthConsentRequest"},
			},
		},
	}

	ensureJSONResponseSchema(op, "200", "OAuthConsentResponse")
}

func applyOAuthDynamicRegistrationOverrides(op *operation) {
	if op == nil {
		return
	}

	op.RequestBody = &requestBody{
		Required: true,
		Content: map[string]mediaType{
			"application/json": {
				Schema: schemaRef{Ref: "#/components/schemas/OAuthDynamicClientRegistrationRequest"},
			},
		},
	}

	delete(op.Responses, "200")
	ensureJSONResponseSchema(op, "201", "OAuthDynamicClientRegistrationResponse")
}

func applyOAuthAuthorizeOverrides(op *operation) {
	if op == nil {
		return
	}

	ensureQueryParam(op, parameter{
		Name:        "response_type",
		In:          "query",
		Required:    true,
		Description: "OAuth response type (must be `code`).",
		Schema:      schemaRef{Type: "string"},
	})
	ensureQueryParam(op, parameter{
		Name:        "client_id",
		In:          "query",
		Required:    true,
		Description: "OAuth client identifier.",
		Schema:      schemaRef{Type: "string"},
	})
	ensureQueryParam(op, parameter{
		Name:        "redirect_uri",
		In:          "query",
		Required:    true,
		Description: "OAuth redirect URI (must match registered redirect URI).",
		Schema:      schemaRef{Type: "string", Format: "uri"},
	})
	ensureQueryParam(op, parameter{
		Name:        "resource",
		In:          "query",
		Required:    false,
		Description: "Canonical target resource URI. Required for remote MCP authorization; must be the actor-scoped MCP URL served by this Lesser instance.",
		Schema:      schemaRef{Type: "string", Format: "uri"},
	})
	ensureQueryParam(op, parameter{
		Name:        "scope",
		In:          "query",
		Required:    false,
		Description: "Space-delimited OAuth scope list.",
		Schema:      schemaRef{Type: "string"},
	})
	ensureQueryParam(op, parameter{
		Name:        "state",
		In:          "query",
		Required:    false,
		Description: "Client-provided state value (returned to redirect URI).",
		Schema:      schemaRef{Type: "string"},
	})
	ensureQueryParam(op, parameter{
		Name:        "code_challenge",
		In:          "query",
		Required:    false,
		Description: "PKCE code challenge.",
		Schema:      schemaRef{Type: "string"},
	})
	ensureQueryParam(op, parameter{
		Name:        "code_challenge_method",
		In:          "query",
		Required:    false,
		Description: "PKCE code challenge method (typically `S256`).",
		Schema:      schemaRef{Type: "string"},
	})
	ensureQueryParam(op, parameter{
		Name:        "mode",
		In:          "query",
		Required:    false,
		Description: "When set to `ui` (or when `Accept: application/json` is used), returns a JSON `{ next_url }` payload instead of issuing a 302 redirect.",
		Schema:      schemaRef{Type: "string"},
	})

	ensureJSONResponseSchema(op, "200", "OAuthAuthorizeResponse")
	ensureRedirectResponse(op, "302", "Redirect to auth UI or redirect_uri", "Location")
}

func applyOAuthLoginOverrides(op *operation) {
	if op == nil {
		return
	}

	ensureQueryParam(op, parameter{
		Name:        "auth_request",
		In:          "query",
		Required:    true,
		Description: "JSON-encoded OAuth authorization request payload.",
		Schema:      schemaRef{Type: "string"},
	})
	ensureQueryParam(op, parameter{
		Name:        "return_to",
		In:          "query",
		Required:    false,
		Description: "Path to return to after login (internal use).",
		Schema:      schemaRef{Type: "string"},
	})

	if op.Responses == nil {
		op.Responses = map[string]response{}
	}
	op.Responses["200"] = response{
		Description: "HTML login page",
		Content: map[string]mediaType{
			"text/html": {Schema: schemaRef{Type: "string"}},
		},
	}
}

func applyAppRegistrationOverrides(op *operation, route routeDef) {
	if route.Method != methodPOST || route.Path != pathApps {
		return
	}

	op.RequestBody = &requestBody{
		Required: true,
		Content: map[string]mediaType{
			"application/json": {
				Schema: schemaRef{Ref: "#/components/schemas/AppRegistrationRequest"},
			},
			"application/x-www-form-urlencoded": {
				Schema: schemaRef{Ref: "#/components/schemas/AppRegistrationRequest"},
			},
			"multipart/form-data": {
				Schema: schemaRef{Ref: "#/components/schemas/AppRegistrationRequest"},
			},
		},
	}

	ensureJSONResponseSchema(op, "200", "AppRegistrationResponse")
}

func applyMediaOverrides(op *operation, route routeDef) {
	if op == nil {
		return
	}

	switch {
	case route.Method == methodPOST && route.Path == "/api/v1/media":
		op.RequestBody = &requestBody{
			Required: true,
			Content: map[string]mediaType{
				"multipart/form-data": {
					Schema: schemaRef{
						Type: "object",
						Properties: map[string]schemaRef{
							"file": {
								Type:   "string",
								Format: "binary",
							},
							"description": {Type: "string"},
							"focus":       {Type: "string"},
							"sensitive":   {Type: "boolean"},
							"spoiler_text": {
								Type: "string",
							},
							"media_type": {Type: "string"},
						},
						Required:             []string{"file"},
						AdditionalProperties: false,
					},
				},
			},
		}

		ensureJSONResponseSchema(op, "200", "MediaAttachment")
	case route.Method == methodGET && route.Path == "/api/v1/media/{id}":
		ensureJSONResponseSchema(op, "200", "MediaAttachment")
	case route.Method == methodPUT && route.Path == "/api/v1/media/{id}":
		ensureJSONResponseSchema(op, "200", "MediaAttachment")
	}
}

func applySSEOverrides(op *operation, route routeDef) {
	if route.Lambda != lambdaSSE || route.Method != methodGET {
		return
	}

	if strings.HasPrefix(route.Path, pathStreamingPrefix) {
		if op.Responses == nil {
			op.Responses = map[string]response{}
		}
		op.Responses["200"] = response{
			Description: "SSE stream",
			Content: map[string]mediaType{
				"text/event-stream": {Schema: schemaRef{Type: "string"}},
			},
		}
		return
	}

	if route.Path == pathStreamingRoot {
		if op.Responses == nil {
			op.Responses = map[string]response{}
		}
		delete(op.Responses, "200")
		op.Responses["404"] = response{
			Description: "Not Found",
			Content: map[string]mediaType{
				"text/plain": {Schema: schemaRef{Type: "string"}},
			},
		}
		return
	}

	if route.Path == pathStreamingHealth {
		if op.Responses == nil {
			op.Responses = map[string]response{}
		}
		op.Responses["200"] = response{
			Description: "OK",
			Content: map[string]mediaType{
				"text/plain": {Schema: schemaRef{Type: "string"}},
			},
		}
	}
}

func ensureQueryParam(op *operation, p parameter) {
	if op == nil {
		return
	}
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.In) == "" {
		return
	}

	for _, existing := range op.Parameters {
		if strings.EqualFold(existing.In, p.In) && strings.EqualFold(existing.Name, p.Name) {
			for i := range op.Parameters {
				if strings.EqualFold(op.Parameters[i].In, p.In) && strings.EqualFold(op.Parameters[i].Name, p.Name) {
					op.Parameters[i] = p
					return
				}
			}
			return
		}
	}
	op.Parameters = append(op.Parameters, p)
}

func replaceQueryParam(op *operation, p parameter) {
	if op == nil || strings.TrimSpace(p.Name) == "" || !strings.EqualFold(p.In, "query") {
		return
	}

	parameters := op.Parameters[:0]
	for _, existing := range op.Parameters {
		if strings.EqualFold(existing.In, "query") && strings.EqualFold(existing.Name, p.Name) {
			continue
		}
		if queryName, ok := componentQueryParamNames[componentNameFromRef(existing.Ref)]; ok && strings.EqualFold(queryName, p.Name) {
			continue
		}
		parameters = append(parameters, existing)
	}
	op.Parameters = append(parameters, p)
	sortOperationParameters(op)
}

func ensureRedirectResponse(op *operation, statusCode, description, locationHeader string) {
	if op == nil {
		return
	}
	if op.Responses == nil {
		op.Responses = map[string]response{}
	}
	if _, ok := op.Responses[statusCode]; ok {
		return
	}

	op.Responses[statusCode] = response{
		Description: description,
		Content:     nil,
	}

	if op.Extensions == nil {
		op.Extensions = map[string]any{}
	}

	headersKey := "x-lesser-response-headers"
	headers, _ := op.Extensions[headersKey].(map[string]any)
	if headers == nil {
		headers = map[string]any{}
	}
	headers[statusCode] = []string{locationHeader}
	op.Extensions[headersKey] = headers
}

func ensureJSONResponseSchema(op *operation, statusCode, schemaName string) {
	if op == nil {
		return
	}
	if op.Responses == nil {
		op.Responses = map[string]response{}
	}

	resp := op.Responses[statusCode]
	if strings.TrimSpace(resp.Description) == "" {
		resp.Description = "OK"
	}
	if resp.Content == nil {
		resp.Content = map[string]mediaType{}
	}

	mt := resp.Content["application/json"]
	mt.Schema = schemaRef{Ref: "#/components/schemas/" + schemaName}
	resp.Content["application/json"] = mt
	op.Responses[statusCode] = resp
}

func intPtr(value int) *int {
	return &value
}
