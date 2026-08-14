package limits

import (
	"context"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

var depthLimitTestSchema = gqlparser.MustLoadSchema(&ast.Source{
	Name: "depth_limit_test.graphql",
	Input: `
		schema { query: Query }

		type Query {
			me: User
			articles: ArticleConnection!
			federationMap: FederationGraph!
		}

		type User {
			id: ID!
			profile: Profile
			reports: [User!]!
		}

		type Profile {
			avatar: String
		}

		type ArticleConnection {
			edges: [ArticleEdge!]!
			pageInfo: PageInfo!
			totalCount: Int!
		}

		type ArticleEdge {
			node: Article!
			cursor: String!
		}

		type Article {
			id: ID!
			title: String!
			author: Actor!
		}

		type Actor {
			publicKey: PublicKey
		}

		type PublicKey {
			id: ID!
		}

		type PageInfo {
			hasNextPage: Boolean!
		}

		type FederationGraph {
			nodes: [InstanceNode!]!
			edges: [FederationEdge!]!
		}

		type FederationEdge {
			source: String!
			target: String!
		}

		type InstanceNode {
			domain: String!
		}
	`,
})

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
	opCtx := mustNamedOpCtxUnvalidated(t, query, "GetMe")

	// Depth: me (1) -> id/profile (2) -> avatar (3)
	dl := FixedDepthLimit(2)
	require.NoError(t, dl.Validate(nil))
	require.NotNil(t, dl.MutateOperationContext(context.Background(), opCtx))

	dlOK := FixedDepthLimit(3)
	require.NoError(t, dlOK.Validate(nil))
	require.Nil(t, dlOK.MutateOperationContext(context.Background(), opCtx))
}

func TestDepthLimit_ConnectionWrappersAreTransparent(t *testing.T) {
	query := `
		query BodyArticles {
			articles {
				edges {
					node {
						id
						title
					}
					cursor
				}
				pageInfo {
					hasNextPage
				}
			}
		}
	`
	opCtx := mustNamedOpCtx(t, query, "BodyArticles")

	dl := FixedDepthLimit(3)
	require.NoError(t, dl.Validate(nil))
	require.Nil(t, dl.MutateOperationContext(context.Background(), opCtx))

	stats := opCtx.Stats.GetExtension(depthExtension).(*DepthStats)
	require.Equal(t, 3, stats.Depth)
}

func TestDepthLimit_ConnectionWrappersStillBlockDeepSelections(t *testing.T) {
	query := `
		query BodyArticles {
			articles {
				edges {
					node {
						author {
							publicKey {
								id
							}
						}
					}
				}
			}
		}
	`
	opCtx := mustNamedOpCtx(t, query, "BodyArticles")

	dl := FixedDepthLimit(3)
	require.NoError(t, dl.Validate(nil))
	require.NotNil(t, dl.MutateOperationContext(context.Background(), opCtx))

	stats := opCtx.Stats.GetExtension(depthExtension).(*DepthStats)
	require.Equal(t, 4, stats.Depth)
}

func TestDepthLimit_AutomationAdmitsNestedNodeConnection(t *testing.T) {
	query := `
		query BodyArticles {
			articles {
				edges {
					cursor
					node {
						id
						title
						author {
							publicKey {
								id
							}
						}
					}
				}
				pageInfo {
					hasNextPage
				}
			}
		}
	`
	opCtx := mustNamedOpCtx(t, query, "BodyArticles")
	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{IsAgent: true})

	dl := &DepthLimit{
		Func: func(ctx context.Context, _ *graphql.OperationContext) int {
			return RequestDepthLimit(ctx, 12, 12)
		},
	}
	require.NoError(t, dl.Validate(nil))
	require.Nil(t, dl.MutateOperationContext(ctx, opCtx))

	stats := opCtx.Stats.GetExtension(depthExtension).(*DepthStats)
	require.Equal(t, 4, stats.Depth)
}

func TestDepthLimit_AutomationStillRejectsAboveLimit(t *testing.T) {
	opCtx := mustNamedOpCtx(t, nestedUserQuery(13), "DeepReports")
	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{IsAgent: true})

	dl := &DepthLimit{
		Func: func(ctx context.Context, _ *graphql.OperationContext) int {
			return RequestDepthLimit(ctx, 12, 12)
		},
	}
	require.NoError(t, dl.Validate(nil))
	require.NotNil(t, dl.MutateOperationContext(ctx, opCtx))

	stats := opCtx.Stats.GetExtension(depthExtension).(*DepthStats)
	require.Equal(t, 13, stats.Depth)
}

func TestDepthLimit_AutomationAdmitsAtLimit(t *testing.T) {
	opCtx := mustNamedOpCtx(t, nestedUserQuery(12), "DeepReports")
	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{IsAgent: true})

	dl := &DepthLimit{
		Func: func(ctx context.Context, _ *graphql.OperationContext) int {
			return RequestDepthLimit(ctx, 12, 12)
		},
	}
	require.NoError(t, dl.Validate(nil))
	require.Nil(t, dl.MutateOperationContext(ctx, opCtx))

	stats := opCtx.Stats.GetExtension(depthExtension).(*DepthStats)
	require.Equal(t, 12, stats.Depth)
}

