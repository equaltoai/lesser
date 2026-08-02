package graph

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	dynamormquery "github.com/theory-cloud/tabletheory/v2/pkg/query"
	"go.uber.org/zap"
)

func TestRound12CMS_DraftLifecycle(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	ctx := round12AuthContext("alice")

	mut := resolver.Mutation()
	qry := resolver.Query()

	title := "Hello Draft"
	draft, err := mut.CreateDraft(ctx, model.CreateDraftInput{
		ContentType:   model.ObjectTypeArticle,
		Title:         &title,
		Content:       "draft body",
		ContentFormat: model.ContentFormatMarkdown,
	})
	require.NoError(t, err)
	require.NotNil(t, draft)
	require.NotEmpty(t, draft.ID)
	require.Equal(t, "alice", draft.AuthorID)

	updatedTitle := "Updated Draft"
	slug := "updated-draft"
	updatedContent := "updated body"
	format := model.ContentFormatHTML
	draft, err = mut.UpdateDraft(ctx, draft.ID, model.UpdateDraftInput{
		Title:         &updatedTitle,
		Slug:          &slug,
		Content:       &updatedContent,
		ContentFormat: &format,
	})
	require.NoError(t, err)
	require.NotNil(t, draft.Title)
	require.Equal(t, updatedTitle, *draft.Title)

	draft, err = mut.AutosaveDraft(ctx, draft.ID, "autosaved body")
	require.NoError(t, err)
	require.NotNil(t, draft)

	draftsBeforePublish, err := qry.MyDrafts(ctx, nil, nil, ptrInt(1000), nil)
	require.NoError(t, err)
	require.NotNil(t, draftsBeforePublish)
	require.NotNil(t, draftsBeforePublish.PageInfo)
	require.GreaterOrEqual(t, draftsBeforePublish.TotalCount, 1)
	foundDraftAuthorID := ""
	for _, edge := range draftsBeforePublish.Edges {
		if edge.Node != nil && edge.Node.ID == draft.ID {
			foundDraftAuthorID = edge.Node.AuthorID
			break
		}
	}
	require.Equal(t, "alice", foundDraftAuthorID)

	when := model.Time(time.Now().Add(15 * time.Minute))
	draft, err = mut.ScheduleDraft(ctx, draft.ID, when)
	require.NoError(t, err)
	require.NotNil(t, draft)

	draft, err = mut.CancelScheduledDraft(ctx, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, draft)

	fetched, err := qry.Draft(ctx, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, draft.ID, fetched.ID)

	crossUserTitle := "Cross User Update"
	_, err = mut.UpdateDraft(round12AuthContext("bob"), draft.ID, model.UpdateDraftInput{
		Title: &crossUserTitle,
	})
	require.Error(t, err)

	toDeleteTitle := "Delete Me"
	toDelete, err := mut.CreateDraft(ctx, model.CreateDraftInput{
		ContentType:   model.ObjectTypeArticle,
		Title:         &toDeleteTitle,
		Content:       "bye",
		ContentFormat: model.ContentFormatMarkdown,
	})
	require.NoError(t, err)
	ok, err := mut.DeleteDraft(round12AuthContext("bob"), toDelete.ID)
	require.Error(t, err)
	require.False(t, ok)

	ok, err = mut.DeleteDraft(ctx, toDelete.ID)
	require.NoError(t, err)
	require.True(t, ok)

	article, err := mut.PublishDraft(ctx, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, article)
	require.Equal(t, "https://localhost/articles/updated-draft", article.ID)
	require.Equal(t, cmsLocalActorID(resolver.getDomain(), "alice"), article.AuthorID)
	require.NotEmpty(t, article.ID)
}

func TestRound12CMS_ArticleReadSurfacesVisibleTombstone(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	ctx := context.Background()
	deletedID := "https://localhost/articles/deleted-article"
	deletedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	tombstoneDB := new(dynamormmocks.MockDB)
	deletedQuery := new(dynamormmocks.MockQuery)
	missingQuery := new(dynamormmocks.MockQuery)
	storage.db = tombstoneDB

	tombstoneDB.On("WithContext", mock.Anything).Return(tombstoneDB).Twice()
	tombstoneDB.On("Model", mock.AnythingOfType("*models.Tombstone")).Return(deletedQuery).Once()
	deletedQuery.On("Where", "PK", "=", "OBJECT#"+deletedID).Return(deletedQuery).Once()
	deletedQuery.On("Where", "SK", "=", "TOMBSTONE").Return(deletedQuery).Once()
	deletedQuery.On("First", mock.AnythingOfType("*models.Tombstone")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Tombstone)
		*dest = models.Tombstone{
			ID:           deletedID,
			FormerType:   "Article",
			Deleted:      deletedAt,
			CreatedAt:    deletedAt,
			DeletedBy:    "https://localhost/users/alice",
			AttributedTo: "https://localhost/users/alice",
			IsPublic:     true,
		}
	}).Return(nil).Once()

	missingID := "https://localhost/articles/never-existed"
	tombstoneDB.On("Model", mock.AnythingOfType("*models.Tombstone")).Return(missingQuery).Once()
	missingQuery.On("Where", "PK", "=", "OBJECT#"+missingID).Return(missingQuery).Once()
	missingQuery.On("Where", "SK", "=", "TOMBSTONE").Return(missingQuery).Once()
	missingQuery.On("First", mock.AnythingOfType("*models.Tombstone")).Return(dynamormerrors.ErrItemNotFound).Once()

	deleted, err := resolver.Query().Article(ctx, deletedID)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	require.Equal(t, deletedID, deleted.ID)
	require.NotNil(t, deleted.DeletedAt)
	require.Equal(t, model.Time(deletedAt), *deleted.DeletedAt)

	missing, err := resolver.Query().Article(ctx, missingID)
	require.NoError(t, err)
	require.Nil(t, missing)

	tombstoneDB.AssertExpectations(t)
	deletedQuery.AssertExpectations(t)
	missingQuery.AssertExpectations(t)
}

