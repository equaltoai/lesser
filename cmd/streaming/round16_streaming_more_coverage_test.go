package main

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	testutils "github.com/equaltoai/lesser/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type repositoryStorageWithList struct {
	storagecore.RepositoryStorage
	listRepo *repositories.ListRepository
}

func (r repositoryStorageWithList) List() *repositories.ListRepository { return r.listRepo }

func TestCanonicalizeStreamAlias_Round16(t *testing.T) {
	t.Run("nil connection returns requested stream unchanged", func(t *testing.T) {
		require.Equal(t, " user ", canonicalizeStreamAlias(nil, " user "))
	})

	t.Run("empty stream", func(t *testing.T) {
		require.Equal(t, "", canonicalizeStreamAlias(&models.WebSocketConnection{Username: "alice"}, "  "))
	})

	t.Run("username missing does not rewrite aliases", func(t *testing.T) {
		require.Equal(t, "user", canonicalizeStreamAlias(&models.WebSocketConnection{Username: " "}, " user "))
	})

	t.Run("known aliases canonicalize to per-user streams", func(t *testing.T) {
		conn := &models.WebSocketConnection{Username: " alice "}
		require.Equal(t, streaming.UserStreamName("alice"), canonicalizeStreamAlias(conn, streaming.UserStream))
		require.Equal(t, streaming.UserNotificationStreamName("alice"), canonicalizeStreamAlias(conn, streaming.UserNotificationStream))
		require.Equal(t, streaming.DirectStreamName("alice"), canonicalizeStreamAlias(conn, streaming.DirectStream))
		require.Equal(t, streaming.PublicStream, canonicalizeStreamAlias(conn, " public "))
	})
}

