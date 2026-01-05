package limits

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

func TestDepthLimit_DisabledWhenLimitNonPositive(t *testing.T) {
	dl := FixedDepthLimit(0)
	require.NoError(t, dl.Validate(nil))

	opCtx := mustOpCtx(t, `{ me { id } }`)
	require.Nil(t, dl.MutateOperationContext(context.Background(), opCtx))
}

func TestDepthLimit_EnforcesDepth(t *testing.T) {
	opCtx := mustOpCtx(t, `{ me { id } }`)

	// Depth: me (1) -> id (2)
	dlTooLow := FixedDepthLimit(1)
	require.NoError(t, dlTooLow.Validate(nil))
	require.NotNil(t, dlTooLow.MutateOperationContext(context.Background(), opCtx))

	dlOK := FixedDepthLimit(2)
	require.NoError(t, dlOK.Validate(nil))
	require.Nil(t, dlOK.MutateOperationContext(context.Background(), opCtx))
}

func TestDepthLimit_CountsFragments(t *testing.T) {
	query := `
		query GetMe {
			me {
				...UserFields
			}
		}
		fragment UserFields on User {
			id
			profile { avatar }
		}
	`
	opCtx := mustNamedOpCtx(t, query, "GetMe")

	// Depth: me (1) -> id/profile (2) -> avatar (3)
	dl := FixedDepthLimit(2)
	require.NoError(t, dl.Validate(nil))
	require.NotNil(t, dl.MutateOperationContext(context.Background(), opCtx))

	dlOK := FixedDepthLimit(3)
	require.NoError(t, dlOK.Validate(nil))
	require.Nil(t, dlOK.MutateOperationContext(context.Background(), opCtx))
}

func TestDepthLimit_FragmentCyclesDoNotPanic(t *testing.T) {
	query := `
		query GetMe {
			me {
				...A
			}
		}
		fragment A on User { ...B }
		fragment B on User { ...A }
	`
	opCtx := mustNamedOpCtx(t, query, "GetMe")

	dl := FixedDepthLimit(1)
	require.NoError(t, dl.Validate(nil))
	_ = dl.MutateOperationContext(context.Background(), opCtx)
}

func mustOpCtx(t *testing.T, query string) *graphql.OperationContext {
	t.Helper()
	doc := mustParseDoc(t, query)
	require.NotEmpty(t, doc.Operations)
	return &graphql.OperationContext{
		Doc:       doc,
		Operation: doc.Operations[0],
	}
}

func mustNamedOpCtx(t *testing.T, query, opName string) *graphql.OperationContext {
	t.Helper()
	doc := mustParseDoc(t, query)
	op := doc.Operations.ForName(opName)
	require.NotNil(t, op)
	return &graphql.OperationContext{
		Doc:           doc,
		OperationName: opName,
		Operation:     op,
	}
}

func mustParseDoc(t *testing.T, query string) *ast.QueryDocument {
	t.Helper()
	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	require.NoError(t, err)
	return doc
}