func TestRound12CMS_ArticleTombstoneDisclosure(t *testing.T) {
	deletedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		ctx          context.Context
		formerType   string
		attributedTo string
		isPublic     bool
		wantVisible  bool
	}{
		{
			name:         "public article is visible",
			ctx:          context.Background(),
			formerType:   "Article",
			attributedTo: "https://localhost/users/alice",
			isPublic:     true,
			wantVisible:  true,
		},
		{
			name:         "non-public article is visible to original author",
			ctx:          round12AuthContext("alice"),
			formerType:   "Article",
			attributedTo: "https://localhost/users/alice",
			wantVisible:  true,
		},
		{
			name:         "non-public article is hidden from other viewer",
			ctx:          round12AuthContext("bob"),
			formerType:   "Article",
			attributedTo: "https://localhost/users/alice",
		},
		{
			name:       "legacy non-public tombstone without attribution is hidden",
			ctx:        round12AuthContext("alice"),
			formerType: "Article",
		},
		{
			name:         "non-article tombstone is hidden",
			ctx:          context.Background(),
			formerType:   "Note",
			attributedTo: "https://localhost/users/alice",
			isPublic:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, storage := newRound12GraphResolver(t)
			deletedID := "https://localhost/articles/deleted-article"
			tombstoneDB := new(dynamormmocks.MockDB)
			tombstoneQuery := new(dynamormmocks.MockQuery)
			storage.db = tombstoneDB

			tombstoneDB.On("WithContext", mock.Anything).Return(tombstoneDB).Once()
			tombstoneDB.On("Model", mock.AnythingOfType("*models.Tombstone")).Return(tombstoneQuery).Once()
			tombstoneQuery.On("Where", "PK", "=", "OBJECT#"+deletedID).Return(tombstoneQuery).Once()
			tombstoneQuery.On("Where", "SK", "=", "TOMBSTONE").Return(tombstoneQuery).Once()
			tombstoneQuery.On("First", mock.AnythingOfType("*models.Tombstone")).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.Tombstone)
				*dest = models.Tombstone{
					ID:           deletedID,
					FormerType:   tt.formerType,
					Deleted:      deletedAt,
					CreatedAt:    deletedAt,
					DeletedBy:    "https://localhost/users/alice",
					AttributedTo: tt.attributedTo,
					IsPublic:     tt.isPublic,
				}
			}).Return(nil).Once()

			article, err := resolver.Query().Article(tt.ctx, deletedID)
			require.NoError(t, err)
			if tt.wantVisible {
				require.NotNil(t, article)
				require.Equal(t, deletedID, article.ID)
			} else {
				require.Nil(t, article)
			}

			tombstoneDB.AssertExpectations(t)
			tombstoneQuery.AssertExpectations(t)
		})
	}
}

func TestRound12CMS_ArticleTombstoneVisibilityRequiresMatchingNonEmptyAttribution(t *testing.T) {
	aliceActorID := "https://example.com/users/alice"

	require.False(t, cmsArticleTombstoneVisibleToActor("", ""))
	require.False(t, cmsArticleTombstoneVisibleToActor("", aliceActorID))
	require.False(t, cmsArticleTombstoneVisibleToActor("https://example.com/users/bob", aliceActorID))
	require.True(t, cmsArticleTombstoneVisibleToActor("  "+aliceActorID+"  ", aliceActorID))
}

func TestRound12CMS_AnonymousViewerCannotSeeAttributedArticleTombstone(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	query := &queryResolver{resolver}
	tombstone := &models.Tombstone{
		FormerType:   "Article",
		AttributedTo: cmsLocalActorID(resolver.getDomain(), ""),
		IsPublic:     false,
	}

	require.False(t, query.cmsArticleTombstoneVisible(context.Background(), tombstone))
}

func TestRound12CMS_ArticleBySlugTombstoneDisclosure(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		wantVisible bool
	}{
		{name: "original author sees non-public tombstone", ctx: round12AuthContext("alice"), wantVisible: true},
		{name: "other viewer does not see non-public tombstone", ctx: round12AuthContext("bob")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, storage := newRound12GraphResolver(t)
			tombstoneDB := new(dynamormmocks.MockDB)
			tenantIndexQuery := new(dynamormmocks.MockQuery)
			legacyIndexQuery := new(dynamormmocks.MockQuery)
			tombstoneQuery := new(dynamormmocks.MockQuery)
			storage.db = tombstoneDB

			slug := "deleted-by-slug"
			deletedID := cmsArticleID(resolver.getDomain(), slug)
			tombstoneDB.On("WithContext", mock.Anything).Return(tombstoneDB).Times(3)
			tombstoneDB.On("Model", mock.AnythingOfType("*models.CMSSlugIndex")).Return(tenantIndexQuery).Once()
			tenantIndexQuery.On("Where", "PK", "=", models.CMSTenantArticleSlugIndexPK("localhost", slug)).Return(tenantIndexQuery).Once()
			tenantIndexQuery.On("Where", "SK", "=", models.CMSSlugIndexSK()).Return(tenantIndexQuery).Once()
			tenantIndexQuery.On("First", mock.AnythingOfType("*models.CMSSlugIndex")).Return(dynamormerrors.ErrItemNotFound).Once()

			tombstoneDB.On("Model", mock.AnythingOfType("*models.CMSSlugIndex")).Return(legacyIndexQuery).Once()
			legacyIndexQuery.On("Where", "PK", "=", models.CMSArticleSlugIndexPK(slug)).Return(legacyIndexQuery).Once()
			legacyIndexQuery.On("Where", "SK", "=", models.CMSSlugIndexSK()).Return(legacyIndexQuery).Once()
			legacyIndexQuery.On("First", mock.AnythingOfType("*models.CMSSlugIndex")).Return(dynamormerrors.ErrItemNotFound).Once()

			tombstoneDB.On("Model", mock.AnythingOfType("*models.Tombstone")).Return(tombstoneQuery).Once()
			tombstoneQuery.On("Where", "PK", "=", "OBJECT#"+deletedID).Return(tombstoneQuery).Once()
			tombstoneQuery.On("Where", "SK", "=", "TOMBSTONE").Return(tombstoneQuery).Once()
			tombstoneQuery.On("First", mock.AnythingOfType("*models.Tombstone")).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.Tombstone)
				*dest = models.Tombstone{
					ID:           deletedID,
					FormerType:   "Article",
					Deleted:      time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
					DeletedBy:    "https://localhost/users/alice",
					AttributedTo: "https://localhost/users/alice",
				}
			}).Return(nil).Once()

			article, err := resolver.Query().ArticleBySlug(tt.ctx, slug)
			require.NoError(t, err)
			if tt.wantVisible {
				require.NotNil(t, article)
				require.Equal(t, deletedID, article.ID)
			} else {
				require.Nil(t, article)
			}

			tombstoneDB.AssertExpectations(t)
			tenantIndexQuery.AssertExpectations(t)
			legacyIndexQuery.AssertExpectations(t)
			tombstoneQuery.AssertExpectations(t)
		})
	}
}

