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
	overrideSchemaProperty(schemas, "AppRegistrationRequest", "client_class", "Deprecated connector-era MCP compatibility field. New public MCP clients should not send this.", true)
	overrideSchemaProperty(schemas, "AppRegistrationRequest", "agent_username", "Deprecated connector-era MCP compatibility field. New public MCP clients should not send this.", true)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationRequest", "client_class", "Deprecated connector-era MCP compatibility field. New public MCP clients should not send this.", true)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationRequest", "agent_username", "Deprecated connector-era MCP compatibility field. New public MCP clients should not send this.", true)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationResponse", "client_class", "Deprecated connector-era MCP compatibility field.", true)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationResponse", "agent_username", "Deprecated connector-era MCP compatibility field.", true)
	overrideSchemaProperty(schemas, "OAuthTokenRequest", "resource", "Canonical target resource URI. For remote MCP access this is the actor-scoped MCP URL.", false)
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