func TestDepthLimit_NonRelayEdgesCountTowardDepth(t *testing.T) {
	query := `
		query FederationMap {
			federationMap {
				edges {
					target
				}
			}
		}
	`
	opCtx := mustNamedOpCtx(t, query, "FederationMap")

	dl := FixedDepthLimit(2)
	require.NoError(t, dl.Validate(nil))
	require.NotNil(t, dl.MutateOperationContext(context.Background(), opCtx))

	stats := opCtx.Stats.GetExtension(depthExtension).(*DepthStats)
	require.Equal(t, 3, stats.Depth)
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
	opCtx := mustNamedOpCtxUnvalidated(t, query, "GetMe")

	dl := FixedDepthLimit(1)
	require.NoError(t, dl.Validate(nil))
	_ = dl.MutateOperationContext(context.Background(), opCtx)
}

func TestDepthLimit_ExtensionNameAndValidateNilFunc(t *testing.T) {
	require.Equal(t, depthExtension, FixedDepthLimit(1).ExtensionName())
	require.Error(t, (&DepthLimit{}).Validate(nil))
}

func TestDepthLimit_MutateOperationContext_NilAndMissingOperation(t *testing.T) {
	dl := FixedDepthLimit(1)
	require.NoError(t, dl.Validate(nil))

	require.Nil(t, dl.MutateOperationContext(context.Background(), nil))
	require.Nil(t, dl.MutateOperationContext(context.Background(), &graphql.OperationContext{}))

	opCtx := mustNamedOpCtx(t, `query GetMe { me { id } }`, "GetMe")
	opCtx.Operation = nil
	opCtx.OperationName = "does-not-exist"
	require.Nil(t, dl.MutateOperationContext(context.Background(), opCtx))
}

func TestDepthLimit_GetDepthStats(t *testing.T) {
	opCtx := mustOpCtx(t, `{ me { id } }`)
	ctx := graphql.WithOperationContext(context.Background(), opCtx)
	require.Nil(t, GetDepthStats(ctx))

	dl := FixedDepthLimit(10)
	require.NoError(t, dl.Validate(nil))
	require.Nil(t, dl.MutateOperationContext(context.Background(), opCtx))

	stats := GetDepthStats(ctx)
	require.NotNil(t, stats)
	require.Equal(t, 2, stats.Depth)
	require.Equal(t, 10, stats.DepthLimit)
}

func TestDepthLimit_HelperDepthFunctions_NilAndInlineFragments(t *testing.T) {
	require.Equal(t, defaultIgnoreDepth, calculateOperationDepth(nil, nil))

	doc := &ast.QueryDocument{}
	require.Equal(t, 2, fieldSelectionDepth(doc, &ast.Field{}, 1, map[string]bool{}))
	require.Equal(t, 1, fieldSelectionDepth(doc, nil, 1, map[string]bool{}))

	require.Equal(t, 3, fragmentSpreadDepth(doc, nil, 3, map[string]bool{}))
	require.Equal(t, 3, fragmentSpreadDepth(nil, &ast.FragmentSpread{Name: "A"}, 3, map[string]bool{}))
	require.Equal(t, 3, fragmentSpreadDepth(doc, &ast.FragmentSpread{}, 3, map[string]bool{}))
	require.Equal(t, 3, fragmentSpreadDepth(doc, &ast.FragmentSpread{Name: "Missing"}, 3, map[string]bool{}))

	stack := map[string]bool{"A": true}
	require.Equal(t, 3, fragmentSpreadDepth(doc, &ast.FragmentSpread{Name: "A"}, 3, stack))

	inline := &ast.InlineFragment{SelectionSet: ast.SelectionSet{&ast.Field{}}}
	require.Equal(t, 1, selectionDepth(doc, inline, 0, map[string]bool{}))
}

// nestedUserQuery builds a valid operation over the self-referential
// User.reports field whose selection depth equals depth: me (1) plus
// depth-2 reports levels plus the terminal id (1).
func nestedUserQuery(depth int) string {
	reports := depth - 2
	var b strings.Builder
	b.WriteString("query DeepReports { me ")
	for i := 0; i < reports; i++ {
		b.WriteString("{ reports ")
	}
	b.WriteString("{ id }")
	for i := 0; i < reports; i++ {
		b.WriteString(" }")
	}
	b.WriteString(" }")
	return b.String()
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
	doc, errs := gqlparser.LoadQuery(depthLimitTestSchema, query)
	require.Empty(t, errs)
	return doc
}

func mustParseDocUnvalidated(t *testing.T, query string) *ast.QueryDocument {
	t.Helper()
	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	require.NoError(t, err)
	return doc
}

func mustNamedOpCtxUnvalidated(t *testing.T, query, opName string) *graphql.OperationContext {
	t.Helper()
	doc := mustParseDocUnvalidated(t, query)
	op := doc.Operations.ForName(opName)
	require.NotNil(t, op)
	return &graphql.OperationContext{
		Doc:           doc,
		OperationName: opName,
		Operation:     op,
	}
}