func TestResolveCanonicalStreamSubscription_Round16(t *testing.T) {
	sh := &StreamingHandler{
		logger: zap.NewNop(),
	}

	authed := &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Username: "alice"}
	noAuth := &models.WebSocketConnection{ConnectionID: "c2", Username: "alice"}
	noUsername := &models.WebSocketConnection{ConnectionID: "c3", UserID: "u1"}

	type tc struct {
		name     string
		conn     *models.WebSocketConnection
		stream   string
		want     string
		wantCode apperrors.ErrorCode
	}

	cases := []tc{
		{name: "empty stream", conn: authed, stream: " ", wantCode: apperrors.CodeBadRequest},
		{name: "public ok", conn: authed, stream: streaming.PublicStream, want: streaming.PublicStream},
		{name: "public invalid suffix", conn: authed, stream: "public:unknown", wantCode: apperrors.CodeBadRequest},
		{name: "hashtag missing tag", conn: authed, stream: "hashtag", wantCode: apperrors.CodeBadRequest},
		{name: "hashtag empty tag", conn: authed, stream: "hashtag: ", wantCode: apperrors.CodeBadRequest},
		{name: "hashtag ok", conn: authed, stream: "hashtag:go", want: streaming.HashtagStreamName("go")},
		{name: "user auth required", conn: noAuth, stream: "user", wantCode: apperrors.CodeUnauthorized},
		{name: "user missing username", conn: noUsername, stream: "user", wantCode: apperrors.CodeInternal},
		{name: "user ok", conn: authed, stream: "user", want: streaming.UserStreamName("alice")},
		{name: "user notification ok", conn: authed, stream: "user:notification", want: streaming.UserNotificationStreamName("alice")},
		{name: "user canonical ok", conn: authed, stream: "user:alice", want: streaming.UserStreamName("alice")},
		{name: "user notification canonical ok", conn: authed, stream: "user:notification:alice", want: streaming.UserNotificationStreamName("alice")},
		{name: "user other invalid", conn: authed, stream: "user:bob", wantCode: apperrors.CodeBadRequest},
		{name: "direct auth required", conn: noAuth, stream: "direct", wantCode: apperrors.CodeUnauthorized},
		{name: "direct missing username", conn: noUsername, stream: "direct", wantCode: apperrors.CodeInternal},
		{name: "direct ok", conn: authed, stream: "direct", want: streaming.DirectStreamName("alice")},
		{name: "direct canonical ok", conn: authed, stream: "direct:alice", want: streaming.DirectStreamName("alice")},
		{name: "direct other invalid", conn: authed, stream: "direct:bob", wantCode: apperrors.CodeBadRequest},
		{name: "list auth required", conn: noAuth, stream: "list:1", wantCode: apperrors.CodeUnauthorized},
		{name: "list missing username", conn: noUsername, stream: "list:1", wantCode: apperrors.CodeInternal},
		{name: "list invalid format", conn: authed, stream: "list:", wantCode: apperrors.CodeBadRequest},
		{name: "unknown root invalid", conn: authed, stream: "system", wantCode: apperrors.CodeBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sh.resolveCanonicalStreamSubscription(context.Background(), tc.conn, tc.stream)
			if tc.wantCode != "" {
				require.Empty(t, got)
				require.NotNil(t, err)
				require.Equal(t, tc.wantCode, err.Code)
				return
			}
			require.Nil(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	origAuthorize := authorizeListStreamSubscriptionFn
	t.Cleanup(func() { authorizeListStreamSubscriptionFn = origAuthorize })

	t.Run("list not found maps to invalid stream", func(t *testing.T) {
		authorizeListStreamSubscriptionFn = func(context.Context, storagecore.RepositoryStorage, string, string) error {
			return apperrors.NotFound("list")
		}

		got, err := sh.resolveCanonicalStreamSubscription(context.Background(), authed, "list:1")
		require.Empty(t, got)
		require.NotNil(t, err)
		require.Equal(t, apperrors.CodeBadRequest, err.Code)
	})

	t.Run("list forbidden maps to invalid stream", func(t *testing.T) {
		authorizeListStreamSubscriptionFn = func(context.Context, storagecore.RepositoryStorage, string, string) error {
			return apperrors.Forbidden("nope")
		}

		got, err := sh.resolveCanonicalStreamSubscription(context.Background(), authed, "list:1")
		require.Empty(t, got)
		require.NotNil(t, err)
		require.Equal(t, apperrors.CodeBadRequest, err.Code)
	})

	t.Run("list unexpected auth error maps to failed subscribe", func(t *testing.T) {
		authorizeListStreamSubscriptionFn = func(context.Context, storagecore.RepositoryStorage, string, string) error {
			return errors.New("boom")
		}

		got, err := sh.resolveCanonicalStreamSubscription(context.Background(), authed, "list:1")
		require.Empty(t, got)
		require.NotNil(t, err)
		require.Equal(t, apperrors.CodeInternal, err.Code)
	})

	t.Run("list ok canonicalizes", func(t *testing.T) {
		authorizeListStreamSubscriptionFn = func(context.Context, storagecore.RepositoryStorage, string, string) error {
			return nil
		}

		got, err := sh.resolveCanonicalStreamSubscription(context.Background(), authed, "list:1")
		require.Nil(t, err)
		require.Equal(t, streaming.ListStreamName("1"), got)
	})
}

func TestAuthorizeListStreamSubscription_Round16(t *testing.T) {
	ctx := context.Background()

	t.Run("repos nil", func(t *testing.T) {
		err := authorizeListStreamSubscription(ctx, nil, "list-1", "alice")
		require.Equal(t, apperrors.CodeInternal, apperrors.GetErrorCode(err))
	})

	t.Run("list repo nil", func(t *testing.T) {
		repos := testutils.NewMockRepositoryStorage()
		err := authorizeListStreamSubscription(ctx, repos, "list-1", "alice")
		require.Equal(t, apperrors.CodeInternal, apperrors.GetErrorCode(err))
	})

	t.Run("get list error returns error", func(t *testing.T) {
		repos, mockDB, mockQuery := newMockReposWithList(t, mockListResult{firstErr: errors.New("db error")})
		err := authorizeListStreamSubscription(ctx, repos, "list-1", "alice")
		require.Error(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("list owned by different user", func(t *testing.T) {
		repos, _, _ := newMockReposWithList(t, mockListResult{username: "bob"})
		err := authorizeListStreamSubscription(ctx, repos, "list-1", "alice")
		require.Equal(t, apperrors.CodeNotFound, apperrors.GetErrorCode(err))
	})

	t.Run("success", func(t *testing.T) {
		repos, _, _ := newMockReposWithList(t, mockListResult{username: "alice"})
		require.NoError(t, authorizeListStreamSubscription(ctx, repos, "list-1", "alice"))
	})
}

type mockListResult struct {
	username string
	firstErr error
}

func newMockReposWithList(t *testing.T, result mockListResult) (storagecore.RepositoryStorage, *mocks.MockDB, *mocks.MockQuery) {
	t.Helper()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	if result.firstErr != nil {
		mockQuery.On("First", mock.Anything).Return(result.firstErr).Once()
	} else {
		mockQuery.On("First", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			list, ok := args.Get(0).(*models.List)
			require.True(t, ok)
			list.ID = "list-1"
			list.Username = result.username
		})
	}

	listRepo := repositories.NewListRepository(mockDB, "table", zap.NewNop(), nil)
	base := testutils.NewMockRepositoryStorage()
	repos := repositoryStorageWithList{RepositoryStorage: base, listRepo: listRepo}
	return repos, mockDB, mockQuery
}
