package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/auth"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services"
	storagepkg "github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

const injectedGrantCreateFailure = "injected tabletheory transport failure: private detail"
const injectedGrantUpdateFailure = "injected regrant transport failure: private detail"

type failingGrantCreateDynamo struct {
	*fakedb.Fake
	failGrantCreates    bool
	failGrantUpdate     bool
	conflictGrantUpdate bool
	hideGrantReads      int
	grantCreateFailures int
	grantUpdateFailures int
	grantReadsHidden    int
}

func (d *failingGrantCreateDynamo) UpdateItem(
	ctx context.Context,
	input *dynamodb.UpdateItemInput,
	opts ...func(*dynamodb.Options),
) (*dynamodb.UpdateItemOutput, error) {
	if isDraftReviewGrantKey(input.Key) {
		switch {
		case d.conflictGrantUpdate:
			d.grantUpdateFailures++
			return nil, &types.ConditionalCheckFailedException{}
		case d.failGrantUpdate:
			d.grantUpdateFailures++
			return nil, errors.New(injectedGrantUpdateFailure)
		}
	}
	return d.Fake.UpdateItem(ctx, input, opts...)
}

func (d *failingGrantCreateDynamo) PutItem(
	ctx context.Context,
	input *dynamodb.PutItemInput,
	opts ...func(*dynamodb.Options),
) (*dynamodb.PutItemOutput, error) {
	if d.failGrantCreates && isDraftReviewGrantKey(input.Item) {
		d.grantCreateFailures++
		return nil, errors.New(injectedGrantCreateFailure)
	}
	return d.Fake.PutItem(ctx, input, opts...)
}

func (d *failingGrantCreateDynamo) GetItem(
	ctx context.Context,
	input *dynamodb.GetItemInput,
	opts ...func(*dynamodb.Options),
) (*dynamodb.GetItemOutput, error) {
	if d.hideGrantReads > 0 && isDraftReviewGrantKey(input.Key) {
		d.hideGrantReads--
		d.grantReadsHidden++
		return &dynamodb.GetItemOutput{}, nil
	}
	return d.Fake.GetItem(ctx, input, opts...)
}

func isDraftReviewGrantKey(attributes map[string]types.AttributeValue) bool {
	pk, pkOK := attributes["PK"].(*types.AttributeValueMemberS)
	sk, skOK := attributes["SK"].(*types.AttributeValueMemberS)
	return pkOK && skOK && strings.Contains(pk.Value, "#DRAFT#REVIEW") && strings.HasPrefix(sk.Value, "GRANT#")
}

type draftReviewWireHarness struct {
	server     *handler.Server
	repository *repositories.DraftRepository
}

func newDraftReviewWireHarness(t *testing.T, client *failingGrantCreateDynamo) *draftReviewWireHarness {
	t.Helper()
	ctx := context.Background()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.DraftReviewGrant{}))

	storage, err := factory.NewRepositoryFactory(db, "test-table", zap.NewNop())
	require.NoError(t, err)
	cfg := &appconfig.Config{
		InstanceMode:                 appconfig.InstanceModeCMS,
		CMSLongFormPublishingEnabled: true,
		CMSDraftSystemEnabled:        true,
	}
	registry, err := services.NewRegistry(
		services.WithStorage(storage),
		services.WithPublisher(streaming.NewMockPublisher()),
		services.WithLogger(zap.NewNop()),
		services.WithConfig(&services.ServiceConfig{BaseURL: "https://example.com", Config: cfg}),
	)
	require.NoError(t, err)

	require.NoError(t, storage.Account().CreateAccount(ctx, &storagepkg.Account{User: &storagepkg.User{
		Username: "reviewer",
		Approved: true,
		Role:     "user",
	}}))
	now := time.Now().UTC()
	require.NoError(t, storage.Draft().CreateDraft(ctx, &models.Draft{
		ID:            "draft-1",
		AuthorID:      "owner",
		ContentType:   "Article",
		Content:       "draft body",
		ContentFormat: "markdown",
		Status:        "draft",
		CreatedAt:     now,
		UpdatedAt:     now,
		LastSavedAt:   now,
	}))

	resolver := &graph.Resolver{
		Registry: registry,
		Config:   cfg,
		Storage:  storage,
		Logger:   zap.NewNop(),
	}
	server := handler.NewDefaultServer(graph.NewExecutableSchema(graph.NewConfig(resolver)))
	server.SetErrorPresenter(graphQLErrorPresenter)
	server.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		return next(auth.WithClaims(ctx, &auth.Claims{Username: "owner"}))
	})

	draftRepository, ok := storage.Draft().(*repositories.DraftRepository)
	require.True(t, ok)
	return &draftReviewWireHarness{server: server, repository: draftRepository}
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

