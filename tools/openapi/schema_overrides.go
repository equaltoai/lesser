//nolint:goconst // OpenAPI schema override literals are clearer inline.
package main

func applySchemaOverrides(spec *openAPISpec) {
	if spec == nil || spec.Components.Schemas == nil {
		return
	}

	overrideCreateStatusRequest(spec.Components.Schemas)
	overrideAccountRegistrationRequest(spec.Components.Schemas)
	overrideSetupCreateAdminRequest(spec.Components.Schemas)
	overrideOAuthClientSchemas(spec.Components.Schemas)
	overrideCommunityNoteSchemas(spec.Components.Schemas)
	overrideSoulMintConversationSchemas(spec.Components.Schemas)
}

func overrideAccountRegistrationRequest(schemas map[string]any) {
	overrideSchemaProperty(
		schemas,
		"AccountRegistrationRequest",
		"wallet_challenge_id",
		"Single-use wallet registration proof. Exactly one of `wallet_challenge_id` or `passkey_registration_proof` is required.",
		false,
	)
	overrideSchemaProperty(
		schemas,
		"AccountRegistrationRequest",
		"passkey_registration_proof",
		"Single-use proof emitted by `POST /api/v1/auth/webauthn/signup/finish`. Exactly one of `wallet_challenge_id` or `passkey_registration_proof` is required.",
		false,
	)

	raw, ok := schemas["AccountRegistrationRequest"]
	if !ok {
		return
	}

	schemaMap, ok := raw.(map[string]any)
	if !ok {
		return
	}

	schemaMap["oneOf"] = []any{
		map[string]any{
			"required": []string{"wallet_challenge_id"},
		},
		map[string]any{
			"required": []string{"passkey_registration_proof"},
		},
	}
}

func overrideSetupCreateAdminRequest(schemas map[string]any) {
	overrideSchemaProperty(
		schemas,
		"SetupCreateAdminRequest",
		"passkey_registration_proof",
		"Single-use proof emitted by `POST /api/v1/auth/webauthn/signup/finish`. Exactly one of `wallet` or `passkey_registration_proof` is required.",
		false,
	)

	raw, ok := schemas["SetupCreateAdminRequest"]
	if !ok {
		return
	}

	schemaMap, ok := raw.(map[string]any)
	if !ok {
		return
	}

	schemaMap["required"] = []string{"username"}
	schemaMap["oneOf"] = []any{
		map[string]any{
			"required": []string{"wallet"},
		},
		map[string]any{
			"required": []string{"passkey_registration_proof"},
		},
	}
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
	visibility["description"] = "Status visibility. `direct` creates are 1:1 only: the content must contain exactly one resolvable local or remote @mention, and Lesser serializes that resolved actor into the ActivityPub addressing fields. Stored DM visibility and repair tooling use the addressing fields as the source of truth; content mentions alone do not authorize or backfill participants."
}

func overrideOAuthClientSchemas(schemas map[string]any) {
	overrideSchemaProperty(schemas, "AppRegistrationRequest", "client_class", "Optional Lesser client classification. Public registration accepts `cli` and `web`; `agent` is not accepted on public registration surfaces.", false)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationRequest", "client_class", "Optional Lesser client classification. Public registration accepts `cli` and `web`; `agent` is not accepted on public registration surfaces.", false)
	overrideSchemaProperty(schemas, "OAuthDynamicClientRegistrationRequest", "application_type", "Optional OIDC/SEP-837 client application type accepted for dynamic-registration compatibility. Lesser accepts `native` or `web` and does not persist this as the Lesser client_class.", false)
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

func overrideSoulMintConversationSchemas(schemas map[string]any) {
	for _, schemaName := range []string{"SoulMintConversation", "SoulMintConversationSummary"} {
		mutateSchemaProperty(schemas, schemaName, "agent_id", func(property map[string]any) {
			property["pattern"] = "^0x[0-9a-fA-F]{64}$"
		})
		mutateSchemaProperty(schemas, schemaName, "conversation_id", func(property map[string]any) {
			property["minLength"] = 1
			property["maxLength"] = 128
			property["pattern"] = "^[A-Za-z0-9._:-]{1,128}$"
		})
		mutateSchemaProperty(schemas, schemaName, "status", func(property map[string]any) {
			property["enum"] = []string{"in_progress", "completed", "failed"}
		})
		mutateSchemaProperty(schemas, schemaName, "charged_credits", func(property map[string]any) {
			property["minimum"] = 0
		})
		mutateSchemaProperty(schemas, schemaName, "created_at", func(property map[string]any) {
			property["format"] = "date-time"
		})
		mutateSchemaProperty(schemas, schemaName, "completed_at", func(property map[string]any) {
			property["format"] = "date-time"
		})
	}

	mutateSchemaProperty(schemas, "SoulMintConversationsResponse", "version", func(property map[string]any) {
		property["enum"] = []string{"1"}
	})
	mutateSchemaProperty(schemas, "SoulMintConversationsResponse", "count", func(property map[string]any) {
		property["minimum"] = 0
	})
	mutateSchemaProperty(schemas, "SoulMintConversationsResponse", "limit", func(property map[string]any) {
		property["minimum"] = 1
		property["maximum"] = 50
	})
	mutateSchemaProperty(schemas, "SoulMintConversationResponse", "version", func(property map[string]any) {
		property["enum"] = []string{"1"}
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
