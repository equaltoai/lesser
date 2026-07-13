package limits

import (
	"context"
	"errors"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	depthExtension     = "DepthLimit"
	errDepthLimitCode  = "DEPTH_LIMIT_EXCEEDED"
	defaultIgnoreDepth = 0
)

// DepthLimit enforces a maximum selection depth for a GraphQL operation.
//
// Depth is defined as the maximum number of nested field selections from the operation root.
// If a query exceeds the configured limit, a 422 status code will be returned by gqlgen.
type DepthLimit struct {
	Func func(ctx context.Context, opCtx *graphql.OperationContext) int
}

var _ interface {
	graphql.OperationContextMutator
	graphql.HandlerExtension
} = &DepthLimit{}

// DepthStats contains computed depth information for the active GraphQL operation.
type DepthStats struct {
	Depth      int
	DepthLimit int
}

// FixedDepthLimit sets a depth limit that does not change.
// A limit <= 0 disables depth enforcement.
func FixedDepthLimit(limit int) *DepthLimit {
	return &DepthLimit{
		Func: func(_ context.Context, _ *graphql.OperationContext) int {
			return limit
		},
	}
}

// ExtensionName implements graphql.HandlerExtension.
func (d DepthLimit) ExtensionName() string {
	return depthExtension
}

// Validate implements graphql.HandlerExtension.
func (d *DepthLimit) Validate(_ graphql.ExecutableSchema) error {
	if d.Func == nil {
		return errors.New("DepthLimit func can not be nil")
	}
	return nil
}

// MutateOperationContext implements graphql.OperationContextMutator.
func (d DepthLimit) MutateOperationContext(ctx context.Context, opCtx *graphql.OperationContext) *gqlerror.Error {
	if opCtx == nil {
		return nil
	}

	limit := d.Func(ctx, opCtx)
	if limit <= 0 {
		return nil
	}

	operation := opCtx.Operation
	if operation == nil && opCtx.Doc != nil {
		operation = opCtx.Doc.Operations.ForName(opCtx.OperationName)
	}
	if operation == nil {
		return nil
	}

	depth := calculateOperationDepth(opCtx.Doc, operation)
	opCtx.Stats.SetExtension(depthExtension, &DepthStats{
		Depth:      depth,
		DepthLimit: limit,
	})

	if depth > limit {
		err := gqlerror.Errorf("operation has depth %d, which exceeds the limit of %d", depth, limit)
		errcode.Set(err, errDepthLimitCode)
		return err
	}

	return nil
}

// GetDepthStats returns DepthStats for the active operation, if available.
func GetDepthStats(ctx context.Context) *DepthStats {
	opCtx := graphql.GetOperationContext(ctx)
	if opCtx == nil {
		return nil
	}

	s, _ := opCtx.Stats.GetExtension(depthExtension).(*DepthStats)
	return s
}

func calculateOperationDepth(doc *ast.QueryDocument, operation *ast.OperationDefinition) int {
	if operation == nil {
		return defaultIgnoreDepth
	}
	return selectionSetDepth(doc, operation.SelectionSet, 0, map[string]bool{})
}

func selectionSetDepth(doc *ast.QueryDocument, selectionSet ast.SelectionSet, currentDepth int, stack map[string]bool) int {
	maxDepth := currentDepth

	for _, selection := range selectionSet {
		depth := selectionDepth(doc, selection, currentDepth, stack)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

func selectionDepth(doc *ast.QueryDocument, selection ast.Selection, currentDepth int, stack map[string]bool) int {
	switch s := selection.(type) {
	case *ast.Field:
		return fieldSelectionDepth(doc, s, currentDepth, stack)
	case *ast.InlineFragment:
		return selectionSetDepth(doc, s.SelectionSet, currentDepth, stack)
	case *ast.FragmentSpread:
		return fragmentSpreadDepth(doc, s, currentDepth, stack)
	default:
		return currentDepth
	}
}

func fieldSelectionDepth(doc *ast.QueryDocument, field *ast.Field, currentDepth int, stack map[string]bool) int {
	if field == nil {
		return currentDepth
	}

	if isTransparentConnectionWrapper(field) {
		return selectionSetDepth(doc, field.SelectionSet, currentDepth, stack)
	}

	nextDepth := currentDepth + 1
	if len(field.SelectionSet) == 0 {
		return nextDepth
	}

	return selectionSetDepth(doc, field.SelectionSet, nextDepth, stack)
}

func isTransparentConnectionWrapper(field *ast.Field) bool {
	if field == nil || len(field.SelectionSet) == 0 || field.ObjectDefinition == nil {
		return false
	}

	parentName := field.ObjectDefinition.Name
	switch field.Name {
	case "edges":
		return strings.HasSuffix(parentName, "Connection") && astDefinitionHasField(field.ObjectDefinition, "pageInfo")
	case "node":
		return strings.HasSuffix(parentName, "Edge") && astDefinitionHasField(field.ObjectDefinition, "cursor")
	default:
		return false
	}
}

func astDefinitionHasField(def *ast.Definition, name string) bool {
	if def == nil {
		return false
	}
	return def.Fields.ForName(name) != nil
}

func fragmentSpreadDepth(doc *ast.QueryDocument, spread *ast.FragmentSpread, currentDepth int, stack map[string]bool) int {
	if spread == nil || doc == nil {
		return currentDepth
	}

	name := spread.Name
	if name == "" {
		return currentDepth
	}

	if stack[name] {
		return currentDepth
	}
	stack[name] = true
	defer delete(stack, name)

	frag := doc.Fragments.ForName(name)
	if frag == nil {
		return currentDepth
	}

	return selectionSetDepth(doc, frag.SelectionSet, currentDepth, stack)
}