func TestRound12CMS_ArticleBySlugUsesIndexTargetForRenamedArticleTombstone(t *testing.T) {
	const (
		originalSlug = "a"
		currentSlug  = "b"
	)

	// Published article IDs remain fixed across slug-index aliases. Both the
	// current slug and the retained former-slug alias therefore point at the
	// original object ID after deletion.
	articleID := cmsArticleID("localhost", originalSlug)
	deletedAt := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)

	for _, slug := range []string{currentSlug, originalSlug} {
		slug := slug
		t.Run(slug, func(t *testing.T) {
			resolver, storage := newRound12GraphResolver(t)
			mockDB := new(dynamormmocks.MockDB)
			indexQuery := new(dynamormmocks.MockQuery)
			tombstoneQuery := new(dynamormmocks.MockQuery)
			storage.db = mockDB

			mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
			mockDB.On("Model", mock.AnythingOfType("*models.CMSSlugIndex")).Return(indexQuery).Once()
			indexQuery.On("Where", "PK", "=", models.CMSTenantArticleSlugIndexPK("localhost", slug)).Return(indexQuery).Once()
			indexQuery.On("Where", "SK", "=", models.CMSSlugIndexSK()).Return(indexQuery).Once()
			indexQuery.On("First", mock.AnythingOfType("*models.CMSSlugIndex")).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.CMSSlugIndex)
				*dest = models.CMSSlugIndex{Slug: slug, TargetID: articleID}
			}).Return(nil).Once()

			mockDB.On("Model", mock.AnythingOfType("*models.Tombstone")).Return(tombstoneQuery).Once()
			tombstoneQuery.On("Where", "PK", "=", "OBJECT#"+articleID).Return(tombstoneQuery).Once()
			tombstoneQuery.On("Where", "SK", "=", "TOMBSTONE").Return(tombstoneQuery).Once()
			tombstoneQuery.On("First", mock.AnythingOfType("*models.Tombstone")).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.Tombstone)
				*dest = models.Tombstone{
					ID:           articleID,
					FormerType:   "Article",
					Deleted:      deletedAt,
					CreatedAt:    deletedAt,
					AttributedTo: "https://localhost/users/alice",
					IsPublic:     true,
				}
			}).Return(nil).Once()

			article, err := resolver.Query().ArticleBySlug(context.Background(), slug)
			require.NoError(t, err)
			require.NotNil(t, article)
			require.Equal(t, articleID, article.ID)
			require.NotNil(t, article.DeletedAt)

			mockDB.AssertExpectations(t)
			indexQuery.AssertExpectations(t)
			tombstoneQuery.AssertExpectations(t)
		})
	}

	t.Run("never existed", func(t *testing.T) {
		resolver, storage := newRound12GraphResolver(t)
		mockDB := new(dynamormmocks.MockDB)
		tenantIndexQuery := new(dynamormmocks.MockQuery)
		legacyIndexQuery := new(dynamormmocks.MockQuery)
		tombstoneQuery := new(dynamormmocks.MockQuery)
		storage.db = mockDB

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)
		mockDB.On("Model", mock.AnythingOfType("*models.CMSSlugIndex")).Return(tenantIndexQuery).Once()
		tenantIndexQuery.On("Where", "PK", "=", models.CMSTenantArticleSlugIndexPK("localhost", "never-existed")).Return(tenantIndexQuery).Once()
		tenantIndexQuery.On("Where", "SK", "=", models.CMSSlugIndexSK()).Return(tenantIndexQuery).Once()
		tenantIndexQuery.On("First", mock.AnythingOfType("*models.CMSSlugIndex")).Return(dynamormerrors.ErrItemNotFound).Once()

		mockDB.On("Model", mock.AnythingOfType("*models.CMSSlugIndex")).Return(legacyIndexQuery).Once()
		legacyIndexQuery.On("Where", "PK", "=", models.CMSArticleSlugIndexPK("never-existed")).Return(legacyIndexQuery).Once()
		legacyIndexQuery.On("Where", "SK", "=", models.CMSSlugIndexSK()).Return(legacyIndexQuery).Once()
		legacyIndexQuery.On("First", mock.AnythingOfType("*models.CMSSlugIndex")).Return(dynamormerrors.ErrItemNotFound).Once()

		missingID := cmsArticleID("localhost", "never-existed")
		mockDB.On("Model", mock.AnythingOfType("*models.Tombstone")).Return(tombstoneQuery).Once()
		tombstoneQuery.On("Where", "PK", "=", "OBJECT#"+missingID).Return(tombstoneQuery).Once()
		tombstoneQuery.On("Where", "SK", "=", "TOMBSTONE").Return(tombstoneQuery).Once()
		tombstoneQuery.On("First", mock.AnythingOfType("*models.Tombstone")).Return(dynamormerrors.ErrItemNotFound).Once()

		article, err := resolver.Query().ArticleBySlug(context.Background(), "never-existed")
		require.NoError(t, err)
		require.Nil(t, article)
	})
}

