package main

import "strings"

func applyOperationOverrides(op *operation, route routeDef) {
	if op == nil {
		return
	}

	applySSEOverrides(op, route)
	applyOAuthOverrides(op, route)
	applyAppRegistrationOverrides(op, route)
	applyMediaOverrides(op, route)
}

func applyOAuthOverrides(op *operation, route routeDef) {
	switch {
	case route.Method == methodPOST && route.Path == "/oauth/token":
		applyOAuthTokenOverrides(op)
	case route.Method == methodPOST && route.Path == "/oauth/consent":
		applyOAuthConsentOverrides(op)
	case route.Method == methodGET && route.Path == "/oauth/authorize":
		applyOAuthAuthorizeOverrides(op)
	case route.Method == methodGET && route.Path == "/oauth/login":
		applyOAuthLoginOverrides(op)
	}
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
	if route.Method != methodPOST || route.Path != "/api/v1/apps" {
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

	if strings.HasPrefix(route.Path, "/api/v1/streaming/") {
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

	if route.Path == "/api/v1/streaming" {
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

	if route.Path == "/api/v1/streaming/health" {
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
			return
		}
	}
	op.Parameters = append(op.Parameters, p)
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
