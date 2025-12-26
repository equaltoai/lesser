package main

import "strings"

func cleanupPlaceholderRequestBody(op *operation, route routeDef) {
	if op == nil || op.RequestBody == nil || op.RequestBody.Content == nil {
		return
	}
	if route.Method != methodPOST && route.Method != methodPUT && route.Method != methodPATCH {
		return
	}
	if strings.TrimSpace(route.Path) == "/api/graphql" {
		return
	}

	// If we don't have a typed request schema for this route, remove existing placeholder request bodies
	// so strict verification can ensure we don't ship free-form request schemas by accident.
	if strings.TrimSpace(route.RequestSchema) != "" &&
		route.RequestSchema != schemaAnyObject &&
		route.RequestSchema != schemaAnyArray {
		return
	}

	for _, mt := range op.RequestBody.Content {
		if isGenericSchemaRef(mt.Schema) || shouldReplacePlaceholderSchema(mt.Schema) {
			continue
		}
		// At least one non-placeholder content type exists; keep the request body intact.
		return
	}

	op.RequestBody = nil
}