func TestRound12CMS_AllSeriesGlobalPaginationRoundTrips(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	firstQuery := new(dynamormmocks.MockQuery)
	endCursorQuery := new(dynamormmocks.MockQuery)
	nonLastCursorQuery := new(dynamormmocks.MockQuery)
	startCursorQuery := new(dynamormmocks.MockQuery)
	storage.db = mockDB

	firstSeries := models.Series{PK: "AUTHOR#alice#SERIES", SK: "ID#series-1", ID: "series-1", AuthorID: "alice", Title: "One"}
	secondSeries := models.Series{PK: "AUTHOR#bob#SERIES", SK: "ID#series-2", ID: "series-2", AuthorID: "bob", Title: "Two"}
	thirdSeries := models.Series{PK: "AUTHOR#carol#SERIES", SK: "ID#series-3", ID: "series-3", AuthorID: "carol", Title: "Three"}
	firstCursor := round12SeriesCursor(t, &firstSeries)
	secondCursor := round12SeriesCursor(t, &secondSeries)

	mockDB.On("WithContext", ctx).Return(mockDB).Times(4)
	mockDB.On("Model", mock.AnythingOfType("*models.Series")).Return(firstQuery).Once()
	firstQuery.On("Where", "SK", "BEGINS_WITH", "ID#").Return(firstQuery).Once()
	firstQuery.On("Limit", 2).Return(firstQuery).Once()
	firstQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Series")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Series)
		*dest = []models.Series{firstSeries, secondSeries}
	}).Return(&dynamormcore.PaginatedResult{NextCursor: string(secondCursor)}, nil).Once()

	expectRound12GlobalSeriesPage := func(query *dynamormmocks.MockQuery, after model.Cursor, item models.Series) {
		mockDB.On("Model", mock.AnythingOfType("*models.Series")).Return(query).Once()
		query.On("Where", "SK", "BEGINS_WITH", "ID#").Return(query).Once()
		query.On("Limit", 2).Return(query).Once()
		query.On("Cursor", string(after)).Return(query).Once()
		query.On("AllPaginated", mock.AnythingOfType("*[]models.Series")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Series)
			*dest = []models.Series{item}
		}).Return(&dynamormcore.PaginatedResult{}, nil).Once()
	}
	expectRound12GlobalSeriesPage(endCursorQuery, secondCursor, thirdSeries)
	expectRound12GlobalSeriesPage(nonLastCursorQuery, firstCursor, secondSeries)
	expectRound12GlobalSeriesPage(startCursorQuery, firstCursor, secondSeries)

	first := 2
	pageOne, err := resolver.Query().AllSeries(ctx, nil, &first, nil)
	require.NoError(t, err)
	require.Len(t, pageOne.Edges, 2)
	require.True(t, pageOne.PageInfo.HasNextPage)
	require.NotNil(t, pageOne.PageInfo.StartCursor)
	require.NotNil(t, pageOne.PageInfo.EndCursor)
	require.Equal(t, firstCursor, pageOne.Edges[0].Cursor)
	require.Equal(t, firstCursor, *pageOne.PageInfo.StartCursor)
	require.Equal(t, secondCursor, pageOne.Edges[1].Cursor)
	require.Equal(t, secondCursor, *pageOne.PageInfo.EndCursor)
	for _, edge := range pageOne.Edges {
		decoded, decodeErr := dynamormquery.DecodeCursor(string(edge.Cursor))
		require.NoError(t, decodeErr)
		require.NotNil(t, decoded)
		_, decodeErr = decoded.ToAttributeValues()
		require.NoError(t, decodeErr)
	}

	pageAfterEnd, err := resolver.Query().AllSeries(ctx, nil, &first, pageOne.PageInfo.EndCursor)
	require.NoError(t, err)
	require.Len(t, pageAfterEnd.Edges, 1)
	require.Equal(t, "carol|series-3", pageAfterEnd.Edges[0].Node.ID)

	pageAfterNonLast, err := resolver.Query().AllSeries(ctx, nil, &first, &pageOne.Edges[0].Cursor)
	require.NoError(t, err)
	require.Len(t, pageAfterNonLast.Edges, 1)
	require.Equal(t, "bob|series-2", pageAfterNonLast.Edges[0].Node.ID)

	pageAfterStart, err := resolver.Query().AllSeries(ctx, nil, &first, pageOne.PageInfo.StartCursor)
	require.NoError(t, err)
	require.Len(t, pageAfterStart.Edges, 1)
	require.Equal(t, "bob|series-2", pageAfterStart.Edges[0].Node.ID)

	mockDB.AssertExpectations(t)
	firstQuery.AssertExpectations(t)
	endCursorQuery.AssertExpectations(t)
	nonLastCursorQuery.AssertExpectations(t)
	startCursorQuery.AssertExpectations(t)
}

func TestRound12CMS_AllSeriesAuthorPaginationRoundTrips(t *testing.T) {
	ctx := context.Background()
	seriesRepo := storagemocks.NewMockSeriesRepository()
	storage := pkgtesting.NewMockRepositoryStorage(pkgtesting.WithSeriesRepository(seriesRepo))
	resolver := &Resolver{
		Config: &config.Config{
			CMSLongFormPublishingEnabled: true,
			CMSSeriesEnabled:             true,
		},
		Storage: storage,
		Logger:  zap.NewNop(),
	}

	firstSeries := &models.Series{PK: "AUTHOR#alice#SERIES", SK: "ID#series-1", ID: "series-1", AuthorID: "alice", Title: "One"}
	secondSeries := &models.Series{PK: "AUTHOR#alice#SERIES", SK: "ID#series-2", ID: "series-2", AuthorID: "alice", Title: "Two"}
	firstCursor := round12SeriesCursor(t, firstSeries)
	secondCursor := round12SeriesCursor(t, secondSeries)
	rawNextCursor := secondSeries.SK
	require.NotEqual(t, string(secondCursor), rawNextCursor)

	seriesRepo.On("ListSeriesByAuthorPaginated", ctx, "alice", 2, "").
		Return([]*models.Series{firstSeries, secondSeries}, rawNextCursor, nil).
		Once()
	seriesRepo.On("ListSeriesByAuthorPaginated", ctx, "alice", 2, firstSeries.SK).
		Return([]*models.Series{secondSeries}, "", nil).
		Twice()

	authorID := "alice"
	first := 2
	pageOne, err := resolver.Query().AllSeries(ctx, &authorID, &first, nil)
	require.NoError(t, err)
	require.Len(t, pageOne.Edges, 2)
	require.True(t, pageOne.PageInfo.HasNextPage)
	require.NotNil(t, pageOne.PageInfo.StartCursor)
	require.NotNil(t, pageOne.PageInfo.EndCursor)
	require.Equal(t, firstCursor, pageOne.Edges[0].Cursor)
	require.Equal(t, firstCursor, *pageOne.PageInfo.StartCursor)
	require.Equal(t, secondCursor, pageOne.Edges[1].Cursor)
	require.Equal(t, secondCursor, *pageOne.PageInfo.EndCursor)
	for _, edge := range pageOne.Edges {
		decoded, decodeErr := dynamormquery.DecodeCursor(string(edge.Cursor))
		require.NoError(t, decodeErr)
		require.NotNil(t, decoded)
		_, decodeErr = decoded.ToAttributeValues()
		require.NoError(t, decodeErr)
	}

	pageAfterNonLast, err := resolver.Query().AllSeries(ctx, &authorID, &first, &pageOne.Edges[0].Cursor)
	require.NoError(t, err)
	require.Len(t, pageAfterNonLast.Edges, 1)
	require.Equal(t, "alice|series-2", pageAfterNonLast.Edges[0].Node.ID)

	pageAfterStart, err := resolver.Query().AllSeries(ctx, &authorID, &first, pageOne.PageInfo.StartCursor)
	require.NoError(t, err)
	require.Len(t, pageAfterStart.Edges, 1)
	require.Equal(t, "alice|series-2", pageAfterStart.Edges[0].Node.ID)

	seriesRepo.AssertExpectations(t)
}

