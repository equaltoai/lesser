package main

import (
	"strconv"
	"strings"
)

var cursorPaginationQueryParams = map[string]struct{}{
	"max_id":   {},
	"since_id": {},
	"min_id":   {},
}

func ensureStandardResponseHeaders(op *operation, route routeDef) {
	if op == nil || op.Responses == nil {
		return
	}

	status := primarySuccessStatus(route)
	if status == 0 {
		return
	}

	respKey := strconv.Itoa(status)
	resp, ok := op.Responses[respKey]
	if !ok || resp.Ref != "" {
		return
	}

	if resp.Headers == nil {
		resp.Headers = map[string]responseHeader{}
	}

	if isCursorPaginated(route) {
		ensureHeader(resp.Headers, "Link", responseHeader{
			Description: "RFC 8288 pagination links (e.g., `rel=\"next\"`, `rel=\"prev\"`).",
			Schema:      schemaRef{Type: "string"},
		})
	}

	if route.RateLimited {
		ensureHeader(resp.Headers, "X-RateLimit-Limit", responseHeader{
			Description: "Request limit per window.",
			Schema:      schemaRef{Type: "integer", Format: "int32"},
		})
		ensureHeader(resp.Headers, "X-RateLimit-Remaining", responseHeader{
			Description: "Requests remaining in the current window.",
			Schema:      schemaRef{Type: "integer", Format: "int32"},
		})
		ensureHeader(resp.Headers, "X-RateLimit-Reset", responseHeader{
			Description: "Unix timestamp (seconds) when the current window resets.",
			Schema:      schemaRef{Type: "integer", Format: "int64"},
		})
	}

	if route.Path == "/api/v1/apps" || route.Path == "/oauth/register" {
		ensureHeader(resp.Headers, "Deprecation", responseHeader{
			Description: "Present when legacy connector-era MCP registration semantics are used. Runtime value is `true`.",
			Schema:      schemaRef{Type: "string"},
		})
		ensureHeader(resp.Headers, "Warning", responseHeader{
			Description: "HTTP warning emitted when legacy `client_class=agent` or `agent_username` registration semantics are used.",
			Schema:      schemaRef{Type: "string"},
		})
	}

	op.Responses[respKey] = resp
}

func ensureHeader(headers map[string]responseHeader, name string, header responseHeader) {
	if headers == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}

	if _, ok := headers[name]; ok {
		return
	}
	headers[name] = header
}

func isCursorPaginated(route routeDef) bool {
	for _, q := range route.QueryParams {
		q = strings.ToLower(strings.TrimSpace(q))
		if q == "" {
			continue
		}
		if _, ok := cursorPaginationQueryParams[q]; ok {
			return true
		}
	}
	return false
}
