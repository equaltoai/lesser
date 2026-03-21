package main

func applySchemaOverrides(spec *openAPISpec) {
	if spec == nil || spec.Components.Schemas == nil {
		return
	}

	overrideCreateStatusRequest(spec.Components.Schemas)
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