func TestRound12CMS_AllSeriesAuthorPaginationRejectsCrossAuthorCursor(t *testing.T) {
	ctx := context.Background()
	seriesRepo := storagemocks.NewMockSeriesRepository()
	storage := pkgtesting.NewMockRepositoryStorage(pkgtesting.WithSeriesRepository(seriesRepo))
	resolver := &Resolver{
		Config: &config.Config{
			CMSLongFormPublishingEnabled: true,
			CMSSeriesEnabled:             true,
		},
		Storage: storage,
		Logger:  zap.NewNop(),
	}

	bobCursor := round12SeriesCursor(t, &models.Series{
		PK: "AUTHOR#bob#SERIES",
		SK: "ID#series-1",
	})
	alice := "alice"
	first := 2

	page, err := resolver.Query().AllSeries(ctx, &alice, &first, &bobCursor)
	require.Nil(t, page)
	require.EqualError(t, err, ErrInvalidAfterCursor.Error())
	seriesRepo.AssertNotCalled(t, "ListSeriesByAuthorPaginated", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestRound12CMS_AllSeriesAuthorPaginationRejectsTamperedCursorEnvelopes(t *testing.T) {
	tests := map[string]model.Cursor{
		"not encoded": "not-a-cursor",
		"missing PK": round12EncodeSeriesCursorEnvelope(t, map[string]types.AttributeValue{
			"SK": &types.AttributeValueMemberS{Value: "ID#series-1"},
		}),
		"non-string PK": round12EncodeSeriesCursorEnvelope(t, map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberN{Value: "1"},
			"SK": &types.AttributeValueMemberS{Value: "ID#series-1"},
		}),
		"missing SK": round12EncodeSeriesCursorEnvelope(t, map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "AUTHOR#alice#SERIES"},
		}),
		"non-string SK": round12EncodeSeriesCursorEnvelope(t, map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "AUTHOR#alice#SERIES"},
			"SK": &types.AttributeValueMemberN{Value: "1"},
		}),
	}

	for name, cursor := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			seriesRepo := storagemocks.NewMockSeriesRepository()
			storage := pkgtesting.NewMockRepositoryStorage(pkgtesting.WithSeriesRepository(seriesRepo))
			resolver := &Resolver{
				Config: &config.Config{
					CMSLongFormPublishingEnabled: true,
					CMSSeriesEnabled:             true,
				},
				Storage: storage,
				Logger:  zap.NewNop(),
			}
			alice := "alice"
			first := 2

			page, err := resolver.Query().AllSeries(ctx, &alice, &first, &cursor)
			require.Nil(t, page)
			require.EqualError(t, err, ErrInvalidAfterCursor.Error())
			seriesRepo.AssertNotCalled(t, "ListSeriesByAuthorPaginated", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func round12SeriesCursor(t *testing.T, series *models.Series) model.Cursor {
	t.Helper()
	return round12EncodeSeriesCursorEnvelope(t, map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: series.PK},
		"SK": &types.AttributeValueMemberS{Value: series.SK},
	})
}

func round12EncodeSeriesCursorEnvelope(t *testing.T, key map[string]types.AttributeValue) model.Cursor {
	t.Helper()
	encoded, err := dynamormquery.EncodeCursor(key, "", "")
	require.NoError(t, err)
	return model.Cursor(encoded)
}