func TestGraphQLErrorPresenter_ClassifiesCMSErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCode   apperrors.ErrorCode
		wantStatus int
	}{
		{name: "feature disabled", err: apperrors.NewAppError(apperrors.CodeFeatureDisabled, apperrors.CategoryBusiness, "cms is disabled"), wantCode: apperrors.CodeFeatureDisabled, wantStatus: http.StatusForbidden},
		{name: "not found", err: errors.New("draft review not found"), wantCode: apperrors.CodeNotFound, wantStatus: http.StatusNotFound},
		{name: "forbidden", err: errors.New("insufficient privileges for CMS write"), wantCode: apperrors.CodeForbidden, wantStatus: http.StatusForbidden},
		{name: "validation", err: errors.New("draft id is required"), wantCode: apperrors.CodeValidation, wantStatus: http.StatusBadRequest},
		{name: "typed validation normalized", err: apperrors.NewAppError(apperrors.CodeRequiredFieldMissing, apperrors.CategoryValidation, "title is required"), wantCode: apperrors.CodeValidation, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := graphql.WithPathContext(context.Background(), graphql.NewPathWithField("draft"))
			got := graphQLErrorPresenter(ctx, tt.err)
			require.Equal(t, string(tt.wantCode), got.Extensions["code"])
			require.Equal(t, tt.wantStatus, got.Extensions["http_status"])
		})
	}
}

func TestGraphQLErrorPresenter_DoesNotClassifyNonCMSErrors(t *testing.T) {
	ctx := graphql.WithPathContext(context.Background(), graphql.NewPathWithField("status"))
	got := graphQLErrorPresenter(ctx, errors.New("status not found"))
	require.NotContains(t, got.Extensions, "code")
}

