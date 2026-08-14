package limits

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

// fakeExecutableSchema is a minimal graphql.ExecutableSchema that uses gqlgen's
// default field pricing (one per scalar/object plus child complexity).
type fakeExecutableSchema struct {
	schema *ast.Schema
}

func (f fakeExecutableSchema) Schema() *ast.Schema { return f.schema }

func (f fakeExecutableSchema) Complexity(context.Context, string, string, int, map[string]any) (int, bool) {
	return 0, false
}

func (f fakeExecutableSchema) Exec(context.Context) graphql.ResponseHandler { return nil }

func TestComplexityLimit_ExtensionNameAndValidateNilFunc(t *testing.T) {
	cl := &ComplexityLimit{Func: func(context.Context, *graphql.OperationContext) int { return 1 }}
	require.Equal(t, complexityExtension, cl.ExtensionName())
	require.Error(t, (&ComplexityLimit{}).Validate(nil))
}

func TestComplexityLimit_DisabledWhenLimitNonPositive(t *testing.T) {
	es := fakeExecutableSchema{schema: depthLimitTestSchema}
	cl := &ComplexityLimit{Func: func(context.Context, *graphql.OperationContext) int { return 0 }}
	require.NoError(t, cl.Validate(es))

	opCtx := mustNamedOpCtx(t, `query GetMe { me { id } }`, "GetMe")
	require.Nil(t, cl.MutateOperationContext(context.Background(), opCtx))
}

func TestComplexityLimit_EnforcesLimit(t *testing.T) {
	es := fakeExecutableSchema{schema: depthLimitTestSchema}

	tooLow := &ComplexityLimit{Func: func(context.Context, *graphql.OperationContext) int { return 1 }}
	require.NoError(t, tooLow.Validate(es))
	require.NotNil(t, tooLow.MutateOperationContext(context.Background(),
		mustNamedOpCtx(t, `query GetMe { me { id } }`, "GetMe")))

	ok := &ComplexityLimit{Func: func(context.Context, *graphql.OperationContext) int { return 2 }}
	require.NoError(t, ok.Validate(es))
	require.Nil(t, ok.MutateOperationContext(context.Background(),
		mustNamedOpCtx(t, `query GetMe { me { id } }`, "GetMe")))
}

func TestComplexityLimit_AutomationProfileIsIndependentOfHuman(t *testing.T) {
	es := fakeExecutableSchema{schema: depthLimitTestSchema}
	// me { id } has default complexity 2, so the configured human limit of 1
	// rejects it while the automation limit admits it.
	cl := &ComplexityLimit{
		Func: func(ctx context.Context, _ *graphql.OperationContext) int {
			return RequestComplexityLimit(ctx, 1, 4000)
		},
	}
	require.NoError(t, cl.Validate(es))

	humanCtx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{})
	require.NotNil(t, cl.MutateOperationContext(humanCtx,
		mustNamedOpCtx(t, `query GetMe { me { id } }`, "GetMe")))

	agentCtx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{IsAgent: true})
	require.Nil(t, cl.MutateOperationContext(agentCtx,
		mustNamedOpCtx(t, `query GetMe { me { id } }`, "GetMe")))
}