func TestRound12CMS_ArticlesSeriesCategoriesPublications(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")
	adminCtx := round12AuthContext("admin")

	mut := resolver.Mutation()
	qry := resolver.Query()

	// Categories.
	categorySlug := "tech"
	category, err := mut.CreateCategory(adminCtx, model.CreateCategoryInput{
		Name: "Tech",
		Slug: &categorySlug,
	})
	require.NoError(t, err)
	require.NotNil(t, category)

	color := "#ff0"
	category, err = mut.UpdateCategory(adminCtx, category.ID, model.UpdateCategoryInput{
		Color: &color,
	})
	require.NoError(t, err)
	require.NotNil(t, category.Color)
	require.Equal(t, color, *category.Color)

	// Series.
	seriesSlug := "my-series"
	series, err := mut.CreateSeries(ctx, model.CreateSeriesInput{
		Title: "My Series",
		Slug:  &seriesSlug,
	})
	require.NoError(t, err)
	require.NotNil(t, series)

	newSeriesTitle := "My Series Updated"
	series, err = mut.UpdateSeries(ctx, series.ID, model.UpdateSeriesInput{
		Title: &newSeriesTitle,
	})
	require.NoError(t, err)
	require.Equal(t, newSeriesTitle, series.Title)

	// Articles.
	domain := resolver.getDomain()
	articleSlug := "hello-world"
	featuredID := "media-1"
	seoTitle := "SEO Title"
	editorNotes := "notes"
	article, err := mut.CreateArticle(ctx, model.CreateArticleInput{
		Slug:            &articleSlug,
		Title:           "Hello World",
		Content:         "<p>Hello</p>",
		ContentFormat:   model.ContentFormatHTML,
		SeriesID:        &series.ID,
		SeriesOrder:     ptrIntValue(1),
		CategoryIDs:     []string{category.ID},
		FeaturedImageID: &featuredID,
		SEOTitle:        &seoTitle,
		EditorNotes:     &editorNotes,
	})
	require.NoError(t, err)
	require.NotNil(t, article)
	require.Equal(t, cmsLocalActorID(domain, "alice"), article.AuthorID)

	blankSlug := "   "
	_, err = mut.UpdateArticle(ctx, article.ID, model.UpdateArticleInput{Slug: &blankSlug})
	require.Error(t, err)

	clearFeatured := ""
	newExcerpt := "excerpt"
	newContent := "updated content"
	updated, err := mut.UpdateArticle(ctx, article.ID, model.UpdateArticleInput{
		Excerpt:         &newExcerpt,
		Content:         &newContent,
		FeaturedImageID: &clearFeatured,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	// Series membership helpers.
	order := 2
	_, err = mut.AddArticleToSeries(ctx, series.ID, article.ID, &order)
	require.NoError(t, err)

	_, err = mut.ReorderSeriesArticles(ctx, series.ID, []string{article.ID})
	require.NoError(t, err)

	_, err = mut.RemoveArticleFromSeries(ctx, series.ID, article.ID)
	require.NoError(t, err)

	// Category membership helpers.
	_, err = mut.AddArticleToCategory(ctx, category.ID, article.ID)
	require.NoError(t, err)

	_, err = mut.RemoveArticleFromCategory(ctx, category.ID, article.ID)
	require.NoError(t, err)

	// Seed a revision and exercise revision queries + restore.
	revision := &models.Revision{
		ID:           "rev-1",
		ObjectID:     article.ID,
		Version:      999,
		Content:      "restored content",
		ChangedBy:    "alice",
		ChangeType:   "update",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MetadataJSON: "{}",
	}
	require.NoError(t, revision.UpdateKeys())
	require.NoError(t, storage.Revision().CreateRevision(ctx, revision))

	restored, err := mut.RestoreRevision(ctx, article.ID, 999)
	require.NoError(t, err)
	require.NotNil(t, restored)

	first := 10
	revisions, err := qry.Revisions(ctx, article.ID, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, revisions)

	revisionNode, err := qry.Revision(ctx, article.ID, 999)
	require.NoError(t, err)
	require.NotNil(t, revisionNode)

	// Article queries.
	byID, err := qry.Article(ctx, article.ID)
	require.NoError(t, err)
	require.NotNil(t, byID)
	require.Equal(t, cmsLocalActorID(domain, "alice"), byID.AuthorID)

	author := "alice"
	seriesFilter := series.ID
	categoryFilter := category.ID
	articlesByAuthor, err := qry.Articles(ctx, &author, &seriesFilter, &categoryFilter, &first, nil)
	require.NoError(t, err)
	if len(articlesByAuthor.Edges) > 0 {
		require.NotEmpty(t, articlesByAuthor.Edges[0].Node.AuthorID)
	}

	_, err = qry.Articles(ctx, nil, nil, nil, &first, nil)
	require.NoError(t, err)

	// Slug-based helpers: seed legacy rows so the fallback path is exercised.
	legacyArticle := &models.Article{
		Object: models.Object{
			ID:           cmsArticleID(domain, "legacy-article"),
			Type:         "Article",
			Name:         "Legacy",
			Content:      "legacy",
			AttributedTo: cmsLocalActorID(domain, "alice"),
		},
		Slug: "legacy-article",
	}
	require.NoError(t, storage.Article().CreateArticle(ctx, legacyArticle))

	legacyCategory := &models.Category{
		ID:        cmsCategoryID(domain, "legacy-category"),
		Name:      "Legacy Category",
		Slug:      "legacy-category",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.Category().CreateCategory(ctx, legacyCategory))

	legacyPublication := &models.Publication{
		ID:        cmsPublicationID(domain, "legacy-publication"),
		Name:      "Legacy Publication",
		Slug:      "legacy-publication",
		ActorID:   cmsLocalActorID(domain, "alice"),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.Publication().CreatePublication(ctx, legacyPublication))

	articleBySlug, err := qry.ArticleBySlug(ctx, "legacy-article")
	require.NoError(t, err)
	require.NotNil(t, articleBySlug)

	categoryBySlug, err := qry.CategoryBySlug(ctx, "legacy-category")
	require.NoError(t, err)
	require.NotNil(t, categoryBySlug)

	publicationBySlug, err := qry.PublicationBySlug(ctx, "legacy-publication")
	require.NoError(t, err)
	require.NotNil(t, publicationBySlug)

	// Publication lifecycle.
	publicationSlug := "my-publication"
	publication, err := mut.CreatePublication(ctx, model.CreatePublicationInput{
		Name: "My Publication",
		Slug: &publicationSlug,
	})
	require.NoError(t, err)
	require.NotNil(t, publication)

	tagline := "tagline"
	logoID := "media-logo"
	bannerID := "media-banner"
	publication, err = mut.UpdatePublication(ctx, publication.ID, model.UpdatePublicationInput{
		Tagline:  &tagline,
		LogoID:   &logoID,
		BannerID: &bannerID,
	})
	require.NoError(t, err)
	require.NotNil(t, publication)

	member, err := mut.InvitePublicationMember(ctx, publication.ID, "bob", model.PublicationRoleEditor)
	require.NoError(t, err)
	require.NotNil(t, member)

	member, err = mut.UpdatePublicationMemberRole(ctx, publication.ID, "bob", model.PublicationRoleWriter)
	require.NoError(t, err)
	require.NotNil(t, member)

	ok, err := mut.RemovePublicationMember(ctx, publication.ID, "bob")
	require.NoError(t, err)
	require.True(t, ok)

	// Query helpers across CMS.
	_, err = qry.Series(ctx, series.ID)
	require.NoError(t, err)

	_, err = qry.Series(ctx, "bad-id")
	require.Error(t, err)

	_, err = qry.SeriesBySlug(ctx, seriesSlug)
	require.NoError(t, err)

	_, err = qry.AllSeries(ctx, nil, &first, nil)
	require.NoError(t, err)

	noAuthSeries, err := qry.AllSeries(context.Background(), nil, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, noAuthSeries)

	_, err = qry.Category(ctx, category.ID)
	require.NoError(t, err)

	_, err = qry.Categories(ctx, nil)
	require.NoError(t, err)

	_, err = qry.RootCategories(ctx)
	require.NoError(t, err)

	_, err = qry.Publication(ctx, publication.ID)
	require.NoError(t, err)

	_, err = qry.MyPublications(ctx)
	require.NoError(t, err)

	// Cleanup path.
	ok, err = mut.DeleteArticle(ctx, article.ID)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = mut.DeleteCategory(adminCtx, category.ID)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestRound12CMS_MyPublicationsDoesNotSelfHealFallbackRows(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)

	fallbackMember := models.PublicationMember{
		PublicationID: "pub-1",
		UserID:        "alice",
		Role:          "owner",
		// Deliberately blank GSI keys model pre-M12 rows that need repair outside
		// of read resolvers.
		GSI1PK: "",
		GSI1SK: "",
	}

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", "SK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Limit", 1000).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		members, ok := args.Get(0).(*[]models.PublicationMember)
		require.True(t, ok)
		*members = []models.PublicationMember{fallbackMember}
	}).Return(nil).Once()

	pubRepo := inmemory.NewPublicationRepository()
	now := time.Now().UTC()
	require.NoError(t, pubRepo.CreatePublication(context.Background(), &models.Publication{
		ID:        "pub-1",
		Name:      "Publication",
		Slug:      "publication",
		ActorID:   "https://localhost/users/publication",
		CreatedAt: now,
		UpdatedAt: now,
	}))
	memberRepo := &round12NoHealPublicationMemberRepo{}
	baseStorage := pkgtesting.NewMockRepositoryStorage(
		pkgtesting.WithPublicationRepository(pubRepo),
		pkgtesting.WithPublicationMemberRepository(memberRepo),
	)
	storage := &round12NoHealCMSStorage{
		RepositoryStorage: baseStorage,
		db:                mockDB,
		publication:       pubRepo,
		member:            memberRepo,
	}
	resolver := &Resolver{
		Config: &config.Config{
			Domain:                       "localhost",
			CMSLongFormPublishingEnabled: true,
		},
		Storage: storage,
		Logger:  zap.NewNop(),
	}

	publications, err := resolver.Query().MyPublications(round12AuthContext("alice"))
	require.NoError(t, err)
	require.Len(t, publications, 1)
	require.Equal(t, "pub-1", publications[0].ID)
	require.Zero(t, memberRepo.updates, "read resolvers must not self-heal publication member rows")
}

