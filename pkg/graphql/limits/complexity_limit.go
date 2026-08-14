package limits

import (
	"context"
	"errors"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	complexityExtension    = "ComplexityLimit"
	errComplexityLimitCode = "COMPLEXITY_LIMIT_EXCEEDED"
)

// ComplexityLimit enforces a maximum query complexity for a GraphQL operation.
// A per-request limit resolved by Func of <= 0 disables complexity enforcement
// for that request, mirroring DepthLimit's semantics.
type ComplexityLimit struct {
	Func func(ctx context.Context, opCtx *graphql.OperationContext) int

	es graphql.ExecutableSchema
}

var _ interface {
	graphql.OperationContextMutator
	graphql.HandlerExtension
} = &ComplexityLimit{}

// ComplexityStats contains computed complexity information for the active
// GraphQL operation.
type ComplexityStats struct {
	// Complexity is the calculated complexity for this request.
	Complexity int

	// ComplexityLimit is the limit for this request returned by the extension func.
	ComplexityLimit int
}

// ExtensionName implements graphql.HandlerExtension.
func (c ComplexityLimit) ExtensionName() string {
	return complexityExtension
}

// Validate implements graphql.HandlerExtension.
func (c *ComplexityLimit) Validate(schema graphql.ExecutableSchema) error {
	if c.Func == nil {
		return errors.New("ComplexityLimit func can not be nil")
	}
	c.es = schema
	return nil
}

// MutateOperationContext implements graphql.OperationContextMutator.
func (c ComplexityLimit) MutateOperationContext(ctx context.Context, opCtx *graphql.OperationContext) *gqlerror.Error {
	if opCtx == nil || c.es == nil {
		return nil
	}

	limit := c.Func(ctx, opCtx)
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

	complexityCalcs := complexity.Calculate(ctx, c.es, operation, opCtx.Variables)
	opCtx.Stats.SetExtension(complexityExtension, &ComplexityStats{
		Complexity:      complexityCalcs,
		ComplexityLimit: limit,
	})

	if complexityCalcs > limit {
		err := gqlerror.Errorf("operation has complexity %d, which exceeds the limit of %d", complexityCalcs, limit)
		errcode.Set(err, errComplexityLimitCode)
		return err
	}

	return nil
}

// GetComplexityStats returns ComplexityStats for the active operation, if available.
func GetComplexityStats(ctx context.Context) *ComplexityStats {
	opCtx := graphql.GetOperationContext(ctx)
	if opCtx == nil {
		return nil
	}

	s, _ := opCtx.Stats.GetExtension(complexityExtension).(*ComplexityStats)
	return s
}
