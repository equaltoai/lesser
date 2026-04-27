package main

func applySchemaOverrides(spec *openAPISpec) {
	if spec == nil || spec.Components.Schemas == nil {
		return
	}

	overrideCreateStatusRequest(spec.Components.Schemas)
	overrideOAuthClientSchemas(spec.Components.Schemas)
	overrideCommunityNoteSchemas(spec.Components.Schemas)
}

func overrideCreateStatusRequest(schemas map[string]any) {
	overrideSchemaProperty(
		schemas,
		"CreateStatusRequest",
		"in_reply_to_id",
		"Reply parent reference. Accepts a local status ID or a canonical remote status URL. Canonical remote URLs are resolved locally first and materialized on the create path when needed. Direct replies remain conversations-owned.",
		false,
	)

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
	visibility["description"] = "Status visibility. `direct` messages must mention exactly one local or remote recipient using an @mention in the status content; group direct messages are not supported in v1."
}

func overrideOAuthClientSchemas(schemas map[string]any) {
	overrideSchemaProperty(schemas, "AppRegistrationRequest", "client_class", "Optional Lesser client classification. Public registration accepts `cli` and `web`; `agent` is not accepted on public registration surfaces.", false)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationRequest", "client_class", "Optional Lesser client classification. Public registration accepts `cli` and `web`; `agent` is not accepted on public registration surfaces.", false)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationResponse", "client_class", "Lesser client classification persisted for the registered public client.", false)
	overrideSchemaProperty(schemas, "OAuthTokenRequest", "resource", "Canonical target resource URI. For remote MCP authorization, this must match the actor-scoped MCP URL used during the authorize request.", false)
}

func overrideCommunityNoteSchemas(schemas map[string]any) {
	mutateSchemaProperty(schemas, "CommunityNoteSource", "url", func(property map[string]any) {
		property["format"] = "uri"
	})
	mutateSchemaProperty(schemas, "CreateCommunityNoteRequest", "content", func(property map[string]any) {
		property["minLength"] = 10
		property["maxLength"] = 500
	})
	mutateSchemaProperty(schemas, "CreateCommunityNoteRequest", "language", func(property map[string]any) {
		property["minLength"] = 2
		property["maxLength"] = 2
	})
	mutateSchemaProperty(schemas, "CreateCommunityNoteRequest", "sources", func(property map[string]any) {
		property["maxItems"] = 5
	})
	mutateSchemaProperty(schemas, "VoteCommunityNoteRequest", "vote_type", func(property map[string]any) {
		property["enum"] = []string{"helpful", "not_helpful", "neutral"}
	})
	mutateSchemaProperty(schemas, "VoteCommunityNoteRequest", "reason", func(property map[string]any) {
		property["maxLength"] = 200
	})
}

func overrideSchemaProperty(schemas map[string]any, schemaName, propertyName, description string, deprecated bool) {
	mutateSchemaProperty(schemas, schemaName, propertyName, func(propertyMap map[string]any) {
		if description != "" {
			propertyMap["description"] = description
		}
		if deprecated {
			propertyMap["deprecated"] = true
		}
	})
}

func mutateSchemaProperty(schemas map[string]any, schemaName, propertyName string, mutate func(map[string]any)) {
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

	if mutate != nil {
		mutate(propertyMap)
	}
}
