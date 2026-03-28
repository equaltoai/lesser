package main

func applySchemaOverrides(spec *openAPISpec) {
	if spec == nil || spec.Components.Schemas == nil {
		return
	}

	overrideCreateStatusRequest(spec.Components.Schemas)
	overrideOAuthClientSchemas(spec.Components.Schemas)
}

func overrideCreateStatusRequest(schemas map[string]any) {
	raw, ok := schemas["CreateStatusRequest"]
	if !ok {
		return
	}

	schemaMap, ok := raw.(map[string]any)
	if !ok {
		return
	}

	properties, ok := schemaMap["properties"].(map[string]any)
	if !ok {
		return
	}

	visibility, ok := properties["visibility"].(map[string]any)
	if !ok {
		return
	}

	visibility["enum"] = []string{"public", "unlisted", "private", "direct"}
	visibility["description"] = "Status visibility. `direct` messages must mention exactly one recipient using an @mention in the status content."
}

func overrideOAuthClientSchemas(schemas map[string]any) {
	overrideSchemaProperty(schemas, "AppRegistrationRequest", "client_class", "Optional Lesser client classification. Public registration accepts `cli` and `web`; `agent` is not accepted on public registration surfaces.", false)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationRequest", "client_class", "Optional Lesser client classification. Public registration accepts `cli` and `web`; `agent` is not accepted on public registration surfaces.", false)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationResponse", "client_class", "Lesser client classification persisted for the registered public client.", false)
	overrideSchemaProperty(schemas, "OAuthTokenRequest", "resource", "Canonical target resource URI. For remote MCP authorization, this must match the actor-scoped MCP URL used during the authorize request.", false)
}

func overrideSchemaProperty(schemas map[string]any, schemaName, propertyName, description string, deprecated bool) {
	raw, ok := schemas[schemaName]
	if !ok {
		return
	}

	schemaMap, ok := raw.(map[string]any)
	if !ok {
		return
	}

	properties, ok := schemaMap["properties"].(map[string]any)
	if !ok {
		return
	}

	propertyRaw, ok := properties[propertyName]
	if !ok {
		return
	}

	propertyMap, ok := propertyRaw.(map[string]any)
	if !ok {
		return
	}

	if description != "" {
		propertyMap["description"] = description
	}
	if deprecated {
		propertyMap["deprecated"] = true
	}
}