func TestGraphQLDuplicateDraftReviewGrantReturnsConflictCode(t *testing.T) {
	ctx := context.Background()
	client := &failingGrantCreateDynamo{Fake: fakedb.New()}
	harness := newDraftReviewWireHarness(t, client)

	grant := &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "draft-1",
		Reviewer:  "reviewer",
		GrantedAt: time.Now().UTC().Add(-time.Minute),
	}
	require.NoError(t, harness.repository.CreateDraftReviewGrant(ctx, grant))
	revokedAt := time.Now().UTC()
	grant.RevokedAt = &revokedAt
	require.NoError(t, harness.repository.RevokeDraftReviewGrant(ctx, grant))
	client.hideGrantReads = 1

	requestBody := []byte(`{"query":"mutation { shareDraftForReview(draftId: \"draft-1\", reviewer: \"reviewer\") { draftId } }"}`)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	harness.server.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var payload struct {
		Errors []struct {
			Extensions map[string]any `json:"extensions"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Len(t, payload.Errors, 1)
	require.Equal(t, string(apperrors.CodeConflict), payload.Errors[0].Extensions["code"])
	require.Equal(t, float64(http.StatusConflict), payload.Errors[0].Extensions["http_status"])
	require.Equal(t, 1, client.grantReadsHidden, "the create-race fault injection must hide the existing grant once")

	persisted, err := harness.repository.GetDraftReviewGrant(ctx, grant.OwnerID, grant.DraftID, grant.Reviewer)
	require.NoError(t, err)
	require.NotNil(t, persisted.RevokedAt, "the failed duplicate must not resurrect the revoked grant")
	require.Empty(t, persisted.GSI2PK, "the revoked grant must stay out of the reviewer queue")
	require.Empty(t, persisted.GSI2SK, "the revoked grant must stay out of the reviewer queue")
}

func TestGraphQLOwnerCanListDraftReviewAssignments(t *testing.T) {
	ctx := context.Background()
	harness := newDraftReviewWireHarness(t, &failingGrantCreateDynamo{Fake: fakedb.New()})
	require.NoError(t, harness.repository.CreateDraftReviewGrant(ctx, &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "draft-1",
		Reviewer:  "reviewer",
		GrantedAt: time.Now().UTC(),
	}))

	requestBody := []byte(`{"query":"query { myDraftReviews(first: 10) { totalCount edges { node { draftId grant { reviewer { username } } } } } }"}`)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	harness.server.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var payload struct {
		Data struct {
			MyDraftReviews struct {
				TotalCount int `json:"totalCount"`
				Edges      []struct {
					Node struct {
						DraftID string `json:"draftId"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"myDraftReviews"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Empty(t, payload.Errors, response.Body.String())
	require.Equal(t, 1, payload.Data.MyDraftReviews.TotalCount)
	require.Len(t, payload.Data.MyDraftReviews.Edges, 1)
	require.Equal(t, "draft-1", payload.Data.MyDraftReviews.Edges[0].Node.DraftID)
}

func TestGraphQLDraftReviewGrantCreateFailureReturnsInternalCode(t *testing.T) {
	client := &failingGrantCreateDynamo{Fake: fakedb.New()}
	harness := newDraftReviewWireHarness(t, client)
	client.failGrantCreates = true

	requestBody := []byte(`{"query":"mutation { shareDraftForReview(draftId: \"draft-1\", reviewer: \"reviewer\") { draftId } }"}`)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	harness.server.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var payload struct {
		Errors []struct {
			Message    string         `json:"message"`
			Extensions map[string]any `json:"extensions"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Len(t, payload.Errors, 1)
	require.Equal(t, string(apperrors.CodeInternal), payload.Errors[0].Extensions["code"])
	require.Equal(t, float64(http.StatusInternalServerError), payload.Errors[0].Extensions["http_status"])
	require.Equal(t, "Failed to create draft review grant", payload.Errors[0].Message)
	require.NotContains(t, response.Body.String(), injectedGrantCreateFailure)
	require.Equal(t, 1, client.grantCreateFailures, "the transport fault injection must fail the grant create")
}

func TestGraphQLDraftReviewGrantRegrantConflictReturnsConflictCode(t *testing.T) {
	ctx := context.Background()
	client := &failingGrantCreateDynamo{Fake: fakedb.New()}
	harness := newDraftReviewWireHarness(t, client)
	grant := seedRevokedDraftReviewGrant(t, ctx, harness.repository)
	client.conflictGrantUpdate = true

	response := executeShareDraftForReview(t, harness.server)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	payload := decodeDraftReviewWireErrors(t, response)
	require.Len(t, payload.Errors, 1)
	require.Equal(t, string(apperrors.CodeConflict), payload.Errors[0].Extensions["code"])
	require.Equal(t, float64(http.StatusConflict), payload.Errors[0].Extensions["http_status"])
	require.Equal(t, 1, client.grantUpdateFailures, "the optimistic-concurrency fault injection must fail the regrant")

	persisted, err := harness.repository.GetDraftReviewGrant(ctx, grant.OwnerID, grant.DraftID, grant.Reviewer)
	require.NoError(t, err)
	require.NotNil(t, persisted.RevokedAt, "a conflicted regrant must not resurrect the revoked grant")
	require.Empty(t, persisted.GSI2PK)
	require.Empty(t, persisted.GSI2SK)
}

func TestGraphQLDraftReviewGrantRegrantFailureReturnsInternalCode(t *testing.T) {
	ctx := context.Background()
	client := &failingGrantCreateDynamo{Fake: fakedb.New()}
	harness := newDraftReviewWireHarness(t, client)
	grant := seedRevokedDraftReviewGrant(t, ctx, harness.repository)
	client.failGrantUpdate = true

	response := executeShareDraftForReview(t, harness.server)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	payload := decodeDraftReviewWireErrors(t, response)
	require.Len(t, payload.Errors, 1)
	require.Equal(t, string(apperrors.CodeInternal), payload.Errors[0].Extensions["code"])
	require.Equal(t, float64(http.StatusInternalServerError), payload.Errors[0].Extensions["http_status"])
	require.Equal(t, "Failed to create draft review grant", payload.Errors[0].Message)
	require.NotContains(t, response.Body.String(), injectedGrantUpdateFailure)
	require.Equal(t, 1, client.grantUpdateFailures, "the transport fault injection must fail the regrant")

	persisted, err := harness.repository.GetDraftReviewGrant(ctx, grant.OwnerID, grant.DraftID, grant.Reviewer)
	require.NoError(t, err)
	require.NotNil(t, persisted.RevokedAt, "a failed regrant must not resurrect the revoked grant")
}

func seedRevokedDraftReviewGrant(t *testing.T, ctx context.Context, repository *repositories.DraftRepository) *models.DraftReviewGrant {
	t.Helper()
	grant := &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "draft-1",
		Reviewer:  "reviewer",
		GrantedAt: time.Now().UTC().Add(-time.Minute),
	}
	require.NoError(t, repository.CreateDraftReviewGrant(ctx, grant))
	revokedAt := time.Now().UTC()
	grant.RevokedAt = &revokedAt
	require.NoError(t, repository.RevokeDraftReviewGrant(ctx, grant))
	return grant
}

func executeShareDraftForReview(t *testing.T, server *handler.Server) *httptest.ResponseRecorder {
	t.Helper()
	requestBody := []byte(`{"query":"mutation { shareDraftForReview(draftId: \"draft-1\", reviewer: \"reviewer\") { draftId } }"}`)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

type draftReviewWireErrorPayload struct {
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

func decodeDraftReviewWireErrors(t *testing.T, response *httptest.ResponseRecorder) draftReviewWireErrorPayload {
	t.Helper()
	var payload draftReviewWireErrorPayload
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	return payload
}
