//nolint:goconst // OpenAPI query parameter literals are clearer inline.
package main

import (
	"sort"
	"strings"
)

var componentQueryParamNames = map[string]string{
	"Limit":   "limit",
	"MaxID":   "max_id",
	"SinceID": "since_id",
	"MinID":   "min_id",
	"Page":    "page",
	"Offset":  "offset",
	"Cursor":  "cursor",
}

func ensureInferredQueryParams(op *operation, route routeDef) {
	if op == nil || len(route.QueryParams) == 0 {
		return
	}

	existing := existingQueryParams(op.Parameters)
	for _, name := range route.QueryParams {
		q := strings.TrimSpace(name)
		if q == "" {
			continue
		}
		qLower := strings.ToLower(q)
		if _, ok := existing[qLower]; ok {
			continue
		}

		if ref := componentParameterRefForQueryParam(qLower); ref != "" {
			op.Parameters = append(op.Parameters, parameter{Ref: ref})
			existing[qLower] = struct{}{}
			continue
		}

		op.Parameters = append(op.Parameters, parameter{
			Name:     q,
			In:       "query",
			Required: false,
			Schema:   schemaRef{Type: "string"},
		})
		existing[qLower] = struct{}{}
	}

	sortOperationParameters(op)
}

func existingQueryParams(params []parameter) map[string]struct{} {
	out := map[string]struct{}{}
	for _, p := range params {
		if strings.EqualFold(p.In, "query") && strings.TrimSpace(p.Name) != "" {
			out[strings.ToLower(strings.TrimSpace(p.Name))] = struct{}{}
			continue
		}

		if p.Ref == "" {
			continue
		}
		componentName := componentNameFromRef(p.Ref)
		if componentName == "" {
			continue
		}
		if queryName, ok := componentQueryParamNames[componentName]; ok && queryName != "" {
			out[strings.ToLower(queryName)] = struct{}{}
		}
	}
	return out
}

func componentParameterRefForQueryParam(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	for component, queryName := range componentQueryParamNames {
		if name == queryName {
			return "#/components/parameters/" + component
		}
	}
	return ""
}

func componentNameFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	const prefix = "#/components/parameters/"
	if !strings.HasPrefix(ref, prefix) {
		return ""
	}
	return strings.TrimPrefix(ref, prefix)
}

func sortOperationParameters(op *operation) {
	if op == nil || len(op.Parameters) < 2 {
		return
	}

	sort.Slice(op.Parameters, func(i, j int) bool {
		a := op.Parameters[i]
		b := op.Parameters[j]

		if a.In != b.In {
			return a.In < b.In
		}

		keyA := strings.TrimSpace(a.Name)
		if keyA == "" {
			keyA = strings.TrimSpace(a.Ref)
		}

		keyB := strings.TrimSpace(b.Name)
		if keyB == "" {
			keyB = strings.TrimSpace(b.Ref)
		}

		return keyA < keyB
	})
}
