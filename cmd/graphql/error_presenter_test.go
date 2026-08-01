package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2"
	"github.com/theory-cloud/tabletheory/v2/pkg/session"
	"github.com/theory-cloud/tabletheory/v2/pkg/testing/fakedb"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"go.uber.org/zap"
)

type duplicateGrantExecutableSchema struct {
	schema     *ast.Schema
	repository *repositories.DraftRepository
	staleGrant *models.DraftReviewGrant
}

func (s *duplicateGrantExecutableSchema) Schema() *ast.Schema {
	return s.schema
}

func (*duplicateGrantExecutableSchema) Complexity(
	context.Context,
	string,
	string,
	int,
	map[string]any,
) (int, bool) {
	return 0, false
}

func (s *duplicateGrantExecutableSchema) Exec(ctx context.Context) graphql.ResponseHandler {
	graphql.AddError(ctx, s.repository.CreateDraftReviewGrant(ctx, s.staleGrant))
	return graphql.OneShot(&graphql.Response{Data: json.RawMessage("null")})
}

func TestGraphQLErrorPresenter_AttachesExtensionsForAppError(t *testing.T) {
	appErr := apperrors.NotFound("thing")

	got := graphQLErrorPresenter(context.Background(), appErr)
	require.NotNil(t, got)
	require.Equal(t, appErr.Message, got.Message)

	code, ok := got.Extensions["code"]
	require.True(t, ok)
	require.Equal(t, string(appErr.Code), code.(string))

	status, ok := got.Extensions["http_status"]
	require.True(t, ok)
	require.Equal(t, appErr.HTTPStatusCode, status.(int))
}

func TestGraphQLDuplicateDraftReviewGrantReturnsConflictCode(t *testing.T) {
	ctx := context.Background()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.DraftReviewGrant{}))
	repository := repositories.NewDraftRepository(db, "test-table", zap.NewNop(), nil)

	staleGrant := &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "draft-1",
		Reviewer:  "reviewer",
		GrantedAt: time.Now().UTC().Add(-time.Minute),
	}
	grant := &models.DraftReviewGrant{
		OwnerID:   staleGrant.OwnerID,
		DraftID:   staleGrant.DraftID,
		Reviewer:  staleGrant.Reviewer,
		GrantedAt: staleGrant.GrantedAt,
	}
	require.NoError(t, repository.CreateDraftReviewGrant(ctx, grant))
	revokedAt := time.Now().UTC()
	grant.RevokedAt = &revokedAt
	require.NoError(t, repository.RevokeDraftReviewGrant(ctx, grant))

	schema := gqlparser.MustLoadSchema(&ast.Source{Input: `
		schema { mutation: Mutation }
		type Mutation { shareDraftForReview: Boolean }
	`})
	server := handler.NewDefaultServer(&duplicateGrantExecutableSchema{
		schema:     schema,
		repository: repository,
		staleGrant: staleGrant,
	})
	server.SetErrorPresenter(graphQLErrorPresenter)

	requestBody := []byte(`{"query":"mutation { shareDraftForReview }"}`)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)

	var payload struct {
		Errors []struct {
			Extensions map[string]any `json:"extensions"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Len(t, payload.Errors, 1)
	require.Equal(t, string(apperrors.CodeConflict), payload.Errors[0].Extensions["code"])
	require.Equal(t, float64(http.StatusConflict), payload.Errors[0].Extensions["http_status"])

	persisted, err := repository.GetDraftReviewGrant(ctx, grant.OwnerID, grant.DraftID, grant.Reviewer)
	require.NoError(t, err)
	require.NotNil(t, persisted.RevokedAt, "the failed duplicate must not resurrect the revoked grant")
	require.Empty(t, persisted.GSI2PK, "the revoked grant must stay out of the reviewer queue")
	require.Empty(t, persisted.GSI2SK, "the revoked grant must stay out of the reviewer queue")
}