type round12NoHealCMSStorage struct {
	core.RepositoryStorage
	db          dynamormcore.DB
	publication interfaces.PublicationRepository
	member      interfaces.PublicationMemberRepository
}

func (s *round12NoHealCMSStorage) GetDB() dynamormcore.DB { return s.db }
func (s *round12NoHealCMSStorage) Publication() interfaces.PublicationRepository {
	return s.publication
}
func (s *round12NoHealCMSStorage) PublicationMember() interfaces.PublicationMemberRepository {
	return s.member
}

type round12NoHealPublicationMemberRepo struct {
	*inmemory.PublicationMemberRepository
	updates int
}

func (r *round12NoHealPublicationMemberRepo) ensure() {
	if r.PublicationMemberRepository == nil {
		r.PublicationMemberRepository = inmemory.NewPublicationMemberRepository()
	}
}

func (r *round12NoHealPublicationMemberRepo) CreateMember(ctx context.Context, member *models.PublicationMember) error {
	r.ensure()
	return r.PublicationMemberRepository.CreateMember(ctx, member)
}

func (r *round12NoHealPublicationMemberRepo) GetMember(ctx context.Context, publicationID, userID string) (*models.PublicationMember, error) {
	r.ensure()
	return r.PublicationMemberRepository.GetMember(ctx, publicationID, userID)
}

func (r *round12NoHealPublicationMemberRepo) DeleteMember(ctx context.Context, publicationID, userID string) error {
	r.ensure()
	return r.PublicationMemberRepository.DeleteMember(ctx, publicationID, userID)
}

func (r *round12NoHealPublicationMemberRepo) ListMembers(ctx context.Context, publicationID string) ([]*models.PublicationMember, error) {
	r.ensure()
	return r.PublicationMemberRepository.ListMembers(ctx, publicationID)
}

func (r *round12NoHealPublicationMemberRepo) ListMembershipsForUserPaginated(context.Context, string, int, string) ([]*models.PublicationMember, string, error) {
	return nil, "", nil
}

func (r *round12NoHealPublicationMemberRepo) Update(context.Context, *models.PublicationMember) error {
	r.updates++
	return nil
}

func TestRound12CMS_MutationPermissions(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := resolver.Mutation()

	aliceCtx := round12AuthContext("alice")
	adminCtx := round12AuthContext("admin")
	bobCtx := round12AuthContext("bob")

	categorySlug := "private-taxonomy"
	_, err := mut.CreateCategory(aliceCtx, model.CreateCategoryInput{
		Name: "Private Taxonomy",
		Slug: &categorySlug,
	})
	require.Error(t, err)

	category, err := mut.CreateCategory(adminCtx, model.CreateCategoryInput{
		Name: "Private Taxonomy",
		Slug: &categorySlug,
	})
	require.NoError(t, err)
	require.NotNil(t, category)

	seriesSlug := "bob-series"
	bobSeries, err := mut.CreateSeries(bobCtx, model.CreateSeriesInput{
		Title: "Bob Series",
		Slug:  &seriesSlug,
	})
	require.NoError(t, err)
	require.NotNil(t, bobSeries)

	articleSlug := "alice-article"
	_, err = mut.CreateArticle(aliceCtx, model.CreateArticleInput{
		Slug:     &articleSlug,
		Title:    "Alice Article",
		Content:  "body",
		SeriesID: &bobSeries.ID,
	})
	require.Error(t, err)
}

func TestRound12CMS_SeriesMutationRejectsCrossTenantSeries(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := resolver.Mutation()
	aliceCtx := round12AuthContext("alice")

	cfg := resolver.Registry.GetConfig()
	require.NotNil(t, cfg)

	cfg.BaseURL = "https://tenant-a.example"
	seriesSlug := "tenant-series"
	tenantASeries, err := mut.CreateSeries(aliceCtx, model.CreateSeriesInput{
		Title: "Tenant A Series",
		Slug:  &seriesSlug,
	})
	require.NoError(t, err)
	require.NotNil(t, tenantASeries)

	authorID, rawSeriesID, ok := parseSeriesGraphQLID(tenantASeries.ID)
	require.True(t, ok)
	storedSeries, err := resolver.Registry.Series().GetSeries(context.Background(), authorID, rawSeriesID)
	require.NoError(t, err)
	require.Equal(t, "tenant-a.example", storedSeries.Tenant)

	cfg.BaseURL = "https://tenant-b.example"
	updatedTitle := "tenant-b takeover"
	_, err = mut.UpdateSeries(aliceCtx, tenantASeries.ID, model.UpdateSeriesInput{Title: &updatedTitle})
	require.Error(t, err)

	articleSlug := "tenant-b-article"
	_, err = mut.CreateArticle(aliceCtx, model.CreateArticleInput{
		Slug:     &articleSlug,
		Title:    "Tenant B Article",
		Content:  "body",
		SeriesID: &tenantASeries.ID,
	})
	require.Error(t, err)

	article, err := mut.CreateArticle(aliceCtx, model.CreateArticleInput{
		Slug:    &articleSlug,
		Title:   "Tenant B Article",
		Content: "body",
	})
	require.NoError(t, err)
	require.NotNil(t, article)
	require.Equal(t, "https://tenant-b.example/articles/tenant-b-article", article.ID)

	order := 1
	_, err = mut.AddArticleToSeries(aliceCtx, tenantASeries.ID, article.ID, &order)
	require.Error(t, err)

	_, err = mut.RemoveArticleFromSeries(aliceCtx, tenantASeries.ID, article.ID)
	require.Error(t, err)

	_, err = mut.ReorderSeriesArticles(aliceCtx, tenantASeries.ID, []string{article.ID})
	require.Error(t, err)

	ok, err = mut.DeleteSeries(aliceCtx, tenantASeries.ID)
	require.Error(t, err)
	require.False(t, ok)
}

func TestRound12CMS_ArticleWritesRejectCrossTenantArticle(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	mut := resolver.Mutation()
	aliceCtx := round12AuthContext("alice")

	cfg := resolver.Registry.GetConfig()
	require.NotNil(t, cfg)

	cfg.BaseURL = "https://tenant-a.example"
	articleSlug := "tenant-a-article"
	tenantAArticle, err := mut.CreateArticle(aliceCtx, model.CreateArticleInput{
		Slug:    &articleSlug,
		Title:   "Tenant A Article",
		Content: "tenant a body",
	})
	require.NoError(t, err)
	require.NotNil(t, tenantAArticle)
	require.Equal(t, "https://tenant-a.example/articles/tenant-a-article", tenantAArticle.ID)

	revision := &models.Revision{
		ID:           "tenant-a-rev-1",
		ObjectID:     tenantAArticle.ID,
		Version:      7,
		Content:      "restored tenant a body",
		ChangedBy:    "alice",
		ChangeType:   "update",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MetadataJSON: "{}",
	}
	require.NoError(t, revision.UpdateKeys())
	require.NoError(t, storage.Revision().CreateRevision(aliceCtx, revision))

	cfg.BaseURL = "https://tenant-b.example"
	updatedTitle := "tenant-b takeover"
	_, err = mut.UpdateArticle(aliceCtx, tenantAArticle.ID, model.UpdateArticleInput{Title: &updatedTitle})
	require.Error(t, err)

	restored, err := mut.RestoreRevision(aliceCtx, tenantAArticle.ID, 7)
	require.Error(t, err)
	require.Nil(t, restored)

	ok, err := mut.DeleteArticle(aliceCtx, tenantAArticle.ID)
	require.Error(t, err)
	require.False(t, ok)

	storedArticle, err := storage.Article().GetArticle(context.Background(), tenantAArticle.ID)
	require.NoError(t, err)
	require.Equal(t, "Tenant A Article", storedArticle.Name)
}

func TestRound12CMS_ArticleCreateCanonicalIDSlugCollisionsAreTenantScoped(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := resolver.Mutation()
	aliceCtx := round12AuthContext("alice")

	cfg := resolver.Registry.GetConfig()
	require.NotNil(t, cfg)

	slug := "shared-slug"
	cfg.BaseURL = "https://tenant-a.example"
	tenantAArticle, err := mut.CreateArticle(aliceCtx, model.CreateArticleInput{
		Slug:    &slug,
		Title:   "Tenant A Article",
		Content: "tenant a body",
	})
	require.NoError(t, err)
	require.NotNil(t, tenantAArticle)
	require.Equal(t, "https://tenant-a.example/articles/shared-slug", tenantAArticle.ID)

	_, err = mut.CreateArticle(aliceCtx, model.CreateArticleInput{
		Slug:    &slug,
		Title:   "Tenant A Duplicate",
		Content: "duplicate",
	})
	require.Error(t, err)

	cfg.BaseURL = "https://tenant-b.example"
	tenantBArticle, err := mut.CreateArticle(aliceCtx, model.CreateArticleInput{
		Slug:    &slug,
		Title:   "Tenant B Article",
		Content: "tenant b body",
	})
	require.NoError(t, err)
	require.NotNil(t, tenantBArticle)
	require.Equal(t, "https://tenant-b.example/articles/shared-slug", tenantBArticle.ID)
}

func TestRound12CMS_HelperBranches(t *testing.T) {
	memberGetter := &round12PublicationMemberGetterStub{}

	mr := &mutationResolver{Resolver: &Resolver{}}
	err := mr.ensureCanUpdatePublication(context.Background(), "alice", &models.Publication{ID: "pub-1"}, memberGetter)
	require.Error(t, err)

	memberGetter.member = &models.PublicationMember{Role: cmsPublicationRoleToStorage(model.PublicationRoleWriter)}
	err = mr.ensureCanUpdatePublication(context.Background(), "alice", &models.Publication{ID: "pub-1"}, memberGetter)
	require.Error(t, err)

	memberGetter.member = &models.PublicationMember{Role: cmsPublicationRoleToStorage(model.PublicationRoleOwner)}
	err = mr.ensureCanUpdatePublication(context.Background(), "alice", &models.Publication{ID: "pub-1"}, memberGetter)
	require.NoError(t, err)

	article := &models.Article{}
	require.Error(t, cmsApplyArticleFeaturedImage(context.Background(), nil, nil, ptrString("x")))
	require.ErrorIs(t, cmsApplyArticleFeaturedImage(context.Background(), nil, article, ptrString("x")), ErrStorageUnavailable)

	article.FeaturedImage = &models.Media{MediaID: "preexisting"}
	require.NoError(t, cmsApplyArticleFeaturedImage(context.Background(), &round12MediaGetterStub{url: "https://cdn.example/media.png"}, article, ptrString("")))
	require.Nil(t, article.FeaturedImage)

	require.NoError(t, cmsApplyArticleFeaturedImage(context.Background(), &round12MediaGetterStub{url: "https://cdn.example/media.png"}, article, ptrString("media-1")))
	require.NotNil(t, article.FeaturedImage)

	url, err := cmsMediaURLFromID(context.Background(), &round12MediaGetterStub{url: "https://cdn.example/logo.png"}, " ")
	require.NoError(t, err)
	require.Empty(t, url)

	url, err = cmsMediaURLFromID(context.Background(), &round12MediaGetterStub{url: "https://cdn.example/logo.png"}, "logo")
	require.NoError(t, err)
	require.Equal(t, "https://cdn.example/logo.png", url)

	require.Equal(t, "", derefString(nil))
	require.Equal(t, "x", derefString(ptrString("x")))
}

type round12PublicationMemberGetterStub struct {
	member *models.PublicationMember
}

func (s *round12PublicationMemberGetterStub) GetMember(context.Context, string, string) (*models.PublicationMember, error) {
	return s.member, nil
}

type round12MediaGetterStub struct {
	url string
}

func (s *round12MediaGetterStub) GetMedia(context.Context, string) (*models.Media, error) {
	return &models.Media{CDNUrl: s.url}, nil
}

func ptrInt(value int) *int {
	return &value
}

func ptrIntValue(value int) *int {
	return &value
}

func ptrString(value string) *string {
	return &value
}
